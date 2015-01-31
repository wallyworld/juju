// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
)

const volumeCmdDoc = `
"juju storage volume" is used to manage storage volumes in
 Juju environment.
`

const volumeCmdPurpose = "manage storage volumes"

// NewVolumeSuperCommand creates the storage volume super subcommand and
// registers the subcommands that it supports.
func NewVolumeSuperCommand() cmd.Command {
	poolcmd := Command{
		SuperCommand: *jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
			Name:        "volume",
			Doc:         volumeCmdDoc,
			UsagePrefix: "juju storage",
			Purpose:     volumeCmdPurpose,
		})}
	poolcmd.Register(envcmd.Wrap(&VolumeListCommand{}))
	return &poolcmd
}

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}

// VolumeInfo defines the serialization behaviour of the storage volume (currently, disk) information.
type VolumeInfo struct {
	Attachments map[string]AttachmentInfo
}

type AttachmentInfo struct {
	Storage     string  `yaml:"storage" json:"storage"`
	Assigned    bool    `yaml:"assigned" json:"assigned"`
	Machine     string  `yaml:"machine" json:"machine"`
	Attached    bool    `yaml:"attached" json:"attached"`
	DeviceName  string  `yaml:"device-name" json:"device-name"`
	Size        *uint64 `yaml:"size" json:"size"`
	FileSystem  string  `yaml:"file-system" json:"file-system"`
	Provisioned bool    `yaml:"provisioned" json:"provisioned"`
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

func formatAttachmentInfo(all []params.VolumeAttachment) (map[string]AttachmentInfo, error) {
	result := make(map[string]AttachmentInfo)
	for _, one := range all {
		// TODO (anastasiamac 2015-01-31) add similar logic for volume tags
		// when available
		storageTag, err := names.ParseStorageTag(one.Storage)
		if err != nil {
			return nil, errors.Annotate(err, "invalid storage tag")
		}
		storageName, err := names.StorageName(storageTag.Id())
		if err != nil {
			panic(err) // impossible
		}
		machineTag, err := names.ParseTag(one.Machine)
		if err != nil {
			return nil, errors.Annotate(err, "invalid machine tag")
		}

		result[one.Volume] = AttachmentInfo{
			Storage:     storageName,
			Assigned:    one.Assigned,
			Machine:     machineTag.Id(),
			Attached:    one.Attached,
			DeviceName:  one.DeviceName,
			Size:        one.Size,
			FileSystem:  one.FileSystem,
			Provisioned: one.Provisioned,
		}
	}
	return result, nil
}
