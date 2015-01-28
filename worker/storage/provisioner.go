// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path/filepath"

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
	// Provision initial storage for loop devices.
	provisionInfo, err := p.machine.ProvisioningInfo()
	if err != nil {
		return err
	}
	logger.Infof("provisioning info: %+v", provisionInfo.Volumes)
	environConfig, err := p.api.EnvironConfig()
	if err != nil {
		logger.Errorf("cannot load environment configuration: %v", err)
		return err
	}
	if err := p.provisionVolumes(environConfig, provisionInfo.Volumes); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
			// TODO - watch for new storage requests
		}
	}
}

func (p *storageProvisioner) provisionVolumes(environConfig *config.Config, volumes []corestorage.VolumeParams) error {
	// TODO - do not assume all volumes are same provider type
	if len(volumes) == 0 || volumes[0].VolumeType != provider.LoopProviderType {
		return nil
	}
	// TODO - get config from pool
	providerType := provider.LoopProviderType
	cfg := map[string]interface{}{
		provider.LoopDataDir: filepath.Join(p.agentConfig.DataDir(), "storage", "block", "loop"),
	}
	providerCfg, err := corestorage.NewConfig("loop", providerType, cfg)
	if err != nil {
		logger.Errorf("invalid provider config: %v", err)
		return err
	}

	// TODO - remove assumption of block devices
	// Create the specified block devices
	logger.Infof("creating %q volumes with parameters %v", providerType, volumes)
	blockDevices, err := p.Create(environConfig, providerCfg, providerType, volumes)
	if err != nil {
		logger.Errorf("cannot create specified volumes: %v", err)
		return err
	}
	// TODO - write block device info to state
	logger.Infof("block devices created: %v", blockDevices)
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
