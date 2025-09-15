// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

import (
	"io"
	"net/http"
	"strings"
	tctesting "testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	azureError *azcore.ResponseError
}

func TestErrorSuite(t *tctesting.T) {
	tc.Run(t, &ErrorSuite{})
}

func (s *ErrorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.azureError = &azcore.ResponseError{
		StatusCode: http.StatusUnauthorized,
	}
}

func (s *ErrorSuite) TestNilContext(c *tc.C) {
	err := errorutils.HandleCredentialError(s.azureError, nil)
	c.Assert(err, tc.DeepEquals, s.azureError)

	invalidated := errorutils.MaybeInvalidateCredential(s.azureError, nil)
	c.Assert(invalidated, tc.IsFalse)

	//c.Assert(c.GetTestLog(), tc.DeepEquals, "")
}

func (s *ErrorSuite) TestHasDenialStatusCode(c *tc.C) {
	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusUnauthorized}), tc.IsTrue)
	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusNotFound}), tc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(nil), tc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(errors.New("FAIL")), tc.IsFalse)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	errorutils.MaybeInvalidateCredential(s.azureError, ctx)
	//c.Assert(c.GetTestLog(), tc.Contains, "could not invalidate stored azure cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, tc.Matches, "(?m)azure cloud denied access: .*")
		called = true
		return nil
	}

	// First test another status code.
	s.azureError.StatusCode = http.StatusAccepted
	errorutils.HandleCredentialError(s.azureError, ctx)
	c.Assert(called, tc.IsFalse)

	for t := range common.AuthorisationFailureStatusCodes {
		called = false
		s.azureError.StatusCode = t
		s.azureError.ErrorCode = "some error code"
		s.azureError.RawResponse = &http.Response{}
		errorutils.HandleCredentialError(s.azureError, ctx)
		c.Assert(called, tc.IsTrue)
	}
}

func (*ErrorSuite) TestNilAzureError(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}
	returnedErr := errorutils.HandleCredentialError(nil, ctx)
	c.Assert(called, tc.IsFalse)
	c.Assert(returnedErr, tc.ErrorIsNil)
}

func (*ErrorSuite) TestMaybeQuotaExceededError(c *tc.C) {
	buf := strings.NewReader(
		`{"error": {"code": "DeployError", "details": [{"code": "QuotaExceeded", "message": "boom"}]}}`)
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	quotaErr, ok := errorutils.MaybeQuotaExceededError(re)
	c.Assert(ok, tc.IsTrue)
	c.Assert(quotaErr, tc.ErrorMatches, "boom")
}

func (*ErrorSuite) TestMaybeHypervisorGenNotSupportedError(c *tc.C) {
	buf := strings.NewReader(`
{"error":{"code":"DeployError","message":"","details":[{"code":"DeploymentFailed","message":"{\"error\":{\"code\":\"BadRequest\",\"message\":\"The selected VM size 'Standard_D2_v2' cannot boot Hypervisor Generation '2'. If this was a Create operation please check that the Hypervisor Generation of the Image matches the Hypervisor Generation of the selected VM Size. If this was an Update operation please select a Hypervisor Generation '2' VM Size. For more information, see https://aka.ms/azuregen2vm\",\"details\":null}}"}]}}`[1:])
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "DeploymentFailed",
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	_, ok := errorutils.MaybeHypervisorGenNotSupportedError(re)
	c.Assert(ok, tc.IsTrue)
}

func (*ErrorSuite) TestIsConflictError(c *tc.C) {
	buf := strings.NewReader(
		`{"error": {"code": "DeployError", "details": [{"code": "Conflict", "message": "boom"}]}}`)

	re := &azcore.ResponseError{
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	ok := errorutils.IsConflictError(re)
	c.Assert(ok, tc.IsTrue)

	se2 := &azcore.ResponseError{
		StatusCode: http.StatusConflict,
	}
	ok = errorutils.IsConflictError(se2)
	c.Assert(ok, tc.IsTrue)
}

func (*ErrorSuite) TestStatusCode(c *tc.C) {
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
	}
	code := errorutils.StatusCode(re)
	c.Assert(code, tc.Equals, http.StatusBadRequest)
}

func (*ErrorSuite) TestErrorCode(c *tc.C) {
	re := &azcore.ResponseError{
		ErrorCode: "failed",
	}
	code := errorutils.ErrorCode(re)
	c.Assert(code, tc.Equals, "failed")
}

func (*ErrorSuite) TestSimpleError(c *tc.C) {
	buf := strings.NewReader(
		`{"error": {"message": "failed"}}`)

	re := &azcore.ResponseError{
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}

	err := errorutils.SimpleError(re)
	c.Assert(err, tc.ErrorMatches, "failed")
}
