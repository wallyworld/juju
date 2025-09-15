// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/rpc/params"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type poolCreateSuite struct {
	baseStorageSuite
}

func TestPoolCreateSuite(t *tctesting.T) {
	tc.Run(t, &poolCreateSuite{})
}

func (s *poolCreateSuite) TestCreatePool(c *tc.C) {
	const (
		pname = "pname"
		ptype = string(provider.LoopProviderType)
	)
	expected, _ := jujustorage.NewConfig(pname, provider.LoopProviderType, nil)

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:     pname,
			Provider: ptype,
			Attrs:    nil,
		}},
	}
	results, err := s.api.CreatePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0], tc.DeepEquals, expected)
}

func (s *poolCreateSuite) TestCreatePoolError(c *tc.C) {
	msg := "as expected"
	s.baseStorageSuite.poolManager.createPool = func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
		return nil, errors.New(msg)
	}

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: "doesnt-matter",
		}},
	}
	results, err := s.api.CreatePool(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.DeepEquals, &params.Error{
		Message: "as expected",
	})
}
