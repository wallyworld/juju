// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/gce/internal/google"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	googleError   *url.Error
	internalError *googlyError
}

func TestErrorSuite(t *tctesting.T) {
	tc.Run(t, &ErrorSuite{})
}

func (s *ErrorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.internalError = &googlyError{"400 Bad Request"}
	s.googleError = &url.Error{"Get", "http://notforreal.com/", s.internalError}
}

func (s *ErrorSuite) TestNilContext(c *tc.C) {
	err := google.HandleCredentialError(s.googleError, nil)
	c.Assert(err, tc.DeepEquals, s.googleError)
	//c.Assert(c.GetTestLog(), tc.DeepEquals, "")
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	google.HandleCredentialError(s.googleError, ctx)
	//c.Assert(c.GetTestLog(), tc.Contains, "could not invalidate stored google cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, tc.Matches,
			regexp.QuoteMeta(`google cloud denied access: Get "http://notforreal.com/": 40`)+".*")
		called = true
		return nil
	}

	// First test another status code.
	s.internalError.SetMessage(http.StatusAccepted, "Accepted")
	google.HandleCredentialError(s.googleError, ctx)
	c.Assert(called, tc.IsFalse)

	for code, descs := range google.AuthorisationFailureStatusCodes {
		for _, desc := range descs {
			called = false
			s.internalError.SetMessage(code, desc)
			google.HandleCredentialError(s.googleError, ctx)
			c.Assert(called, tc.IsTrue)
		}
	}

	called = false
	for code := range google.AuthorisationFailureStatusCodes {
		s.internalError.SetMessage(code, "Some strange error")
		google.HandleCredentialError(s.googleError, ctx)
		c.Assert(called, tc.IsFalse)
	}
}

func (*ErrorSuite) TestNilGoogleError(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}
	returnedErr := google.HandleCredentialError(nil, ctx)
	c.Assert(called, tc.IsFalse)
	c.Assert(returnedErr, tc.ErrorIsNil)
}

func (*ErrorSuite) TestAnyOtherError(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}

	notinterestingErr := errors.New("not kaboom")
	returnedErr := google.HandleCredentialError(notinterestingErr, ctx)
	c.Assert(called, tc.IsFalse)
	c.Assert(returnedErr, tc.DeepEquals, notinterestingErr)
}

type googlyError struct {
	msg string
}

func (e *googlyError) Error() string { return e.msg }

func (e *googlyError) SetMessage(code int, desc string) {
	e.msg = fmt.Sprintf("%v %v", code, desc)
}
