// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy_test

import (
	"context"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/internal/provider/kubernetes/proxy"
)

type infoSuite struct {
	client *fake.Clientset
	clock  *testclock.Clock
}

func TestInfoSuite(t *tctesting.T) {
	tc.Run(t, &infoSuite{})
}

func (i *infoSuite) SetUpTest(c *tc.C) {
	i.clock = testclock.NewClock(time.Time{})

	i.client = fake.NewSimpleClientset()
	_, err := i.client.CoreV1().Namespaces().Create(context.TODO(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: testNamespace,
			},
		},
		meta.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (i *infoSuite) TestHasControllerProxyFalse(c *tc.C) {
	has, err := proxy.HasControllerProxy("test",
		i.client.CoreV1().ConfigMaps(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsFalse)
}

func (i *infoSuite) TestHasControllerProxy(c *tc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := i.client.CoreV1().Secrets(testNamespace).Create(context.TODO(), &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels.Set{},
			Name:   config.Name,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: config.Name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		c.Context(),
		config,
		labels.Set{},
		i.clock,
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)

	has, err := proxy.HasControllerProxy(config.Name,
		i.client.CoreV1().ConfigMaps(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

func (i *infoSuite) TestGetControllerProxier(c *tc.C) {
	config := proxy.ControllerProxyConfig{
		Name:          "controller-proxy",
		Namespace:     testNamespace,
		RemotePort:    "17707",
		TargetService: "controller-service",
	}

	// fake k8s client does not populate the token for secret, so we have to do it manually.
	_, err := i.client.CoreV1().Secrets(testNamespace).Create(context.TODO(), &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels.Set{},
			Name:   config.Name,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: config.Name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	err = proxy.CreateControllerProxy(
		c.Context(),
		config,
		labels.Set{},
		i.clock,
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.RbacV1().Roles(testNamespace),
		i.client.RbacV1().RoleBindings(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = proxy.GetControllerProxy(
		config.Name,
		"https://localhost:8123",
		i.client.CoreV1().ConfigMaps(testNamespace),
		i.client.CoreV1().ServiceAccounts(testNamespace),
		i.client.CoreV1().Secrets(testNamespace),
	)
	c.Assert(err, tc.ErrorIsNil)
}
