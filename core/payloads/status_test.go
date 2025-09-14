// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testing"
)

var (
	okayStates = []string{
		payloads.StateStarting,
		payloads.StateRunning,
		payloads.StateStopping,
		payloads.StateStopped,
	}
)

type statusSuite struct {
	testing.BaseSuite
}

func TestStatusSuite(t *tctesting.T) {
	tc.Run(t, &statusSuite{})
}

func (s *statusSuite) TestValidateStateOkay(c *tc.C) {
	for _, state := range okayStates {
		c.Logf("checking %q", state)
		err := payloads.ValidateState(state)

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *statusSuite) TestValidateStateUndefined(c *tc.C) {
	var state string
	err := payloads.ValidateState(state)

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *statusSuite) TestValidateStateBadState(c *tc.C) {
	state := "some bogus state"
	err := payloads.ValidateState(state)

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}
