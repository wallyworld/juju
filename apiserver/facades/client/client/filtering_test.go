// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/core/network"
)

type filteringUnitTests struct {
}

func TestFilteringUnitTests(t *tctesting.T) {
	tc.Run(t, &filteringUnitTests{})
}

func (f *filteringUnitTests) TestMatchPortRanges(c *tc.C) {

	match, ok, err := client.MatchPortRanges([]string{"80/tcp"}, network.PortRange{80, 80, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"80-90/tcp"}, network.PortRange{80, 90, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"90/tcp"}, network.PortRange{80, 90, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"70"}, network.PortRange{7070, 7070, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070"}, network.PortRange{7070, 7070, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/udp"}, network.PortRange{7070, 7070, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/tcp"}, network.PortRange{7065, 7069, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/tcp"}, network.PortRange{7069, 7071, "tcp"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)
}

func (s *filteringUnitTests) TestMatchSubnet(c *tc.C) {

	// We do not resolve hostnames.
	match, ok, err := client.MatchSubnet([]string{"localhost"}, "127.0.0.1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchSubnet([]string{"127.0.0.1"}, "127.0.0.1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)

	match, ok, err = client.MatchSubnet([]string{"localhost"}, "10.0.0.1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
	c.Check(match, tc.IsFalse)

	match, ok, err = client.MatchSubnet([]string{"testing.local"}, "testing.local")
	c.Check(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)
	c.Check(match, tc.IsTrue)
}
