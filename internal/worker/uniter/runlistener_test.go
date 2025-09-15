// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/exec"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/internal/worker/uniter/runcommands"
	"github.com/juju/juju/juju/sockets"
)

type ListenerSuite struct {
	testing.BaseSuite
	socketPath sockets.Socket
}

func TestListenerSuite(t *tctesting.T) {
	tc.Run(t, &ListenerSuite{})
}

func sockPath(c *tc.C) sockets.Socket {
	sockPath := filepath.Join(c.MkDir(), "test.listener")
	return sockets.Socket{Address: sockPath, Network: "unix"}
}

func (s *ListenerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.socketPath = sockPath(c)
}

// Mirror the params to uniter.NewRunListener, but add cleanup to close it.
func (s *ListenerSuite) NewRunListener(c *tc.C, operator bool) *uniter.RunListener {
	listener, err := uniter.NewRunListener(s.socketPath, loggo.GetLogger("test"))
	c.Assert(err, tc.ErrorIsNil)
	listener.RegisterRunner("test/0", &mockCommandRunner{
		c:        c,
		operator: operator,
	})
	s.AddCleanup(func(c *tc.C) {
		c.Assert(listener.Close(), tc.ErrorIsNil)
	})
	return listener
}

func (s *ListenerSuite) TestNewRunListenerOnExistingSocketRemovesItAndSucceeds(c *tc.C) {
	s.NewRunListener(c, false)
	s.NewRunListener(c, false)
}

func (s *ListenerSuite) TestClientCall(c *tc.C) {
	s.NewRunListener(c, false)

	client, err := sockets.Dial(s.socketPath)
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	var result exec.ExecResponse
	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      -1,
		RemoteUnitName:  "",
		ForceRemoteUnit: false,
		UnitName:        "test/0",
	}
	err = client.Call(uniter.JujuExecEndpoint, args, &result)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(string(result.Stdout), tc.Equals, "some-command stdout")
	c.Assert(string(result.Stderr), tc.Equals, "some-command stderr")
	c.Assert(result.Code, tc.Equals, 42)
}

func (s *ListenerSuite) TestUnregisterRunner(c *tc.C) {
	listener := s.NewRunListener(c, false)
	listener.UnregisterRunner("test/0")

	client, err := sockets.Dial(s.socketPath)
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	var result exec.ExecResponse
	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      -1,
		RemoteUnitName:  "",
		ForceRemoteUnit: false,
		UnitName:        "test/0",
	}
	err = client.Call(uniter.JujuExecEndpoint, args, &result)
	c.Assert(err, tc.ErrorMatches, ".*no runner is registered for unit test/0")
}

func (s *ListenerSuite) TestOperatorFlag(c *tc.C) {
	s.NewRunListener(c, true)

	client, err := sockets.Dial(s.socketPath)
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	var result exec.ExecResponse
	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      -1,
		RemoteUnitName:  "",
		ForceRemoteUnit: false,
		UnitName:        "test/0",
		Operator:        true,
	}
	err = client.Call(uniter.JujuExecEndpoint, args, &result)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(string(result.Stdout), tc.Equals, "some-command stdout")
	c.Assert(string(result.Stderr), tc.Equals, "some-command stderr")
	c.Assert(result.Code, tc.Equals, 42)
}

type ChannelCommandRunnerSuite struct {
	testing.BaseSuite
	abort          chan struct{}
	commands       runcommands.Commands
	commandChannel chan string
	runner         *uniter.ChannelCommandRunner
}

func TestChannelCommandRunnerSuite(t *tctesting.T) {
	tc.Run(t, &ChannelCommandRunnerSuite{})
}

func (s *ChannelCommandRunnerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.abort = make(chan struct{}, 1)
	s.commands = runcommands.NewCommands()
	s.commandChannel = make(chan string, 1)
	runner, err := uniter.NewChannelCommandRunner(uniter.ChannelCommandRunnerConfig{
		Abort:          s.abort,
		Commands:       s.commands,
		CommandChannel: s.commandChannel,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.runner = runner
}

func (s *ChannelCommandRunnerSuite) TestCommandsAborted(c *tc.C) {
	close(s.abort)
	_, err := s.runner.RunCommands(uniter.RunCommandsArgs{
		Commands: "some-command",
	})
	c.Assert(err, tc.ErrorMatches, "command execution aborted")
}

type mockCommandRunner struct {
	c        *tc.C
	operator bool
}

var _ uniter.CommandRunner = (*mockCommandRunner)(nil)

func (r *mockCommandRunner) RunCommands(args uniter.RunCommandsArgs) (results *exec.ExecResponse, err error) {
	r.c.Log("mock runner: " + args.Commands)
	r.c.Assert(args.Operator, tc.Equals, r.operator)
	return &exec.ExecResponse{
		Code:   42,
		Stdout: []byte(args.Commands + " stdout"),
		Stderr: []byte(args.Commands + " stderr"),
	}, nil
}
