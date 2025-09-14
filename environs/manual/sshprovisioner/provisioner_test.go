// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"fmt"
	"os"
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v3/shell"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	apiclient "github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

type provisionerSuite struct {
	testing.JujuConnSuite
}

func TestProvisionerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &provisionerSuite{})
}

func (s *provisionerSuite) getArgs(c *tc.C) manual.ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, tc.ErrorIsNil)
	client := apiclient.NewClient(s.APIState)
	s.AddCleanup(func(*tc.C) { client.Close() })
	return manual.ProvisionMachineArgs{
		Host:           hostname,
		Client:         client,
		UpdateBehavior: &params.UpdateBehavior{true, true},
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *tc.C) {
	base := jujuversion.DefaultSupportedLTSBase()
	const arch = "amd64"

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = hostname
	args.User = "ubuntu"

	defaultToolsURL := envtools.DefaultBaseURL
	envtools.DefaultBaseURL = ""

	defer fakeSSH{
		Base:               base,
		Arch:               arch,
		InitUbuntuUser:     true,
		SkipProvisionAgent: true,
	}.install(c).Restore()

	// Attempt to provision a machine with no tools available, expect it to fail.
	machineId, err := sshprovisioner.ProvisionMachine(args)
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
	c.Assert(machineId, tc.Equals, "")

	cfg := s.Environ.Config()
	number, ok := cfg.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	binVersion := version.Binary{
		Number:  number,
		Release: "ubuntu",
		Arch:    arch,
	}
	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, "released", "released", binVersion)
	envtools.DefaultBaseURL = defaultToolsURL

	for i, errorCode := range []int{255, 0} {
		c.Logf("test %d: code %d", i, errorCode)
		defer fakeSSH{
			Base:                   base,
			Arch:                   arch,
			InitUbuntuUser:         true,
			ProvisionAgentExitCode: errorCode,
		}.install(c).Restore()
		machineId, err = sshprovisioner.ProvisionMachine(args)
		if errorCode != 0 {
			c.Assert(err, tc.ErrorMatches, fmt.Sprintf("subprocess encountered error code %d", errorCode))
			c.Assert(machineId, tc.Equals, "")
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(machineId, tc.Not(tc.Equals), "")
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Check(machineId, tc.Equals, fmt.Sprint(i+1))

			m, err := s.State.Machine(machineId)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(m.Addresses(), tc.HasLen, 0)

			instanceId, err := m.InstanceId()
			c.Assert(err, tc.ErrorIsNil)

			c.Check(instanceId, tc.Equals, instance.Id("manual:"+hostname))
		}
	}

	// Attempting to provision a machine twice should fail. We effect
	// this by checking for existing juju systemd configurations.
	defer fakeSSH{
		Provisioned:        true,
		InitUbuntuUser:     true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	_, err = sshprovisioner.ProvisionMachine(args)
	c.Assert(err, tc.Equals, manual.ErrProvisioned)
	defer fakeSSH{
		Provisioned:              true,
		CheckProvisionedExitCode: 255,
		InitUbuntuUser:           true,
		SkipDetection:            true,
		SkipProvisionAgent:       true,
	}.install(c).Restore()
	_, err = sshprovisioner.ProvisionMachine(args)
	c.Assert(err, tc.ErrorMatches, "error checking if provisioned: subprocess encountered error code 255")
}

func (s *provisionerSuite) TestFinishInstanceConfig(c *tc.C) {
	base := jujuversion.DefaultSupportedLTSBase()
	const arch = "amd64"
	defer fakeSSH{
		Base:           base,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()

	machineId, err := sshprovisioner.ProvisionMachine(s.getArgs(c))
	c.Assert(err, tc.ErrorIsNil)

	// Now check what we would've configured it with.
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	icfg, err := machinemanager.InstanceConfig(systemState, machinemanager.StateBackend(s.State), machineId, agent.BootstrapNonce, "/var/lib/juju")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(icfg, tc.NotNil)
	c.Check(icfg.APIInfo, tc.NotNil)

	apiInfo := s.APIInfo(c)
	c.Check(icfg.APIInfo.Addrs, tc.DeepEquals, apiInfo.Addrs)
}

func (s *provisionerSuite) TestProvisioningScript(c *tc.C) {
	base := jujuversion.DefaultSupportedLTSBase()
	const arch = "amd64"
	defer fakeSSH{
		Base:           base,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()

	machineId, err := sshprovisioner.ProvisionMachine(s.getArgs(c))
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.UpdateModelConfig(
		map[string]interface{}{
			"enable-os-upgrade": false,
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	icfg, err := machinemanager.InstanceConfig(systemState, machinemanager.StateBackend(s.State), machineId, agent.BootstrapNonce, "/var/lib/juju")
	c.Assert(err, tc.ErrorIsNil)

	script, err := sshprovisioner.ProvisioningScript(icfg)
	c.Assert(err, tc.ErrorIsNil)

	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	c.Assert(err, tc.ErrorIsNil)
	err = udata.ConfigureJuju()
	c.Assert(err, tc.ErrorIsNil)
	cloudcfg.SetSystemUpgrade(false)
	provisioningScript, err := cloudcfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)

	removeLogFile := "rm -f '/var/log/cloud-init-output.log'\n"
	expectedScript := removeLogFile + shell.DumpFileOnErrorScript("/var/log/cloud-init-output.log") + provisioningScript
	c.Assert(script, tc.Equals, expectedScript)
}
