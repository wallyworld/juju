// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type poolRemoveSuite struct {
	baseStorageSuite
}

func TestPoolRemoveSuite(t *tctesting.T) {
	tc.Run(t, &poolRemoveSuite{})
}

func (s *poolRemoveSuite) createPools(c *tc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, map[string]interface{}{"zip": "zap"})
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *poolRemoveSuite) TestRemovePool(c *tc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemoveNotExists(c *tc.C) {
	poolName := fmt.Sprintf("%v%v", tstName, 0)

	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemoveInUse(c *tc.C) {
	s.createPools(c, 1)
	poolName := fmt.Sprintf("%v%v", tstName, 0)
	s.poolsInUse = []string{poolName}
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolName,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolName))

	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
}

func (s *poolRemoveSuite) TestRemoveSomeInUse(c *tc.C) {
	s.createPools(c, 2)
	poolNameInUse := fmt.Sprintf("%v%v", tstName, 0)
	poolNameNotInUse := fmt.Sprintf("%v%v", tstName, 1)
	s.poolsInUse = []string{poolNameInUse}
	args := params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{
			Name: poolNameInUse,
		}, {
			Name: poolNameNotInUse,
		}},
	}
	results, err := s.api.RemovePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("storage pool %q in use", poolNameInUse))
	c.Assert(results.Results[1].Error, tc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
}
