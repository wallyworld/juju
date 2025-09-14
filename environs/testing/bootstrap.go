// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testhelpers"
)

var logger = loggo.GetLogger("juju.environs.testing")

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap() func() {
	f := func(
		environs.BootstrapContext,
		ssh.Client,
		environs.Environ,
		envcontext.ProviderCallContext,
		instances.Instance,
		*instancecfg.InstanceConfig,
		environs.BootstrapDialOpts,
	) error {
		logger.Infof("provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testhelpers.PatchValue(&common.FinishBootstrap, f)
}

// BootstrapContext creates a simple bootstrap execution context.
func BootstrapContext(ctx context.Context, c *tc.C) environs.BootstrapContext {
	return modelcmd.BootstrapContext(ctx, cmdtesting.Context(c))
}

func BootstrapTODOContext(c *tc.C) environs.BootstrapContext {
	return BootstrapContext(context.TODO(), c)
}
