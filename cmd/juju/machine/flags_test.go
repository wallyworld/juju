// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/storage"
)

type FlagsSuite struct {
	testing.BaseSuite
}

func TestFlagsSuite(t *tctesting.T) {
	tc.Run(t, &FlagsSuite{})
}

func (*FlagsSuite) TestDisksFlagErrors(c *tc.C) {
	var disks []storage.Constraints
	f := machine.NewDisksFlag(&disks)
	err := f.Set("-1")
	c.Assert(err, tc.ErrorMatches, `cannot parse disk constraints: cannot parse count: count must be greater than zero, got "-1"`)
	c.Assert(disks, tc.HasLen, 0)
}

func (*FlagsSuite) TestDisksFlag(c *tc.C) {
	var disks []storage.Constraints
	f := machine.NewDisksFlag(&disks)
	err := f.Set("crystal,1G")
	c.Assert(err, tc.ErrorIsNil)
	err = f.Set("2,2G")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(disks, tc.DeepEquals, []storage.Constraints{
		{Pool: "crystal", Size: 1024, Count: 1},
		{Size: 2048, Count: 2},
	})
}
