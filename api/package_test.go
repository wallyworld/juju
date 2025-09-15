// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	tctesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

func TestImportSuite(t *tctesting.T) {
	tc.Run(t, &ImportSuite{})
}

func (*ImportSuite) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api")

	c.Assert(found, tc.SameContents, []string{
		"api/agent/keyupdater",
		"api/base",
		"api/watcher",
		"core/arch",
		"core/base",
		"core/constraints",
		"core/devices",
		"core/facades",
		"core/instance",
		"core/life",
		"core/macaroon",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/os/ostype",
		"core/paths",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/watcher",
		"docker",
		"environs/context",
		"feature",
		"proxy",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"storage",
		"tools",
		"utils/proxy",
		"utils/stringcompare",
		"version",
	})
}
