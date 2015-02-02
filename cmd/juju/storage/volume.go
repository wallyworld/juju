// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}

// VolumeInfo defines the serialization behaviour of the storage volume (currently, disk) information.
type VolumeInfo struct {
	Attachments map[string]map[string]map[string]AttachmentInfo
}

type AttachmentInfo struct {
	Storage     string  `yaml:"storage,omitempty" json:"storage,omitempty"`
	Assigned    bool    `yaml:"assigned,omitempty" json:"assigned,omitempty"`
	Attached    bool    `yaml:"attached,omitempty" json:"attached,omitempty"`
	Size        *uint64 `yaml:"size,omitempty" json:"size,omitempty"`
	FileSystem  string  `yaml:"file-system,omitempty" json:"file-system,omitempty"`
	Provisioned bool    `yaml:"provisioned,omitempty" json:"provisioned,omitempty"`
}

func formatVolumeInfo(all []params.StorageVolume) ([]VolumeInfo, error) {
	result := make([]VolumeInfo, len(all))
	for i, one := range all {
		a, err := formatAttachmentInfo(one.Attachments)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i] = VolumeInfo{
			Attachments: a,
		}
	}
	return result, nil
}

func formatAttachmentInfo(all []params.VolumeAttachment) (map[string]map[string]map[string]AttachmentInfo, error) {
	result := map[string]map[string]map[string]AttachmentInfo{}
	for _, one := range all {
		// TODO (anastasiamac 2015-01-31) add similar logic for volume tags
		// when available
		storageName := ""
		if one.Storage != "" {
			storageTag, err := names.ParseStorageTag(one.Storage)
			if err != nil {
				return result, errors.Annotate(err, "invalid storage tag")
			}
			storageName, _ = names.StorageName(storageTag.Id())
		}

		machineTag, err := names.ParseTag(one.Machine)
		if err != nil {
			return result, errors.Annotate(err, "invalid machine tag")
		}
		machineId := machineTag.Id()

		deviceName := one.DeviceName
		volumeName := one.Volume

		ai := AttachmentInfo{
			Storage:     storageName,
			Assigned:    one.Assigned,
			Attached:    one.Attached,
			Size:        one.Size,
			FileSystem:  one.FileSystem,
			Provisioned: one.Provisioned,
		}

		// group 1st by machine
		machineColl, ok := result[machineId]
		if !ok {
			machineColl = map[string]map[string]AttachmentInfo{}
			result[machineId] = machineColl
		}

		// then group by device name
		deviceColl, ok := machineColl[deviceName]
		if !ok {
			deviceColl = map[string]AttachmentInfo{}
			machineColl[deviceName] = deviceColl
		}

		// then group by volume name
		deviceColl[volumeName] = ai
	}
	return result, nil
}
