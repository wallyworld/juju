// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
)

var defaultEBSPools = map[string]map[string]interface{}{
	"ebs":     map[string]interface{}{},
	"ebs-ssd": map[string]interface{}{"volume-type": "gp2"},
}

func addDefaultStoragePools(st *state.State) error {
	settings := state.NewStateSettings(st)
	pm := pool.NewPoolManager(settings)

	for name, attrs := range defaultEBSPools {
		if err := addDefaultPool(pm, name, ec2storage.EBSProviderType, attrs); err != nil {
			return err
		}
	}
	return nil
}

func addDefaultPool(pm pool.PoolManager, name string, providerType storage.ProviderType, attrs map[string]interface{}) error {
	_, err := pm.Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotatef(err, "loading default pool %q", name)
	}
	if err != nil {
		// We got a not found error, so default pool doesn't exist.
		if _, err := pm.Create(name, providerType, attrs); err != nil {
			return errors.Annotatef(err, "creating default pool %q", name)
		}
	}
	return nil
}
