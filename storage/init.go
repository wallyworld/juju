// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// TODO - fix the import loops
//import (
//	"github.com/juju/juju/environs"
//	_ "github.com/juju/juju/provider/all"
//	"github.com/juju/juju/storage/provider"
//)
//
//func init() {
//	// All environments providers support rootfs loop devices.
//	// As a failsafe, ensure at least this storage provider is registered.
//	for _, envType := range environs.RegisteredProviders() {
//		RegisterEnvironStorageProviders(envType, provider.LoopProviderType)
//	}
//}
