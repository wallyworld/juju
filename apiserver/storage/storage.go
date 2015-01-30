// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
	"github.com/juju/juju/storage/volume"
)

func init() {
	common.RegisterStandardFacadeForFeature("Storage", 1, NewAPI, feature.Storage)
}

var getState = func(st *state.State) storageAccess {
	return stateShim{st}
}

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage    storageAccess
	authorizer common.Authorizer
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:    getState(st),
		authorizer: authorizer,
	}, nil
}

func (api *API) Show(entities params.Entities) (params.StorageShowResults, error) {
	all := make([]params.StorageShowResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		all[i] = api.createStorageInstanceResult(entity.Tag)
	}
	return params.StorageShowResults{Results: all}, nil
}

func (api *API) List() (params.StorageListResult, error) {
	stateInstances, err := api.storage.AllStorageInstances()
	if err != nil {
		return params.StorageListResult{}, err
	}
	paramsInstances := make([]params.StorageInstance, len(stateInstances))
	for i, stateInst := range stateInstances {
		paramsInst, err := api.getStorageInstance(stateInst)
		if err != nil {
			err = errors.Annotatef(err, "getting storage instance %q", stateInst.Id())
			return params.StorageListResult{}, err
		}
		paramsInstances[i] = paramsInst
	}
	return params.StorageListResult{paramsInstances}, nil
}

func (api *API) createStorageInstanceResult(tag string) params.StorageShowResult {
	aTag, err := names.ParseTag(tag)
	if err != nil {
		return params.StorageShowResult{
			Error: params.ErrorResult{
				Error: common.ServerError(errors.Annotatef(common.ErrPerm, "getting %v", tag))},
		}
	}
	stateInstance, err := api.storage.StorageInstance(aTag.Id())
	if err != nil {
		return params.StorageShowResult{
			Error: params.ErrorResult{
				Error: common.ServerError(errors.Annotatef(common.ErrPerm, "getting %v", tag))},
		}
	}
	paramsStorageInstance, err := api.getStorageInstance(stateInstance)
	if err != nil {
		return params.StorageShowResult{
			Error: params.ErrorResult{
				Error: common.ServerError(errors.Annotatef(err, "getting %v", tag))},
		}
	}
	return params.StorageShowResult{Result: paramsStorageInstance}
}

func (api *API) getStorageInstance(si state.StorageInstance) (params.StorageInstance, error) {
	var location *string
	var totalSize *uint64
	var availableSize *uint64
	info, err := si.Info()
	if err == nil {
		location = &info.Location
		totalSize = &info.Size
		// TODO(axw) avail size?
	} else if !errors.IsNotProvisioned(err) {
		return params.StorageInstance{}, err
	}
	return params.StorageInstance{
		OwnerTag:      si.Owner().String(),
		StorageTag:    si.Tag().String(),
		Location:      location,
		TotalSize:     totalSize,
		AvailableSize: availableSize,
	}, nil
}

var getPoolManager = func(psm pool.SettingsManager) pool.PoolManager {
	return pool.NewPoolManager(psm)
}

func (a *API) ListPools(filter params.StoragePoolFilter) (params.StoragePoolsResult, error) {
	settings := a.storage.StateSettings()
	poolManager := getPoolManager(settings)

	all, err := poolManager.List()
	if err != nil {
		return params.StoragePoolsResult{}, err
	}
	results := []params.StoragePool{}
	// Convert to sets as easier to deal with
	typeSet := set.NewStrings(filter.Types...)
	nameSet := set.NewStrings(filter.Names...)
	for _, apool := range all {
		if one, k := filterPoolInstance(typeSet, nameSet, apool); k {
			results = append(results, one)
		}
	}
	return params.StoragePoolsResult{Pools: results}, nil
}

func filterPoolInstance(typeSet, nameSet set.Strings, apool pool.Pool) (params.StoragePool, bool) {
	keep := func(aSet set.Strings, value string) bool {
		if len(aSet) > 0 {
			return aSet.Contains(value)
		}
		return true
	}

	empty := params.StoragePool{}
	// filter by name
	if !keep(nameSet, apool.Name()) {
		return empty, false
	}
	// filter by type
	poolType := fmt.Sprintf("%v", apool.Type())
	if !keep(typeSet, poolType) {
		return empty, false
	}
	one := params.StoragePool{
		Name:   apool.Name(),
		Type:   poolType,
		Config: apool.Config(),
	}
	return one, true
}

func (a *API) CreatePool(p params.StoragePool) error {
	// TODO(anastasiamac 2015-01-29) move to business logic layer
	providerType := storage.ProviderType(p.Type)
	if err := checkProviderTypeSupported(a, providerType); err != nil {
		return errors.Trace(err)
	}

	settings := a.storage.StateSettings()
	poolManager := getPoolManager(settings)

	_, err := poolManager.Create(p.Name, providerType, p.Config)
	if err != nil {
		return err
	}
	return nil
}

var checkProviderTypeSupported = (*API).checkProviderTypeSupported

func (a *API) checkProviderTypeSupported(providerType storage.ProviderType) error {
	cfg, err := a.storage.EnvironConfig()
	if err != nil {
		return errors.Trace(err)
	}
	envT := cfg.Type()
	if !storage.IsProviderSupported(envT, providerType) {
		return fmt.Errorf("provider type %v is not supported for %v", providerType, envT)
	}
	return nil
}

var getVolumeManager = func(vs volume.VolumeState) volume.VolumeManager {
	return volume.NewVolumeManager(vs)
}

func (a *API) ListVolumes(filter params.StorageVolumeFilter) (params.StorageVolumesResult, error) {
	volumeManager := getVolumeManager(a.storage)

	all, err := volumeManager.List()
	if err != nil {
		return params.StorageVolumesResult{}, err
	}
	results := []params.StorageDisk{}
	// Convert to sets as easier to deal with
	machineSet := set.NewStrings(filter.Machines...)
	for _, disk := range all {
		if one, k := filterDisk(machineSet, disk); k {
			results = append(results, one)
		}
	}
	return params.StorageVolumesResult{Disks: results}, nil
}

// filterDisk returns converted Disk and boolean indicating
// if disk contains attachments that match filter
func filterDisk(machineSet set.Strings, disk volume.Disk) (params.StorageDisk, bool) {
	attachments := []params.VolumeAttachment{}
	for _, attachment := range disk.Attachments() {
		if one, k := filterAttachment(machineSet, attachment); k {
			attachments = append(attachments, one)
		}
	}
	// it's possible that there will be no attachments on this disk
	// that match the filter. This disk will be filtered out too then :D
	return params.StorageDisk{Attachments: attachments}, len(attachments) > 0
}

func filterAttachment(machineSet set.Strings, attachment volume.Attachment) (params.VolumeAttachment, bool) {
	if len(machineSet) > 0 {
		empty := params.VolumeAttachment{}
		// filter by machine
		if !machineSet.Contains(attachment.Machine()) {
			return empty, false
		}
	}
	one := params.VolumeAttachment{
		Volume:      attachment.Volume().String(),
		Machine:     names.NewMachineTag(attachment.Machine()).String(),
		DeviceName:  attachment.DeviceName(),
		Size:        attachment.Size(),
		Storage:     names.NewStorageTag(attachment.Storage()).String(),
		Assigned:    attachment.Assigned(),
		Attached:    attachment.Attached(),
		FileSystem:  attachment.FilesystemType(),
		Provisioned: attachment.Provisioned(),
	}
	return one, true
}
