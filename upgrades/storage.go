// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
	"github.com/juju/juju/storage/provider"
)

var defaultLoopPools = map[string]map[string]interface{}{
	"loop": map[string]interface{}{},
}

var defaultEBSPools = map[string]map[string]interface{}{
	ec2storage.EBSPool:    map[string]interface{}{},
	ec2storage.EBSSSDPool: map[string]interface{}{"volume-type": "gp2"},
}

var ensureStorage = AddDefaultStoragePools

// TODO - only exported so we can call from machine agent pending agreeing
// on how to solve change in upgrade behaviour.
func AddDefaultStoragePools(st *state.State, agentConfig agent.Config) error {
	settings := state.NewStateSettings(st)
	pm := pool.NewPoolManager(settings)

	for name, attrs := range defaultEBSPools {
		if err := addDefaultPool(pm, name, ec2storage.EBSProviderType, attrs); err != nil {
			return err
		}
	}

	// Register the "rootfs" and "tmpfs" pool.
	if err := addDefaultPool(pm, provider.RootfsPool, provider.RootfsProviderType, map[string]interface{}{}); err != nil {
		return err
	}
	if err := addDefaultPool(pm, provider.TmpfsPool, provider.TmpfsProviderType, map[string]interface{}{}); err != nil {
		return err
	}

	// Register the default loop pool.
	//
	// TODO(axw) this is broken. The data-dir cannot be encoded in the config, because the loop
	// provider operates on different machines. It's not necessarily true that each machine
	// has the same data-dir path.
	cfg := map[string]interface{}{
		provider.LoopDataDir: filepath.Join(agentConfig.DataDir(), "storage", "block", "loop"),
	}
	return addDefaultPool(pm, "loop", provider.LoopProviderType, cfg)
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
