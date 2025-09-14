// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

type environNetworkSuite struct {
	baseEnvironSuite
}

func TestEnvironNetworkSuite(t *tctesting.T) {
	tc.Run(t, &environNetworkSuite{})
}

func (s *environNetworkSuite) TestSupportsSpaces(c *tc.C) {
	netEnv, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, tc.IsTrue)

	spaceSupport, err := netEnv.SupportsSpaces(s.callCtx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaceSupport, tc.IsTrue)
}
