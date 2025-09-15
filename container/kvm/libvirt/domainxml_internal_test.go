// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

// gocheck boilerplate.
type domainXMLInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestDomainXMLInternalSuite(t *tctesting.T) {
	tc.Run(t, &domainXMLInternalSuite{})
}

func (domainXMLInternalSuite) TestDeviceID(c *tc.C) {
	table := []struct {
		in     int
		want   string
		errMsg string
	}{
		{0, "vda", ""},
		{4, "vde", ""},
		{15, "vdp", ""},
		{25, "vdz", ""},
		{-1, "", "got -1 but only support devices 0-25"},
		{26, "", "got 26 but only support devices 0-25"},
		{120, "", "got 120 but only support devices 0-25"},
	}
	for i, test := range table {
		c.Logf("test %d for input %d", i+1, test.in)
		got, err := deviceID(test.in)
		if err != nil {
			c.Check(err, tc.ErrorMatches, test.errMsg)
			c.Check(got, tc.Equals, "")
			continue
		}
		c.Check(got, tc.Equals, test.want)
		c.Check(err, tc.ErrorIsNil)
	}
}
