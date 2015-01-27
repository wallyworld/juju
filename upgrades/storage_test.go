// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/pool"
	"github.com/juju/juju/upgrades"
)

type defaultStoragePoolsSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

//TODO - better tests
func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	err := upgrades.AddDefaultStoragePools(s.State)
	c.Assert(err, jc.ErrorIsNil)
	settings := state.NewStateSettings(s.State)
	pm := pool.NewPoolManager(settings)
	p, err := pm.Get("ebs")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Name(), gc.Equals, "ebs")
	c.Assert(p.Type(), gc.Equals, ec2storage.EBSProviderType)
	p, err = pm.Get("ebs-ssd")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Name(), gc.Equals, "ebs-ssd")
	c.Assert(p.Type(), gc.Equals, ec2storage.EBSProviderType)
}
