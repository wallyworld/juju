// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"runtime"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type containerAgentSuite struct {
	testing.BaseSuite
}

func TestContainerAgentSuite(t *tctesting.T) {
	if runtime.GOOS != "linux" {
		t.Skip("containeragent only runs on Linux")
	}
	tc.Run(t, &containerAgentSuite{})
}

type mainWrapperTC struct {
	args []string
	code int
}

func (s *containerAgentSuite) TestMainWrapper(c *tc.C) {
	factory := commandFactory{
		containerAgentCmd: func(ctx *cmd.Context, args []string) int {
			return 11
		},
		jujuExec: func(ctx *cmd.Context, args []string) int {
			return 12
		},
		jujuIntrospect: func(ctx *cmd.Context, args []string) int {
			return 14
		},
	}
	for _, testcase := range []mainWrapperTC{
		{args: []string{"containeragent"}, code: 11},
		{args: []string{"juju-exec"}, code: 12},
		{args: []string{"juju-introspect"}, code: 14},
	} {
		c.Check(mainWrapper(factory, testcase.args), tc.DeepEquals, testcase.code)
	}
}

func (s *containerAgentSuite) TestRegisteredSubCommandsForContainerAgentCommand(c *tc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	containerAgentCmd, err := containerAgentCommand(ctx)
	c.Assert(err, tc.ErrorIsNil)
	ctx, err = cmdtesting.RunCommand(c, containerAgentCmd, []string{"help", "commands"}...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
documentation  Generate the documentation for all commands
help           Show help on a command or other topic.
init           Initialize containeragent local state.
unit           Start containeragent.
`[1:])
}
