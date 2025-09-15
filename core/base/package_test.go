// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package base -destination distrosource_mock_test.go github.com/juju/juju/core/base DistroSource

type ImportTest struct{}

func TestImportTest(t *tctesting.T) {
	tc.Run(t, &ImportTest{})
}

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/base")
	c.Assert(found, tc.SameContents, []string{
		"core/os/ostype",
	})
}
