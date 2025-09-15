// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

type lxdStateCharmProfilerSuite struct{}

func TestLxdStateCharmProfilerSuite(t *tctesting.T) {
	tc.Run(t, &lxdStateCharmProfilerSuite{})
}

func (*lxdStateCharmProfilerSuite) TestLXDProfileEmptyCharm(c *tc.C) {
	wrapper := lxdStateCharmProfiler{
		Charm: nil,
	}
	c.Check(wrapper.LXDProfile(), tc.IsNil)
}

func (*lxdStateCharmProfilerSuite) TestLXDProfileCharmNoProfile(c *tc.C) {
	wrapper := lxdStateCharmProfiler{
		Charm: &state.Charm{},
	}
	c.Check(wrapper.LXDProfile(), tc.IsNil)
}
