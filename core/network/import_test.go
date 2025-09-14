// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

func TestImportSuite(t *tctesting.T) {
	tc.Run(t, &ImportSuite{})
}

var allowedCoreImports = set.NewStrings("core/life")

func (*ImportSuite) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/network")
	for _, packageImport := range found {
		c.Assert(allowedCoreImports.Contains(packageImport), tc.IsTrue)
	}
}
