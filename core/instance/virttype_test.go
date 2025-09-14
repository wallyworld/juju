// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	tctesting "testing"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
)

type VirtTypeSuite struct{}

func TestVirtTypeSuite(t *tctesting.T) {
	tc.Run(t, &VirtTypeSuite{})
}

func (s *VirtTypeSuite) TestParseVirtType(c *tc.C) {
	parseVirtTypeTests := []struct {
		arg   string
		value VirtType
		err   string
	}{{
		arg:   "",
		value: DefaultInstanceType,
	}, {
		arg:   "container",
		value: api.InstanceTypeContainer,
	}, {
		arg:   "virtual-machine",
		value: api.InstanceTypeVM,
	}, {
		arg: "foo",
		err: `LXD VirtType "foo" not valid`,
	}}
	for i, t := range parseVirtTypeTests {
		c.Logf("test %d: %s", i, t.arg)
		v, err := ParseVirtType(t.arg)
		if t.err == "" {
			c.Check(err, tc.ErrorIsNil)
			c.Check(v, tc.Equals, t.value)
		} else {
			c.Check(err, tc.ErrorMatches, t.err)
		}
	}
}

func (s *VirtTypeSuite) TestNormaliseVirtType(c *tc.C) {
	virtTypes := []struct {
		arg      VirtType
		expected VirtType
	}{{
		arg:      api.InstanceTypeAny,
		expected: api.InstanceTypeContainer,
	}, {
		arg:      api.InstanceTypeContainer,
		expected: api.InstanceTypeContainer,
	}, {
		arg:      api.InstanceTypeVM,
		expected: api.InstanceTypeVM,
	}}
	for i, t := range virtTypes {
		c.Logf("test %d: %s", i, t.arg)
		v := NormaliseVirtType(t.arg)
		c.Check(v, tc.Equals, t.expected)
	}
}
