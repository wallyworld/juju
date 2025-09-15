// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	maasError error
}

func TestErrorSuite(t *tctesting.T) {
	tc.Run(t, &ErrorSuite{})
}

func (s *ErrorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.maasError = gomaasapi.NewPermissionError("denial")
}

func (s *ErrorSuite) TestNilContext(c *tc.C) {
	denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, s.maasError, nil)
	//c.Assert(c.GetTestLog(), tc.DeepEquals, "")
	c.Assert(denied, tc.IsTrue)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *tc.C) {
	ctx := context.NewEmptyCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, s.maasError, ctx)
	//c.Assert(c.GetTestLog(), tc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(denied, tc.IsTrue)
}

func (s *ErrorSuite) TestHandleCredentialErrorPermissionError(c *tc.C) {
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Trace(s.maasError)
	s.checkMaasPermissionHandling(c, true)

	s.maasError = errors.Annotatef(s.maasError, "more and more")
	s.checkMaasPermissionHandling(c, true)
}

func (s *ErrorSuite) TestHandleCredentialErrorAnotherError(c *tc.C) {
	s.maasError = errors.New("fluffy")
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestNilError(c *tc.C) {
	s.maasError = nil
	s.checkMaasPermissionHandling(c, false)
}

func (s *ErrorSuite) TestGomaasError(c *tc.C) {
	// check accepted status codes
	s.maasError = gomaasapi.ServerError{StatusCode: http.StatusAccepted}
	s.checkMaasPermissionHandling(c, false)

	for t := range common.AuthorisationFailureStatusCodes {
		s.maasError = gomaasapi.ServerError{StatusCode: t}
		s.checkMaasPermissionHandling(c, true)
	}
}

func (s *ErrorSuite) checkMaasPermissionHandling(c *tc.C, handled bool) {
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, tc.Matches, "cloud denied access:.*")
		called = true
		return nil
	}

	denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, s.maasError, ctx)
	c.Assert(called, tc.Equals, handled)
	c.Assert(denied, tc.Equals, handled)
}
