// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testhelpers"
)

type ErrorsSuite struct {
	testhelpers.IsolationSuite
}

func TestErrorsSuite(t *tctesting.T) {
	tc.Run(t, &ErrorsSuite{})
}

func (*ErrorsSuite) TestWrapZoneIndependentError(c *tc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	wrapped := environs.ZoneIndependentError(err2)
	c.Assert(errors.Is(wrapped, environs.ErrAvailabilityZoneIndependent), tc.IsTrue)
	c.Assert(wrapped, tc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestInvalidCredentialWrapped(c *tc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	err := common.CredentialNotValidError(err2)

	// This is to confirm that Is(err, ErrorCredentialNotValid) is correct.
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestCredentialNotValidErrorLocationer(c *tc.C) {
	err := errors.New("some error")
	err = common.CredentialNotValidError(err)
	_, ok := err.(errors.Locationer)
	c.Assert(ok, tc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidCredentialNew(c *tc.C) {
	err := fmt.Errorf("%w: Your account is blocked.", common.ErrorCredentialNotValid)
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "credential not valid: Your account is blocked.")
}

func (s *ErrorsSuite) TestInvalidCredentialf(c *tc.C) {
	err1 := errors.New("foo")
	err := fmt.Errorf("bar: %w", common.CredentialNotValidError(err1))
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "bar: foo")
}

var authFailureError = errors.New("auth failure")

func (s *ErrorsSuite) TestNilContext(c *tc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	denied := common.MaybeHandleCredentialError(isAuthF, authFailureError, nil)
	//c.Assert(c.GetTestLog(), tc.DeepEquals, "")
	c.Assert(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidationCallbackErrorOnlyLogs(c *tc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	ctx := context.NewEmptyCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	denied := common.MaybeHandleCredentialError(isAuthF, authFailureError, ctx)
	//c.Assert(c.GetTestLog(), tc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionError(c *tc.C) {
	s.checkPermissionHandling(c, authFailureError, true)

	e := errors.Trace(authFailureError)
	s.checkPermissionHandling(c, e, true)

	e = errors.Annotatef(authFailureError, "more and more")
	s.checkPermissionHandling(c, e, true)
}

func (s *ErrorsSuite) TestHandleCredentialErrorAnotherError(c *tc.C) {
	s.checkPermissionHandling(c, errors.New("fluffy"), false)
}

func (s *ErrorsSuite) TestNilError(c *tc.C) {
	s.checkPermissionHandling(c, nil, false)
}

func (s *ErrorsSuite) checkPermissionHandling(c *tc.C, e error, handled bool) {
	isAuthF := func(e error) bool {
		return handled
	}
	ctx := context.NewEmptyCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, tc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	}

	denied := common.MaybeHandleCredentialError(isAuthF, e, ctx)
	c.Assert(called, tc.Equals, handled)
	c.Assert(denied, tc.Equals, handled)
}
