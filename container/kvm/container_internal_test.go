// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	tctesting "testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type containerInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestContainerInternalSuite(t *tctesting.T) {
	tc.Run(t, &containerInternalSuite{})
}

func (containerInternalSuite) TestInterfaceInfo(c *tc.C) {
	i := interfaceInfo{config: corenetwork.InterfaceInfo{
		MACAddress: "mac", ParentInterfaceName: "piname", InterfaceName: "iname"}}
	c.Check(i.InterfaceName(), tc.Equals, "iname")
	c.Check(i.ParentInterfaceName(), tc.Equals, "piname")
	c.Assert(i.MACAddress(), tc.Equals, "mac")
}
