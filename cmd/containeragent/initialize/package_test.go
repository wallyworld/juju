// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type importSuite struct{}

func TestImportSuite(t *tctesting.T) {
	tc.Run(t, &importSuite{})
}

func (*importSuite) TestImports(c *tc.C) {
	found := set.NewStrings(
		coretesting.FindJujuCoreImports(c, "github.com/juju/juju/cmd/containeragent/initialize")...)

	expected := set.NewStrings(
		"agent",
		"agent/constants",
		"api",
		"api/agent/agent",
		"api/agent/caasapplication",
		"api/agent/keyupdater",
		"api/base",
		"api/common",
		"api/common/cloudspec",
		"api/watcher",
		"apiserver/errors",
		"charmhub",
		"charmhub/path",
		"charmhub/transport",
		"cloud",
		"cmd",
		"cmd/constants",
		"cmd/containeragent/utils",
		"cmd/output",
		"controller",
		"core/arch",
		"core/base",
		"core/charm/metrics",
		"core/constraints",
		"core/devices",
		"core/facades",
		"core/instance",
		"core/leadership",
		"core/lease",
		"core/life",
		"core/logger",
		"core/macaroon",
		"core/machinelock",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/os/ostype",
		"core/paths",
		"core/permission",
		"core/presence",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/watcher",
		"docker",
		"environs/cloudspec",
		"environs/config",
		"environs/context",
		"environs/tags",
		"feature",
		"internal/provider/lxd/lxdnames",
		"internal/provider/kubernetes/constants",
		"internal/worker/apicaller",
		"internal/worker/introspection",
		"internal/worker/introspection/pprof",
		"juju/osenv",
		"juju/sockets",
		"logfwd",
		"logfwd/syslog",
		"mongo",
		"network",
		"network/debinterfaces",
		"network/netplan",
		"packaging",
		"packaging/dependency",
		"pki",
		"proxy",
		"pubsub/agent",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"service/common",
		"service/pebble/identity",
		"service/pebble/plan",
		"service/snap",
		"service/systemd",
		"state/errors",
		"storage",
		"tools",
		"utils/proxy",
		"utils/scriptrunner",
		"version",
		"utils/stringcompare",
	)

	unexpected := found.Difference(expected)
	// TODO: review if there are any un-expected imports!
	// Show the values rather than just checking the length so a failing
	// test shows them.
	c.Check(unexpected.SortedValues(), tc.DeepEquals, []string{})
	// If unneeded show any values this is good as we've reduced
	// dependencies, and they should be removed from expected above.
	unneeded := expected.Difference(found)
	c.Check(unneeded.SortedValues(), tc.DeepEquals, []string{})
}
