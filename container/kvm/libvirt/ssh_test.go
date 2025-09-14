// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt

import (
	tctesting "testing"

	"github.com/juju/tc"
)

// libvirtSSHSuite is gocheck boilerplate
type libvirtSSHSuite struct{}

// gocheck boilerplate
func TestNetworkUbuntuSuite(t *tctesting.T) {
	tc.Run(t, libvirtSSHSuite{})
}

func (libvirtSSHSuite) TestKeepTheImports(c *tc.C) {
	c.Assert(true, tc.IsTrue)
}
