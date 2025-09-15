// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type mockStoragePoolManager struct {
	testhelpers.Stub
	poolmanager.PoolManager
	storageType storage.ProviderType
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, m.storageType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	testhelpers.Stub
	storage.ProviderRegistry
}
