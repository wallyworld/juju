// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// StorageInstance holds data for a storage instance.
type StorageInstance struct {
	StorageTag    string
	OwnerTag      string
	Location      string
	StorageName   string
	AvailableSize uint64
	TotalSize     uint64
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

// PoolInstance holds data for a pool instance.
type PoolInstance struct {
	Name   string
	Type   string
	Traits map[string]interface{}
}

// PoolListFilter holds a filter for pool list API call.
type PoolListFilter struct {
	Names []string
	Types []string
}

// PoolListResults holds a collection of pool instances.
type PoolListResults struct {
	Results []PoolListResult
}

// PoolListResult holds information about a pool instance
// or error related to its retrieval.
type PoolListResult struct {
	Result PoolInstance
	Error  ErrorResult
}
