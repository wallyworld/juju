// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

// Pool configuration attribute names.
const (
	StoragePoolName     = "name"
	StorageProviderType = "type"
)

// Attrs defines storage attributes.
type Attrs map[string]string

// StoragePoolDetails defines the details of a storage pool to save.
// This type is also used when returning query results from state.
type StoragePoolDetails struct {
	Name     string
	Provider string
	Attrs    Attrs
}

// StoragePoolFilter defines attributes used to filter storage pools.
type StoragePoolFilter struct {
	// Names are pool's names to filter on.
	Names []string
	// Providers are pool's storage provider types to filter on.
	Providers []string
}

// BuiltInStoragePools returns the built in providers common to all.
func BuiltInStoragePools() ([]StoragePoolDetails, error) {
	providerTypes, err := provider.CommonStorageProviders().StorageProviderTypes()
	if err != nil {
		return nil, errors.Annotate(err, "getting built in storage provider types")
	}
	result := make([]StoragePoolDetails, len(providerTypes))
	for i, pType := range providerTypes {
		result[i] = StoragePoolDetails{
			Name:     string(pType),
			Provider: string(pType),
		}
	}
	return result, nil
}

// DefaultStoragePools returns the default storage pools to add to a new model
// for a given provider registry.
func DefaultStoragePools(registry storage.ProviderRegistry) ([]*storage.Config, error) {
	var result []*storage.Config
	providerTypes, err := registry.StorageProviderTypes()
	if err != nil {
		return nil, errors.Annotate(err, "getting storage provider types")
	}
	for _, providerType := range providerTypes {
		p, err := registry.StorageProvider(providerType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, pool := range p.DefaultPools() {
			result = append(result, pool)
		}
	}
	return result, nil
}
