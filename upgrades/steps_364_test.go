// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/upgrades"
)

var v364 = version.MustParse("3.6.4")

type steps364Suite struct {
	testing.BaseSuite
}

func TestSteps364Suite(t *tctesting.T) {
	tc.Run(t, &steps364Suite{})
}

func (s *steps364Suite) TestAddsVirtualHostKeys(c *tc.C) {
	step := findStateStep(c, v364, "add virtual host keys")
	c.Assert(step.Targets(), tc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
