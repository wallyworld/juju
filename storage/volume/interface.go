// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package volume

import "github.com/juju/names"

// Volume
type Volume interface {
	// TODO(anastasiamac 2015-01-30 add name and persisted parameters
	// when model is decided
	Attachments() []Attachment
}

// Attachment is a block device
type Attachment interface {
	// Disk returns the tag for the disk.
	Disk() names.DiskTag

	// AttachmentName returns the unique name of the attachment.
	AttachmentName() string

	// Storage returns the ID of the storage instance that this
	// attachment is assigned to.
	Storage() string

	// Assigned indicates whether attachment is assigned to a store.
	//
	// A block device can be assigned to at most one store. It is possible
	// for multiple block devices to be assigned to the same store, e.g.
	// multi-attach volumes.
	Assigned() bool

	// Machine returns the ID of the machine the attachment is attached to.
	Machine() string

	// Provisioned indicates whether attachment is provisioned.
	Provisioned() bool

	// Attached returns true if the block device is known to be attached to
	// its associated machine.
	Attached() bool

	DeviceName() string
	UUID() string
	Label() string
	Size() uint64
	InUse() bool
	FilesystemType() string
}

// A VolumeManager provides access to storage volumes.
type VolumeManager interface {
	// List returns disks from state.
	List() ([]Volume, error)
}
