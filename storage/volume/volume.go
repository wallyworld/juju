// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package volume

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// NewVolumeManager returns a VolumeManager implementation using the specified state.
func NewVolumeManager(st VolumeState) VolumeManager {
	return &volumeManager{vst: st}
}

var _ VolumeManager = (*volumeManager)(nil)

type volumeManager struct {
	vst VolumeState
}

type VolumeState interface {
	AllBlockDevices() ([]state.BlockDevice, error)
}

// List is defined on VolumeManager interface.
func (pm *volumeManager) List() ([]Disk, error) {
	devices, err := pm.vst.AllBlockDevices()
	if err != nil {
		return nil, errors.Annotate(err, "listing block devices")
	}
	if len(devices) < 1 {
		return nil, nil
	}
	attachments := make([]Attachment, len(devices))
	for i, d := range devices {
		attachments[i] = pm.constructAttachment(d)
	}
	// TODO(anastasiamac 2015-01-30) since volumes don't really exist yet,
	// only return one disk for now
	volumes := make([]Disk, 1)
	volumes[0] = &disk{attachments: attachments}
	return volumes, nil
}

var _ Disk = (*disk)(nil)

type disk struct {
	attachments []Attachment
	// TODO(anastasiamac 2015-01-30 add name and persisted parameters
	// when model is decided
}

// Attachments implements Disk.Attachments
func (v *disk) Attachments() []Attachment {
	return v.attachments
}

var _ Attachment = (*attachment)(nil)

type attachment struct {
	diskTag     names.DiskTag
	name        string
	storageId   string
	assigned    bool
	machineId   string
	attached    bool
	deviceName  string
	uuid        string
	label       string
	size        uint64
	inuse       bool
	fstype      string
	provisioned bool
}

// Volume implements Attachment.Volume
func (a *attachment) Volume() names.DiskTag {
	return a.diskTag
}

// AttachmentName implements Attachment.AttachmentName
func (a *attachment) AttachmentName() string {
	return a.name
}

// StorageInstance implements Attachment.StorageInstance
func (a *attachment) Storage() string {
	return a.storageId
}

// Assigned implements Attachment.Assigned
func (a *attachment) Assigned() bool {
	return a.assigned
}

// Machine implements Attachment.Machine
func (a *attachment) Machine() string {
	return a.machineId
}

// Attached implements Attachment.Attached
func (a *attachment) Attached() bool {
	return a.attached
}

// Attached implements Attachment.Attached
func (a *attachment) DeviceName() string {
	return a.deviceName
}

// Attached implements Attachment.Attached
func (a *attachment) UUID() string {
	return a.uuid
}

// Label implements Attachment.Label
func (a *attachment) Label() string {
	return a.label
}

// Size implements Attachment.Size
func (a *attachment) Size() uint64 {
	return a.size
}

// InUse implements Attachment.InUse
func (a *attachment) InUse() bool {
	return a.inuse
}

// FilesystemType implements Attachment.FilesystemType
func (a *attachment) FilesystemType() string {
	return a.fstype
}

// Provisioned implements Attachment.Provisioned
func (a *attachment) Provisioned() bool {
	return a.provisioned
}

func (vm *volumeManager) constructAttachment(d state.BlockDevice) Attachment {
	dTag, ok := d.Tag().(names.DiskTag)
	if !ok {
		// (axw 2015-01-30) it will always be a DiskTag
		panic("tag should always be a disk tag")
	}

	result := &attachment{
		diskTag:   dTag,
		name:      d.Name(),
		machineId: d.Machine(),
		attached:  d.Attached(),
	}

	result.storageId, result.assigned = d.StorageInstance()

	info, err := d.Info()
	if err != nil {
		// this will only happen if attachment is not provisioned
		return result
	}

	result.provisioned = true
	result.deviceName = info.DeviceName
	result.uuid = info.UUID
	result.label = info.Label
	result.size = info.Size
	result.inuse = info.InUse
	result.fstype = info.FilesystemType

	return result
}
