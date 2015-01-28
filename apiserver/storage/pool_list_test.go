// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/pool"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type poolSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	poolManager pool.PoolManager
	settings    pool.SettingsManager

	api        *storage.API
	authorizer testing.FakeAuthorizer
}

var _ = gc.Suite(&poolSuite{})

var poolAttrs = map[string]interface{}{
	"name": "testpool", "type": "loop", "foo": "bar",
}

func (s *poolSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.settings = state.NewStateSettings(s.State)
	s.poolManager = pool.NewPoolManager(s.settings)
	s.authorizer = testing.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.api, err = storage.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(storage.GetPoolManager, func(psm pool.SettingsManager) pool.PoolManager {
		return s.poolManager
	})
}

func (s *poolSuite) createSettings(c *gc.C) {
	err := s.settings.CreateSettings("pool#testpool", poolAttrs)
	c.Assert(err, jc.ErrorIsNil)
	// Create settings that isn't a pool.
	err = s.settings.CreateSettings("r#1", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *poolSuite) TestList(c *gc.C) {
	s.createSettings(c)
	pools, err := s.api.PoolList(params.PoolListFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	one := pools.Results[0]
	c.Assert(one.Error.Error, gc.IsNil)
	c.Assert(one.Result.Traits, gc.DeepEquals, poolAttrs)
	c.Assert(one.Result.Name, gc.Equals, "testpool")
	c.Assert(one.Result.Type, gc.Equals, "loop")
}

func (s *poolSuite) TestListManyResults(c *gc.C) {
	s.createSettings(c)
	err := s.settings.CreateSettings("pool#testpool2", map[string]interface{}{
		"name": "testpool2", "type": "loop", "foo2": "bar2",
	})
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.PoolList(params.PoolListFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 2)

	all := pools.Results
	poolCfgs := make(map[string]map[string]interface{})
	for _, p := range all {
		poolCfgs[p.Result.Name] = p.Result.Traits
	}
	c.Assert(poolCfgs, jc.DeepEquals, map[string]map[string]interface{}{
		"testpool":  {"name": "testpool", "type": "loop", "foo": "bar"},
		"testpool2": {"name": "testpool2", "type": "loop", "foo2": "bar2"},
	})
}

func (s *poolSuite) TestListByName(c *gc.C) {
	s.createSettings(c)
	tstName := "testpool2"
	err := s.settings.CreateSettings("pool#testpool2", map[string]interface{}{
		"name": tstName, "type": "loop", "foo2": "bar2",
	})
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.PoolList(params.PoolListFilter{Names: []string{tstName}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Result.Name, gc.DeepEquals, tstName)
}

func (s *poolSuite) TestListByType(c *gc.C) {
	s.createSettings(c)
	tstType := "rayofsunshine"
	err := s.settings.CreateSettings("pool#testpool2", map[string]interface{}{
		"name": "testpool2", "type": tstType, "foo2": "bar2",
	})
	c.Assert(err, jc.ErrorIsNil)
	pools, err := s.api.PoolList(params.PoolListFilter{Types: []string{tstType}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 1)
	c.Assert(pools.Results[0].Result.Type, gc.DeepEquals, tstType)
}

func (s *poolSuite) TestListNoPools(c *gc.C) {
	pools, err := s.api.PoolList(params.PoolListFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools.Results, gc.HasLen, 0)
}
