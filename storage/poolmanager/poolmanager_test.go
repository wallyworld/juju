// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
)

type poolSuite struct {
	// TODO - don't use state directly, mock it out and add feature tests.
	statetesting.StateSuite
	registry    storage.StaticProviderRegistry
	poolManager poolmanager.PoolManager
	settings    poolmanager.SettingsManager
}

func TestPoolSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &poolSuite{})
}

var poolAttrs = map[string]interface{}{
	"name": "testpool", "type": "loop", "foo": "bar", "bleep": "bloop",
}

func (s *poolSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.settings = state.NewStateSettings(s.State)
	s.registry = storage.StaticProviderRegistry{
		map[storage.ProviderType]storage.Provider{
			"loop": &dummystorage.StorageProvider{},
		},
	}
	s.poolManager = poolmanager.New(s.settings, s.registry)
}

func (s *poolSuite) createSettings(c *tc.C) {
	err := s.settings.CreateSettings("pool#testpool", poolAttrs)
	c.Assert(err, tc.ErrorIsNil)
	// Create settings that isn't a pool.
	err = s.settings.CreateSettings("r#1", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *poolSuite) TestList(c *tc.C) {
	s.createSettings(c)
	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0].Attrs(), tc.DeepEquals, storage.Attrs{"foo": "bar", "bleep": "bloop"})
	c.Assert(pools[0].Name(), tc.Equals, "testpool")
	c.Assert(pools[0].Provider(), tc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestListManyResults(c *tc.C) {
	s.createSettings(c)
	err := s.settings.CreateSettings("pool#testpool2", map[string]interface{}{
		"name": "testpool2", "type": "loop", "foo2": "bar2",
	})
	c.Assert(err, tc.ErrorIsNil)
	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 2)
	poolCfgs := make(map[string]map[string]interface{})
	for _, p := range pools {
		poolCfgs[p.Name()] = p.Attrs()
	}
	c.Assert(poolCfgs, tc.DeepEquals, map[string]map[string]interface{}{
		"testpool":  {"foo": "bar", "bleep": "bloop"},
		"testpool2": {"foo2": "bar2"},
	})
}

func (s *poolSuite) TestListNoPools(c *tc.C) {
	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 0)
}

func (s *poolSuite) TestPool(c *tc.C) {
	s.createSettings(c)
	p, err := s.poolManager.Get("testpool")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Attrs(), tc.DeepEquals, storage.Attrs{"foo": "bar", "bleep": "bloop"})
	c.Assert(p.Name(), tc.Equals, "testpool")
	c.Assert(p.Provider(), tc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreate(c *tc.C) {
	created, err := s.poolManager.Create("testpool", storage.ProviderType("loop"), storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	p, err := s.poolManager.Get("testpool")
	c.Assert(created, tc.DeepEquals, p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Attrs(), tc.DeepEquals, storage.Attrs{"foo": "bar"})
	c.Assert(p.Name(), tc.Equals, "testpool")
	c.Assert(p.Provider(), tc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestCreateAlreadyExists(c *tc.C) {
	_, err := s.poolManager.Create("testpool", storage.ProviderType("loop"), storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.poolManager.Create("testpool", storage.ProviderType("loop"), storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, ".*cannot overwrite.*")
}

func (s *poolSuite) TestCreateMissingName(c *tc.C) {
	_, err := s.poolManager.Create("", "loop", storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "pool name is missing")
}

func (s *poolSuite) TestCreateMissingType(c *tc.C) {
	_, err := s.poolManager.Create("testpool", "", storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "provider type is missing")
}

func (s *poolSuite) TestCreateInvalidConfig(c *tc.C) {
	s.registry.Providers["invalid"] = &dummystorage.StorageProvider{
		ValidateConfigFunc: func(*storage.Config) error {
			return errors.New("no good")
		},
	}
	_, err := s.poolManager.Create("testpool", "invalid", nil)
	c.Assert(err, tc.ErrorMatches, "validating storage provider config: no good")
}

func (s *poolSuite) TestReplace(c *tc.C) {
	s.createSettings(c)
	err := s.poolManager.Replace("testpool", "", storage.Attrs{"zip": "zap"})
	c.Assert(err, tc.ErrorIsNil)
	p, err := s.poolManager.Get("testpool")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Attrs(), tc.DeepEquals, storage.Attrs{"zip": "zap"})
	c.Assert(p.Name(), tc.Equals, "testpool")
	c.Assert(p.Provider(), tc.Equals, storage.ProviderType("loop"))
}

func (s *poolSuite) TestReplaceMissingName(c *tc.C) {
	err := s.poolManager.Replace("", "", storage.Attrs{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "pool name is missing")
}

func (s *poolSuite) TestReplaceNewProvider(c *tc.C) {
	s.registry.Providers["notebook"] = &dummystorage.StorageProvider{}
	s.createSettings(c)
	err := s.poolManager.Replace("testpool", "notebook", storage.Attrs{"handwritten": "true"})
	c.Assert(err, tc.ErrorIsNil)
	p, err := s.poolManager.Get("testpool")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Attrs(), tc.DeepEquals, storage.Attrs{"handwritten": "true"})
	c.Assert(p.Name(), tc.Equals, "testpool")
	c.Assert(p.Provider(), tc.Equals, storage.ProviderType("notebook"))
}

func (s *poolSuite) TestReplaceInvalidConfig(c *tc.C) {
	s.registry.Providers["invalid"] = &dummystorage.StorageProvider{
		ValidateConfigFunc: func(*storage.Config) error {
			return errors.New("no good")
		},
	}
	s.createSettings(c)
	err := s.poolManager.Replace("testpool", "invalid", storage.Attrs{"zip": "zap"})
	c.Assert(err, tc.ErrorMatches, "validating storage provider config: no good")
}

func (s *poolSuite) TestReplaceNotFound(c *tc.C) {
	err := s.poolManager.Replace("deadpool", "", storage.Attrs{"zip": "zap"})
	c.Assert(err, tc.ErrorMatches, "pool \"deadpool\" not found")
}

func (s *poolSuite) TestDelete(c *tc.C) {
	s.createSettings(c)
	err := s.poolManager.Delete("testpool")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.poolManager.Get("testpool")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = s.poolManager.Delete("testpool")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}
