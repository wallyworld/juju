// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// StorageInstance holds data for a storage instance.
type StorageInstance struct {
	StorageTag string
	OwnerTag   string

	// Using pointers below to make values nullable.
	// Nil values are unknown/unavailable.

	Location      *string
	AvailableSize *uint64
	TotalSize     *uint64
	Tags          []string
}

// StorageShowResults holds a collection of storage instances.
type StorageShowResults struct {
	Results []StorageShowResult
}

// StorageShowResult holds information about a storage instance
// or error related to its retrieval.
type StorageShowResult struct {
	Result StorageInstance
	Error  ErrorResult
}

// StorageListResult holds information about storage instances.
type StorageListResult struct {
	Instances []StorageInstance
}

// StoragePool holds data for a pool instance.
type StoragePool struct {
	// Name is the pool's name.
	Name string
	// Type is the type of storage provider this pool represents, eg "loop", "ebs.
	Type string
	// Traits are the pool's configuration attributes.
	Traits map[string]interface{}
}

// StoragePoolFilter holds a filter for pool API call.
type StoragePoolFilter struct {
	// Names are pool's names to filter on.
	Names []string
	// Types are pool's storage provider type to filter on.
	Types []string
}

// StoragePoolsResult holds a collection of pool instances.
type StoragePoolsResult struct {
	Pools []StoragePool
}
