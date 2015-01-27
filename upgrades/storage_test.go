// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/pool"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/upgrades"
)

type defaultStoragePoolsSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

//TODO - better tests
func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	err := upgrades.AddDefaultStoragePools(s.State, &mockAgentConfig{dataDir: s.DataDir()})
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
	p, err = pm.Get("loop")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Name(), gc.Equals, "loop")
	c.Assert(p.Type(), gc.Equals, provider.LoopProviderType)
}
