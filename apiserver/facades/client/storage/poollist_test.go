// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	apiserverstorage "github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type poolSuite struct {
	baseStorageSuite
}

func TestPoolSuite(t *tctesting.T) {
	tc.Run(t, &poolSuite{})
}

const (
	tstName = "testpool"
)

func (s *poolSuite) createPools(c *tc.C, num int) {
	var err error
	for i := 0; i < num; i++ {
		poolName := fmt.Sprintf("%v%v", tstName, i)
		s.baseStorageSuite.pools[poolName], err =
			storage.NewConfig(poolName, provider.LoopProviderType, nil)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *poolSuite) TestEnsureStoragePoolFilter(c *tc.C) {
	filter := params.StoragePoolFilter{}
	c.Assert(filter.Providers, tc.HasLen, 0)
	c.Assert(apiserverstorage.EnsureStoragePoolFilter(s.apiCaas, filter).Providers, tc.DeepEquals, []string{"kubernetes"})
}

func (s *poolSuite) TestList(c *tc.C) {
	s.createPools(c, 1)
	results, err := s.api.ListPools(params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	one := results.Results[0]
	c.Assert(one.Error, tc.IsNil)
	c.Assert(one.Result, tc.HasLen, 1)
	c.Assert(one.Result[0].Name, tc.Equals, fmt.Sprintf("%v%v", tstName, 0))
	c.Assert(one.Result[0].Provider, tc.Equals, string(provider.LoopProviderType))
}

func (s *poolSuite) TestListManyResults(c *tc.C) {
	s.registry.Providers["static"] = nil
	s.createPools(c, 2)
	results, err := s.api.ListPools(params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1", "static")
}

func (s *poolSuite) TestListByName(c *tc.C) {
	s.createPools(c, 2)
	tstName := fmt.Sprintf("%v%v", tstName, 1)

	results, err := s.api.ListPools(params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Names: []string{tstName},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Result, tc.HasLen, 1)
	c.Assert(results.Results[0].Result[0].Name, tc.DeepEquals, tstName)
}

func (s *poolSuite) TestListByType(c *tc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.ListPools(params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "rayofsunshine", "tmpfs")
}

func (s *poolSuite) TestListByNameAndTypeAnd(c *tc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.ListPools(params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType},
			Names:     []string{poolName},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Result, tc.HasLen, 1)
	c.Assert(results.Results[0].Result[0].Provider, tc.DeepEquals, tstType)
	c.Assert(results.Results[0].Result[0].Name, tc.DeepEquals, poolName)
}

func (s *poolSuite) TestListByNamesOr(c *tc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.ListPools(params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Names: []string{
				fmt.Sprintf("%v%v", tstName, 1),
				fmt.Sprintf("%v%v", tstName, 0),
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1")
}

func assertPoolNames(c *tc.C, results []params.StoragePool, expected ...string) {
	expectedNames := set.NewStrings(expected...)
	c.Assert(len(expectedNames), tc.Equals, len(results))
	for _, one := range results {
		c.Assert(expectedNames.Contains(one.Name), tc.IsTrue)
	}
}

func (s *poolSuite) TestListByTypesOr(c *tc.C) {
	s.createPools(c, 2)
	s.registerProviders(c)
	tstType := string(provider.TmpfsProviderType)
	poolName := "rayofsunshine"
	var err error
	s.baseStorageSuite.pools[poolName], err =
		storage.NewConfig(poolName, provider.TmpfsProviderType, nil)
	c.Assert(err, tc.ErrorIsNil)
	results, err := s.api.ListPools(params.StoragePoolFilters{
		[]params.StoragePoolFilter{{
			Providers: []string{tstType, string(provider.LoopProviderType)},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertPoolNames(c, results.Results[0].Result, "testpool0", "testpool1", "rayofsunshine", "loop", "tmpfs")
}

func (s *poolSuite) TestListNoPools(c *tc.C) {
	s.registry.Providers["static"] = nil
	results, err := s.api.ListPools(params.StoragePoolFilters{[]params.StoragePoolFilter{{}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	assertPoolNames(c, results.Results[0].Result, "static")
}

func (s *poolSuite) TestListFilterEmpty(c *tc.C) {
	err := apiserverstorage.ValidatePoolListFilter(s.api, s.registry, params.StoragePoolFilter{})
	c.Assert(err, tc.ErrorIsNil)
}

const (
	validProvider   = string(provider.LoopProviderType)
	invalidProvider = "invalid"
	validName       = "pool"
	invalidName     = "7ool"
)

func (s *poolSuite) TestListFilterValidProviders(c *tc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{validProvider})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterUnregisteredProvider(c *tc.C) {
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{validProvider})
	c.Assert(err, tc.ErrorMatches, `storage provider "loop" not found`)
}

func (s *poolSuite) TestListFilterUnknownProvider(c *tc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidateProviderCriteria(
		s.api,
		s.registry,
		[]string{invalidProvider})
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) TestListFilterValidNames(c *tc.C) {
	err := apiserverstorage.ValidateNameCriteria(
		s.api,
		[]string{validName})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterInvalidNames(c *tc.C) {
	err := apiserverstorage.ValidateNameCriteria(
		s.api,
		[]string{invalidName})
	c.Assert(err, tc.ErrorMatches, ".*not valid.*")
}

func (s *poolSuite) TestListFilterValidProvidersAndNames(c *tc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{validName}})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *poolSuite) TestListFilterValidProvidersAndInvalidNames(c *tc.C) {
	s.registerProviders(c)
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{validProvider},
			Names:     []string{invalidName}})
	c.Assert(err, tc.ErrorMatches, ".*not valid.*")
}

func (s *poolSuite) TestListFilterInvalidProvidersAndValidNames(c *tc.C) {
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{validName}})
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) TestListFilterInvalidProvidersAndNames(c *tc.C) {
	err := apiserverstorage.ValidatePoolListFilter(
		s.api,
		s.registry,
		params.StoragePoolFilter{
			Providers: []string{invalidProvider},
			Names:     []string{invalidName}})
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *poolSuite) registerProviders(c *tc.C) {
	common := provider.CommonStorageProviders()
	providerTypes, err := common.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	for _, providerType := range providerTypes {
		p, err := common.StorageProvider(providerType)
		c.Assert(err, tc.ErrorIsNil)
		s.registry.Providers[providerType] = p
	}
}
