// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type CredentialSuite struct {
	testhelpers.IsolationSuite

	backend *testBackend
	api     *credentialcommon.CredentialManagerAPI
}

func TestCredentialSuite(t *tctesting.T) {
	tc.Run(t, &CredentialSuite{})
}

func (s *CredentialSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.backend = newMockBackend()
	s.api = credentialcommon.NewCredentialManagerAPI(s.backend)
}

func (s *CredentialSuite) TestInvalidateModelCredential(c *tc.C) {
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testhelpers.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialSuite) TestInvalidateModelCredentialError(c *tc.C) {
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
	return &testBackend{Stub: &testhelpers.Stub{}}
}

type testBackend struct {
	*testhelpers.Stub
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}
