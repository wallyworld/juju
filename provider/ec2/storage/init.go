// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The storage package provides storage provider implementations
// for AWS. See github.com/juju/juju/storage.Provider.
package storage

import (
	"github.com/juju/juju/storage"
)

func init() {
	storage.RegisterProvider(EBSProviderType, &ebsProvider{})
}
