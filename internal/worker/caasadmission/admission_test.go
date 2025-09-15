// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	admission "k8s.io/api/admissionregistration/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/worker/caasadmission"
	pkitest "github.com/juju/juju/pki/test"
)

type AdmissionSuite struct {
}

type dummyAdmissionCreator struct {
	EnsureMutatingWebhookConfigurationFunc func() (func(), error)
}

func TestAdmissionSuite(t *tctesting.T) {
	tc.Run(t, &AdmissionSuite{})
}

func (d *dummyAdmissionCreator) EnsureMutatingWebhookConfiguration() (func(), error) {
	if d.EnsureMutatingWebhookConfigurationFunc == nil {
		return func() {}, nil
	}
	return d.EnsureMutatingWebhookConfigurationFunc()
}

func int32Ptr(i int32) *int32 {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func (a *AdmissionSuite) TestAdmissionCreatorObject(c *tc.C) {
	var (
		ensureWebhookCalled              = false
		ensureWebhookCleanupCalled       = false
		namespace                        = "testns"
		path                             = "/test"
		port                       int32 = 1111
		svcName                          = "testsvc"
	)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	serviceRef := &admission.ServiceReference{
		Namespace: namespace,
		Name:      svcName,
		Path:      strPtr(path),
		Port:      int32Ptr(port),
	}

	admissionCreator, err := caasadmission.NewAdmissionCreator(
		authority, "testns", "testmodel", "deadbeef", "badf00d", constants.LabelVersion1,
		func(obj *admission.MutatingWebhookConfiguration) (func(), error) {
			ensureWebhookCalled = true

			c.Assert(obj.Namespace, tc.Equals, namespace)
			c.Assert(len(obj.Webhooks), tc.Equals, 1)
			webhook := obj.Webhooks[0]
			c.Assert(webhook.AdmissionReviewVersions, tc.DeepEquals, []string{"v1beta1"})
			c.Assert(webhook.SideEffects, tc.NotNil)
			c.Assert(*webhook.SideEffects, tc.Equals, admission.SideEffectClassNone)
			svc := webhook.ClientConfig.Service
			c.Assert(svc.Name, tc.Equals, svcName)
			c.Assert(svc.Namespace, tc.Equals, namespace)
			c.Assert(*svc.Path, tc.Equals, path)
			c.Assert(*svc.Port, tc.Equals, port)

			return func() { ensureWebhookCleanupCalled = true }, nil
		}, serviceRef)

	c.Assert(err, tc.ErrorIsNil)

	cleanup, err := admissionCreator.EnsureMutatingWebhookConfiguration()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ensureWebhookCalled, tc.IsTrue)

	cleanup()
	c.Assert(ensureWebhookCleanupCalled, tc.IsTrue)
}
