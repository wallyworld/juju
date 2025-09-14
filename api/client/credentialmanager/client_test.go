// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/credentialmanager"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

func TestCredentialManagerSuite(t *tctesting.T) {
	tc.Run(t, &CredentialManagerSuite{})
}

type CredentialManagerSuite struct {
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	args := params.InvalidateCredentialArg{Reason: "auth fail"}
	result := new(params.ErrorResult)
	results := params.ErrorResult{}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("InvalidateModelCredential", args, result).SetArg(2, results).Return(nil)
	client := credentialmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.InvalidateModelCredential("auth fail")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialBackendFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	args := params.InvalidateCredentialArg{}
	result := new(params.ErrorResult)
	results := params.ErrorResult{Error: apiservererrors.ServerError(errors.New("boom"))}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("InvalidateModelCredential", args, result).SetArg(2, results).Return(nil)
	client := credentialmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.InvalidateModelCredential("")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	args := params.InvalidateCredentialArg{}
	result := new(params.ErrorResult)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("InvalidateModelCredential", args, result).Return(errors.New("foo"))
	client := credentialmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.InvalidateModelCredential("")
	c.Assert(err, tc.ErrorMatches, "foo")
}
