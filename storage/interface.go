// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

// FeatureFlag is the name of the feature for the JUJU_DEV_FEATURE_FLAGS
// envar. Add this string to the envar to enable support for storage.
const FeatureFlag = "storage"

// VolumeParams is a fully specified set of parameters for volume creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type VolumeParams struct {
	// Name is a unique name assigned by Juju for the requested volume.
	Name string

	// Size is the minimum size of the volume in MiB.
	Size uint64

	// Options is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Options map[string]interface{}

	// The provider type for this volume.
	Provider ProviderType

	// Instance is the ID of the instance that the volume should be attached
	// to initially. This will only be empty if the instance is not yet
	// provisioned, in which case the parameters refer to a volume that is
	// being created in conjunction with the instance.
	Instance instance.Id
}

type FilesystemParams struct {
	// Name is a unique name assigned by Juju for the requested filesystem.
	Name string

	// Size is the minimum size of the filesystem in MiB.
	Size uint64

	// HACK. This should be on the attachment params.
	Location string

	// Options is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Options map[string]interface{}

	// The provider type for this volume.
	Provider ProviderType
}

// ProviderType uniquely identifies a storage provider, such as "ebs" or "loop".
type ProviderType string

// Provider is an interface for obtaining storage sources.
type Provider interface {
	// VolumeSource returns a VolumeSource given the
	// specified cloud and storage provider configurations.
	//
	// If the storage provider does not support creating volumes as a
	// first-class primitive, then VolumeSource must return an error
	// satisfying errors.IsNotSupported.
	VolumeSource(environConfig *config.Config, providerConfig *Config) (VolumeSource, error)

	// FilesystemSource returns a FilesystemSource given the
	// specified cloud and storage provider configurations.
	//
	// If the storage provider does not support creating filesystems
	// as a first-class primitive, then FilesystemSource must return
	// an error satisfying errors.IsNotSupported.
	FilesystemSource(environConfig *config.Config, providerConfig *Config) (FilesystemSource, error)

	// ValidateConfig validates the provided storage provider config,
	// returning an error if it is invalid.
	ValidateConfig(*Config) error
}

// VolumeSource provides an interface for creating, destroying and
// describing volumes in the environment. A VolumeSource is configured
// in a particular way, and corresponds to a storage "pool".
type VolumeSource interface {
	// CreateVolumes creates volumes with the specified size, in MiB.
	//
	// TODO(axw) CreateVolumes should return something other than
	// []BlockDevice, so we can communicate additional information
	// about the volumes that are not relevant at the attachment
	// level.
	CreateVolumes(params []VolumeParams) ([]BlockDevice, error)

	// DescribeVolumes returns the properties of the volumes with the
	// specified provider volume IDs.
	//
	// TODO(axw) as in CreateVolumes, we should return something other
	// than []BlockDevice here.
	DescribeVolumes(volIds []string) ([]BlockDevice, error)

	// DestroyVolumes destroys the volumes with the specified provider
	// volume IDs.
	DestroyVolumes(volIds []string) error

	// ValidateVolumeParams validates the provided volume creation
	// parameters, returning an error if they are invalid.
	//
	// If the provider is incapable of provisioning volumes separately
	// from machine instances (e.g. MAAS), then ValidateVolumeParams
	// must return an error if params.Instance is non-empty.
	ValidateVolumeParams(params VolumeParams) error

	// AttachVolumes attaches the volumes with the specified provider
	// volume IDs to the instances with the corresponding index.
	//
	// TODO(axw) we need to validate attachment requests prior to
	// recording in state. For example, the ec2 provider must reject
	// an attempt to attach a volume to an instance if they are in
	// different availability zones.
	AttachVolumes(volIds []string, instId []instance.Id) error

	// DetachVolumes detaches the volumes with the specified provider
	// volume IDs from the instances with the corresponding index.
	//
	// TODO(axw) we need to record in state whether or not volumes
	// are detachable, and reject attempts to attach/detach on
	// that basis.
	DetachVolumes(volIds []string, instId []instance.Id) error
}

// FilesystemSource provides an interface for creating, destroying and
// describing filesystems in the environment. A FilesystemSource is
// configured in a particular way, and corresponds to a storage "pool".
type FilesystemSource interface {
	// CreateFilesystems creates filesystems with the specified size, in MiB.
	CreateFilesystems(params []FilesystemParams) ([]Filesystem, error)
}

type Filesystem struct {
	Name string
	Size uint64

	// This is dumb, we should be storing this information on an
	// attachment, to support shared filesystems.
	Location string
}
