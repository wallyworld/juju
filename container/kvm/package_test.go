// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"runtime"

	"github.com/juju/juju/core/arch"
)

func supportedArch() bool {
	for _, a := range []string{arch.AMD64, arch.ARM64, arch.PPC64EL} {
		if runtime.GOARCH == a {
			return true
		}
	}
	return false
}
