// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type poolCreateSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	poolManager pool.PoolManager
	settings    pool.SettingsManager

	api        *storage.API
	authorizer testing.FakeAuthorizer
}

var _ = gc.Suite(&poolCreateSuite{})

func (s *poolCreateSuite) SetUpTest(c *gc.C) {
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
	s.PatchValue(storage.CheckProviderTypeSupported, func(a *storage.API, providerT jujustorage.ProviderType) error {
		return nil
	})
}

func (s *poolCreateSuite) TestCreatePool(c *gc.C) {
	pname := "pname"
	ptype := "ptype"
	pcfg := map[string]interface{}{"just": "checking"}

	err := s.api.CreatePool(params.StoragePool{
		Name:   pname,
		Type:   ptype,
		Config: pcfg,
	})
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	one := pools[0]
	c.Assert(one.Name(), gc.Equals, pname)
	c.Assert(one.Type(), gc.DeepEquals, jujustorage.ProviderType(ptype))
}
