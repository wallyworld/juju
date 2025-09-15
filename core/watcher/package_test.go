// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportTest struct{}

func TestImportTest(t *tctesting.T) {
	tc.Run(t, &ImportTest{})
}

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/watcher")

	// This package brings in nothing else from outside juju/juju/core
	c.Assert(found, tc.SameContents, []string{
		"core/life",
		"core/migration",
		"core/network",
		"core/resources",
		"core/secrets",
		"core/status",
		//  TODO: these have been brought in from migration and this is BAD.
		"docker",
	})

}
