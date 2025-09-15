// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
)

type defaultStoragePoolsSuite struct {
	testhelpers.IsolationSuite
}

func TestDefaultStoragePoolsSuite(t *tctesting.T) {
	tc.Run(t, &defaultStoragePoolsSuite{})
}

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *tc.C) {
	p1, err := storage.NewConfig("pool1", storage.ProviderType("whatever"), map[string]interface{}{"1": "2"})
	c.Assert(err, tc.ErrorIsNil)
	p2, err := storage.NewConfig("pool2", storage.ProviderType("whatever"), map[string]interface{}{"3": "4"})
	c.Assert(err, tc.ErrorIsNil)
	provider := &dummystorage.StorageProvider{
		DefaultPools_: []*storage.Config{p1, p2},
	}

	settings := poolmanager.MemSettings{make(map[string]map[string]interface{})}
	pm := poolmanager.New(settings, storage.StaticProviderRegistry{
		map[storage.ProviderType]storage.Provider{"whatever": provider},
	})

	err = poolmanager.AddDefaultStoragePools(provider, pm)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(settings.Settings, tc.DeepEquals, map[string]map[string]interface{}{
		"pool#pool1": {"1": "2", "name": "pool1", "type": "whatever"},
		"pool#pool2": {"3": "4", "name": "pool2", "type": "whatever"},
	})
}
