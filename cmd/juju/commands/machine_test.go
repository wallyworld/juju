// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

// MachineSuite tests the connectivity of all the machine subcommands. These
// tests go from the command line, api client, api server, db. The db changes
// are then checked.  Only one test for each command is done here to check
// connectivity.  Exhaustive unit tests are at each layer.
type MachineSuite struct {
	jujutesting.JujuConnSuite
}

func TestMachineSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &MachineSuite{})
}

func (s *MachineSuite) RunCommand(c *tc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	juju := NewJujuCommand(context, "")
	if err := cmdtesting.InitCommand(juju, args); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *MachineSuite) TestMachineAdd(c *tc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	count := len(machines)

	ctx, err := s.RunCommand(c, "add-machine")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Contains, `created machine`)

	machines, err = s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, count+1)
}

func (s *MachineSuite) TestMachineRemove(c *tc.C) {
	machine := s.Factory.MakeMachine(c, nil)

	ctx, err := s.RunCommand(c, "remove-machine", "--no-prompt", machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Contains, `will remove machine`)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machine.Life(), tc.Equals, state.Dying)
}
