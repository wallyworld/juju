// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/environs/config"
	corestorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.storage.provisioner")

var _ Provisioner = (*storageProvisioner)(nil)

// Provisioner represents a running storage provisioner worker.
type Provisioner interface {
	worker.Worker

	// Create makes the specified volumes using a storage provider of a given type.
	Create(*config.Config, *corestorage.Config, corestorage.ProviderType, []corestorage.VolumeParams) ([]corestorage.BlockDevice, error)
}

type storageProvisioner struct {
	api         *environment.Facade
	machine     *provisioner.Machine
	agentConfig agent.Config
	tomb        tomb.Tomb
}

// Err returns the reason why the provisioner has stopped or tomb.ErrStillAlive
// when it is still alive.
func (p *storageProvisioner) Err() (reason error) {
	return p.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (p *storageProvisioner) Kill() {
	p.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *storageProvisioner) Wait() error {
	return p.tomb.Wait()
}

// Stop stops the provisioner and returns any error encountered while
// provisioning.
func (p *storageProvisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

// NewStorageProvisioner creates a new storage provisioner.
func NewStorageProvisioner(api *environment.Facade, machine *provisioner.Machine, agentConfig agent.Config) Provisioner {
	p := &storageProvisioner{
		api:         api,
		machine:     machine,
		agentConfig: agentConfig,
	}
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}

func (p *storageProvisioner) loop() error {
	// TODO - this is a hack.
	environConfig, err := p.api.EnvironConfig()
	if err != nil {
		logger.Errorf("cannot load environment configuration: %v", err)
		return err
	}
	timer := time.NewTimer(0)
	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-timer.C:
			volumes, err := p.machine.StorageParams()
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("volumes: %+v", volumes)
			if err := p.provisionVolumes(environConfig, volumes); err != nil {
				return errors.Trace(err)
			}
			// TODO(axw) create filesystems too.
			timer.Reset(10 * time.Second)
		}
	}
}

func (p *storageProvisioner) provisionVolumes(environConfig *config.Config, volumes []corestorage.VolumeParams) error {
	if len(volumes) == 0 {
		return nil
	}
	byProvider := make(map[corestorage.ProviderType][]corestorage.VolumeParams)
	for _, v := range volumes {
		byProvider[v.Provider] = append(byProvider[v.Provider], v)
	}
	for providerType, volumes := range byProvider {
		// TODO(axw) should we instead record the pool name on the
		// volume params, and then group volumes by pool when creating?
		cfg := map[string]interface{}{}
		if providerType == provider.LoopProviderType {
			cfg[provider.LoopDataDir] = filepath.Join(p.agentConfig.DataDir(), "storage", "block", "loop")
		}
		poolName := string(providerType) // TODO(axw) use pool name ...
		providerCfg, err := corestorage.NewConfig(poolName, providerType, cfg)
		if err != nil {
			logger.Errorf("invalid provider config: %v", err)
			return err
		}
		logger.Infof("creating %q volumes with parameters %v", providerType, volumes)

		// Create the volumes.
		blockDevices, err := p.Create(environConfig, providerCfg, providerType, volumes)
		if err != nil {
			logger.Errorf("cannot create specified volumes: %v", err)
			return err
		}
		logger.Infof("block devices created: %v", blockDevices)
		if err := p.machine.SetProvisionedBlockDevices(blockDevices); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (p *storageProvisioner) Create(
	envCfg *config.Config, providerCfg *corestorage.Config, providerType corestorage.ProviderType, volumes []corestorage.VolumeParams,
) ([]corestorage.BlockDevice, error) {

	provider, err := corestorage.StorageProvider(providerType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	source, err := provider.VolumeSource(envCfg, providerCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return source.CreateVolumes(volumes)
}
