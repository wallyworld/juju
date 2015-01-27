// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/storage"
)

func init() {
	storage.RegisterProvider(LoopProviderType, &loopProvider{})
}
