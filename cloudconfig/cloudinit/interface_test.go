// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	tctesting "testing"

	"github.com/juju/tc"
)

var _ CloudConfig = (*ubuntuCloudConfig)(nil)
var _ CloudConfig = (*centOSCloudConfig)(nil)

type InterfaceSuite struct{}

func TestNetworkUbuntuSuite(t *tctesting.T) {
	tc.Run(t, InterfaceSuite{})
}

func (HelperSuite) TestNewCloudConfigWithoutMACMatch(c *tc.C) {
	cfg, err := New("ubuntu", WithNetplanMACMatch(true))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.(*ubuntuCloudConfig).useNetplanHWAddrMatch, tc.IsTrue)
}
