// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/credentialmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type CredentialManagerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	backend    *testBackend

	api *credentialmanager.CredentialManagerAPI
}

func TestCredentialManagerSuite(t *tctesting.T) {
	tc.Run(t, &CredentialManagerSuite{})
}

func (s *CredentialManagerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.backend = newMockBackend()

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("read"),
		AdminTag: names.NewUserTag("admin"),
	}
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	api, err := credentialmanager.NewCredentialManagerAPIForTest(s.backend, s.resources, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialUnauthorized(c *tc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := credentialmanager.NewCredentialManagerAPIForTest(s.backend, s.resources, s.authorizer)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *tc.C) {
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testhelpers.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialError(c *tc.C) {
	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResult{Error: apiservererrors.ServerError(expected)})
	s.backend.CheckCalls(c, []testhelpers.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func newMockBackend() *testBackend {
	return &testBackend{
		Stub: &testhelpers.Stub{},
	}
}

type testBackend struct {
	*testhelpers.Stub
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}
