// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/charmhub/transport"
)

type ErrorsSuite struct{}

func TestErrorsSuite(t *tctesting.T) {
	tc.Run(t, &ErrorsSuite{})
}

func (ErrorsSuite) TestHandleBasicAPIErrors(c *tc.C) {
	var list transport.APIErrors
	err := handleBasicAPIErrors(list, &FakeLogger{})
	c.Assert(err, tc.ErrorIsNil)
}

func (ErrorsSuite) TestHandleBasicAPIErrorsNotFound(c *tc.C) {
	list := transport.APIErrors{{Code: transport.ErrorCodeNotFound, Message: "foo"}}
	err := handleBasicAPIErrors(list, &FakeLogger{})
	c.Assert(err, tc.ErrorMatches, `charm or bundle not found`)
}

func (ErrorsSuite) TestHandleBasicAPIErrorsOther(c *tc.C) {
	list := transport.APIErrors{{Code: "other", Message: "foo"}}
	err := handleBasicAPIErrors(list, &FakeLogger{})
	c.Assert(err, tc.ErrorMatches, `foo`)
}
