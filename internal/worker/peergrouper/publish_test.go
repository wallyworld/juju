// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
)

type publishSuite struct {
	testing.BaseSuite
}

func TestPublishSuite(t *tctesting.T) {
	tc.Run(t, &publishSuite{})
}

type mockAPIHostPortsSetter struct {
	calls        int
	apiHostPorts []network.SpaceHostPorts
}

func (s *mockAPIHostPortsSetter) SetAPIHostPorts(apiHostPorts []network.SpaceHostPorts) error {
	s.calls++
	s.apiHostPorts = apiHostPorts
	return nil
}

func (s *publishSuite) TestPublisherSetsAPIHostPortsOnce(c *tc.C) {
	var mock mockAPIHostPortsSetter
	statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}

	hostPorts1 := network.NewSpaceHostPorts(1234, "testing1.invalid", "127.0.0.1")
	hostPorts2 := network.NewSpaceHostPorts(1234, "testing2.invalid", "127.0.0.2")

	// statePublish.SetAPIHostPorts should not update state a second time.
	apiServers := []network.SpaceHostPorts{hostPorts1}
	for i := 0; i < 2; i++ {
		err := statePublish.SetAPIHostPorts(apiServers)
		c.Assert(err, tc.ErrorIsNil)
	}

	c.Assert(mock.calls, tc.Equals, 1)
	c.Assert(mock.apiHostPorts, tc.DeepEquals, apiServers)

	apiServers = append(apiServers, hostPorts2)
	for i := 0; i < 2; i++ {
		err := statePublish.SetAPIHostPorts(apiServers)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(mock.calls, tc.Equals, 2)
	c.Assert(mock.apiHostPorts, tc.DeepEquals, apiServers)
}

func (s *publishSuite) TestPublisherSortsHostPorts(c *tc.C) {
	ipV4First := network.NewSpaceHostPorts(1234, "testing1.invalid", "127.0.0.1", "::1")
	ipV6First := network.NewSpaceHostPorts(1234, "testing1.invalid", "::1", "127.0.0.1")

	check := func(publish, expect []network.SpaceHostPort) {
		var mock mockAPIHostPortsSetter
		statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}
		for i := 0; i < 2; i++ {
			err := statePublish.SetAPIHostPorts([]network.SpaceHostPorts{publish})
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Assert(mock.calls, tc.Equals, 1)
		c.Assert(mock.apiHostPorts, tc.DeepEquals, []network.SpaceHostPorts{expect})
	}

	check(ipV6First, ipV4First)
	check(ipV4First, ipV4First)
}

func (s *publishSuite) TestPublisherRejectsNoServers(c *tc.C) {
	var mock mockAPIHostPortsSetter
	statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}
	err := statePublish.SetAPIHostPorts(nil)
	c.Assert(err, tc.ErrorMatches, "no API servers specified")
}
