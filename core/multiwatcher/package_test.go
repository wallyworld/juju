// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

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
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/multiwatcher")

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, tc.SameContents, []string{
		"core/arch",
		"core/constraints",
		"core/instance",
		"core/life",
		"core/model",
		"core/network",
		"core/permission",
		"core/status",
		"utils/stringcompare",
	})
}
