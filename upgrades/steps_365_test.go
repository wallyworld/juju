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

var v365 = version.MustParse("3.6.5")

type steps365Suite struct {
	testing.BaseSuite
}

func TestSteps365Suite(t *tctesting.T) {
	tc.Run(t, &steps365Suite{})
}

func (s *steps365Suite) TestSplitMigrationStatusMessages(c *tc.C) {
	step := findStateStep(c, v365, "split migration status messages")
	c.Assert(step.Targets(), tc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
