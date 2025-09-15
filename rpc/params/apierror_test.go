// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

type errorSuite struct{}

var _ rpc.ErrorCoder = (*params.Error)(nil)

func TestErrorSuite(t *tctesting.T) {
	tc.Run(t, &errorSuite{})
}

func (*errorSuite) TestErrCode(c *tc.C) {
	var err error
	err = &params.Error{Code: params.CodeDead, Message: "brain dead test"}
	c.Check(params.ErrCode(err), tc.Equals, params.CodeDead)

	err = errors.Trace(err)
	c.Check(params.ErrCode(err), tc.Equals, params.CodeDead)
}

func (*errorSuite) TestTranslateWellKnownError(c *tc.C) {
	var tests = []struct {
		name string
		err  params.Error
		test func(err error) bool
	}{
		{params.CodeNotFound, params.Error{Code: params.CodeNotFound, Message: "look a NotFound error"}, errors.IsNotFound},
		{params.CodeUserNotFound, params.Error{Code: params.CodeUserNotFound, Message: "look a UserNotFound error"}, errors.IsUserNotFound},
		{params.CodeUnauthorized, params.Error{Code: params.CodeUnauthorized, Message: "look a Unauthorized error"}, errors.IsUnauthorized},
		{params.CodeNotImplemented, params.Error{Code: params.CodeNotImplemented, Message: "look a NotImplemented error"}, errors.IsNotImplemented},
		{params.CodeAlreadyExists, params.Error{Code: params.CodeAlreadyExists, Message: "look a AlreadyExists error"}, errors.IsAlreadyExists},
		{params.CodeNotSupported, params.Error{Code: params.CodeNotSupported, Message: "look a NotSupported error"}, errors.IsNotSupported},
		{params.CodeNotValid, params.Error{Code: params.CodeNotValid, Message: "look a NotValid error"}, errors.IsNotValid},
		{params.CodeNotProvisioned, params.Error{Code: params.CodeNotProvisioned, Message: "look a NotProvisioned error"}, errors.IsNotProvisioned},
		{params.CodeNotAssigned, params.Error{Code: params.CodeNotAssigned, Message: "look a NotAssigned error"}, errors.IsNotAssigned},
		{params.CodeBadRequest, params.Error{Code: params.CodeBadRequest, Message: "look a BadRequest error"}, errors.IsBadRequest},
		{params.CodeMethodNotAllowed, params.Error{Code: params.CodeMethodNotAllowed, Message: "look a MethodNotAllowed error"}, errors.IsMethodNotAllowed},
		{params.CodeForbidden, params.Error{Code: params.CodeForbidden, Message: "look a Forbidden error"}, errors.IsForbidden},
		{params.CodeQuotaLimitExceeded, params.Error{Code: params.CodeQuotaLimitExceeded, Message: "look a QuotaLimitExceeded error"}, errors.IsQuotaLimitExceeded},
		{params.CodeNotYetAvailable, params.Error{Code: params.CodeNotYetAvailable, Message: "look a NotYetAvailable error"}, errors.IsNotYetAvailable},
	}

	for _, v := range tests {
		c.Assert(v.test(v.err), tc.IsFalse, tc.Commentf("test %s: params error is not a juju/errors error", v.name))
		c.Assert(v.test(params.TranslateWellKnownError(v.err)), tc.IsTrue, tc.Commentf("test %s: translated error is a juju/errors error", v.name))
	}
}
