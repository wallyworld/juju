// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/upgrades/upgradevalidation"
)

type versionSuite struct {
	testhelpers.IsolationSuite
}

func TestVersionSuite(t *tctesting.T) {
	tc.Run(t, &versionSuite{})
}

type versionCheckTC struct {
	from    string
	to      string
	allowed bool
	minVers string
	err     string
}

func (s *versionSuite) TestUpgradeControllerAllowed(c *tc.C) {
	for i, t := range []versionCheckTC{
		{
			from:    "2.8.0",
			to:      "3.0.0",
			allowed: false,
			minVers: "2.9.36",
		}, {
			from:    "2.9.65",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.36",
		}, {
			from:    "2.9.37",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.36",
		}, {
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `upgrading controller to "4.0.0" is not supported from "2.9.0"`,
		}, {
			from:    "3.0.0",
			to:      "2.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		},
	} {
		s.assertUpgradeControllerAllowed(c, i, t)
	}
}

func (s *versionSuite) assertUpgradeControllerAllowed(c *tc.C, i int, t versionCheckTC) {
	c.Logf("testing %d", i)

	restore := testhelpers.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.36"),
	})
	defer restore()

	from := version.MustParse(t.from)
	to := version.MustParse(t.to)
	minVers := version.MustParse(t.minVers)
	allowed, vers, err := upgradevalidation.UpgradeControllerAllowed(from, to)
	c.Check(allowed, tc.Equals, t.allowed)
	c.Check(vers, tc.DeepEquals, minVers)
	if t.err == "" {
		c.Check(err, tc.ErrorIsNil)
	} else {
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *versionSuite) TestMigrateToAllowed(c *tc.C) {
	for i, t := range []versionCheckTC{
		{
			from:    "2.8.0",
			to:      "3.0.0",
			allowed: false,
			minVers: "2.9.43",
		}, {
			from:    "2.9.43",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.43",
		}, {
			from:    "2.9.44",
			to:      "3.0.0",
			allowed: true,
			minVers: "2.9.43",
		},
		{
			from:    "2.9.0",
			to:      "4.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `migrate to "4.0.0" is not supported from "2.9.0"`,
		},
		{
			from:    "3.0.0",
			to:      "2.0.0",
			allowed: false,
			minVers: "0.0.0",
			err:     `downgrade is not allowed`,
		},
	} {
		s.assertMigrateToAllowed(c, i, t)
	}
}

func (s *versionSuite) assertMigrateToAllowed(c *tc.C, i int, t versionCheckTC) {
	c.Logf("testing %d", i)
	from := version.MustParse(t.from)
	to := version.MustParse(t.to)
	minVers := version.MustParse(t.minVers)
	allowed, vers, err := upgradevalidation.MigrateToAllowed(from, to)
	c.Check(allowed, tc.Equals, t.allowed)
	c.Check(vers, tc.DeepEquals, minVers)
	if t.err == "" {
		c.Check(err, tc.ErrorIsNil)
	} else {
		c.Check(err, tc.ErrorMatches, t.err)
	}
}
