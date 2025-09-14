// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type roleBindingSuite struct {
	resourceSuite
	namespace         string
	roleBindingClient rbacv1client.RoleBindingInterface
}

func TestRoleBindingSuite(t *tctesting.T) {
	tc.Run(t, &roleBindingSuite{})
}

func (s *roleBindingSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.roleBindingClient = s.client.RbacV1().RoleBindings(s.namespace)
}

func (s *roleBindingSuite) TestApply(c *tc.C) {
	RoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	// Create.
	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", RoleBinding)
	c.Assert(rbResource.Apply(context.TODO()), tc.ErrorIsNil)
	result, err := s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	RoleBinding.SetAnnotations(map[string]string{"a": "b"})
	rbResource = resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", RoleBinding)
	c.Assert(rbResource.Apply(context.TODO()), tc.ErrorIsNil)

	result, err = s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestGet(c *tc.C) {
	template := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	roleBinding1 := template
	roleBinding1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.TODO(), &roleBinding1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", &template)
	c.Assert(len(rbResource.GetAnnotations()), tc.Equals, 0)
	err = rbResource.Get(context.TODO())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rbResource.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(rbResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(rbResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestDelete(c *tc.C) {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.TODO(), &roleBinding, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)

	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", &roleBinding)
	err = rbResource.Delete(context.TODO())
	c.Assert(err, tc.ErrorIsNil)

	err = rbResource.Delete(context.TODO())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = rbResource.Get(context.TODO())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *roleBindingSuite) TestListRoleBindings(c *tc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create rb1
	rb1Name := "rb1"
	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rb1Name,
			Labels: labelSet,
		},
	}
	_, err = s.roleBindingClient.Create(context.TODO(), rb1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create rb2
	rb2Name := "rb2"
	rb2 := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rb2Name,
			Labels: labelSet,
		},
	}
	_, err = s.roleBindingClient.Create(context.TODO(), rb2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	rbs, err := resources.ListRoleBindings(c.Context(), s.roleBindingClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rbs), tc.Equals, 2)
	c.Assert(rbs[0].GetName(), tc.Equals, rb1Name)
	c.Assert(rbs[1].GetName(), tc.Equals, rb2Name)

	// List resources with no labels.
	rbs, err = resources.ListRoleBindings(c.Context(), s.roleBindingClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rbs), tc.Equals, 2)

	// List resources with wrong labels.
	rbs, err = resources.ListRoleBindings(c.Context(), s.roleBindingClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rbs), tc.Equals, 0)
}
