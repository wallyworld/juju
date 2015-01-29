// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The storage package provides storage provider implementations
// for AWS. See github.com/juju/juju/storage.Provider.
package storage

import (
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

func init() {
	storage.RegisterProvider(EBSProviderType, &ebsProvider{})
	storage.RegisterEnvironStorageProviders("ec2", EBSProviderType)
	// TODO(axw) we should have a way of registering common storage
	// providers, which ought to work across all clouds.
	storage.RegisterEnvironStorageProviders("ec2", provider.LoopProviderType)
	storage.RegisterEnvironStorageProviders("ec2", provider.RootfsProviderType)
	storage.RegisterEnvironStorageProviders("ec2", provider.TmpfsProviderType)

	storage.RegisterDefaultPool("ec2", storage.StorageKindBlock, EBSPool)
}
