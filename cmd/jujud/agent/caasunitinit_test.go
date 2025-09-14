// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/juju/sockets"
)

type CAASUnitInitSuite struct {
	coretesting.BaseSuite
}

func TestCAASUnitInitSuite(t *tctesting.T) {
	tc.Run(t, &CAASUnitInitSuite{})
}

func (s *CAASUnitInitSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *CAASUnitInitSuite) newCommand(c *tc.C, st *testhelpers.Stub) *CAASUnitInitCommand {
	cmd := NewCAASUnitInitCommand()
	cmd.copyFunc = func(src, dst string) error {
		st.AddCall("Copy", src, dst)
		return st.NextErr()
	}
	cmd.symlinkFunc = func(src, dst string) error {
		st.AddCall("Symlink", src, dst)
		return st.NextErr()
	}
	cmd.removeAllFunc = func(path string) error {
		st.AddCall("RemoveAll", path)
		return st.NextErr()
	}
	cmd.mkdirAllFunc = func(path string, mode os.FileMode) error {
		st.AddCall("MkdirAll", path, mode)
		return st.NextErr()
	}
	cmd.statFunc = func(path string) (os.FileInfo, error) {
		st.AddCall("Stat", path)
		return nil, st.NextErr()
	}
	cmd.waitForPIDFunc = func(pid int) {
		st.AddCall("waitForPID", pid)
	}
	return cmd
}

func (s *CAASUnitInitSuite) checkCommand(c *tc.C, cmd *CAASUnitInitCommand, args []string,
	unit string, operatorFile string,
	operatorCACertFile string, charmDir string,
) []testhelpers.StubCall {
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx, tc.NotNil)

	toolsPath := "/var/lib/juju/tools/" + unit
	agentPath := "/var/lib/juju/agents/" + unit

	// Directory setup
	calls := []testhelpers.StubCall{
		{FuncName: "Stat", Args: []interface{}{"/var/lib/juju/tools/jujuc"}},
		{FuncName: "RemoveAll", Args: []interface{}{toolsPath}},
		{FuncName: "MkdirAll", Args: []interface{}{toolsPath, os.FileMode(0775)}},
	}

	calls = append(calls,
		testhelpers.StubCall{FuncName: "RemoveAll", Args: []interface{}{agentPath}},
		testhelpers.StubCall{FuncName: "MkdirAll", Args: []interface{}{agentPath, os.FileMode(0775)}},
	)

	// Symlinks
	calls = append(calls,
		testhelpers.StubCall{FuncName: "Symlink", Args: []interface{}{"/var/lib/juju/tools/jujud", toolsPath + "/jujud"}},
	)
	for _, cmdName := range jujuc.CommandNames() {
		_ = cmdName
		calls = append(calls,
			testhelpers.StubCall{FuncName: "Symlink", Args: []interface{}{"/var/lib/juju/tools/jujuc", toolsPath + "/" + cmdName}})
	}

	// Copies
	if operatorFile != "" {
		calls = append(calls,
			testhelpers.StubCall{FuncName: "Copy", Args: []interface{}{operatorFile, agentPath + "/operator-client.yaml"}},
		)
	}
	if operatorCACertFile != "" {
		calls = append(calls,
			testhelpers.StubCall{FuncName: "Copy", Args: []interface{}{operatorCACertFile, agentPath + "/ca.crt"}},
		)
	}
	if charmDir != "" {
		calls = append(calls,
			testhelpers.StubCall{FuncName: "Copy", Args: []interface{}{charmDir, agentPath + "/charm"}},
		)
	}

	return calls
}

func (s *CAASUnitInitSuite) TestInitUnit(c *tc.C) {
	args := []string{"--unit", "unit-wow-0",
		"--operator-file", "operator/file/path",
		"--operator-ca-cert-file", "operator/cert/file/path",
		"--charm-dir", "charm/dir"}
	st := &testhelpers.Stub{}
	cmd := s.newCommand(c, st)
	calls := s.checkCommand(c, cmd, args, "unit-wow-0",
		"operator/file/path", "operator/cert/file/path", "charm/dir")
	st.CheckCalls(c, calls)
}

func (s *CAASUnitInitSuite) TestInitUnitWaitSend(c *tc.C) {
	socketName := fmt.Sprintf("@%d", rand.Int63())
	listening := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		st := &testhelpers.Stub{}
		cmd := s.newCommand(c, st)
		cmd.socketName = socketName
		cmd.listenFunc = func(s sockets.Socket) (net.Listener, error) {
			l, err := sockets.Listen(s)
			close(listening)
			return l, err
		}
		calls := s.checkCommand(c, cmd, []string{"--wait"}, "unit-wow-0",
			"operator/file/path", "operator/cert/file/path", "charm/dir")
		calls = append(calls,
			testhelpers.StubCall{FuncName: "waitForPID", Args: []interface{}{os.Getpid()}})
		st.CheckCalls(c, calls)
	}()

	select {
	case <-listening:
	case <-time.After(coretesting.LongWait):
		c.Fatal("failed to listen")
	}

	stdErr := &bytes.Buffer{}
	args := []string{"--send", "--unit", "unit-wow-0",
		"--operator-file", "operator/file/path",
		"--operator-ca-cert-file", "operator/cert/file/path",
		"--charm-dir", "charm/dir"}
	st := &testhelpers.Stub{}
	cmd := s.newCommand(c, st)
	cmd.stdErr = stdErr
	cmd.socketName = socketName
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx, tc.NotNil)
	c.Assert(stdErr.Bytes(), tc.Not(tc.HasLen), 0)

	wg.Wait()
}

func (s *CAASUnitInitSuite) TestWaitPID(c *tc.C) {
	var cmd *exec.Cmd
	pid := 0
	cmd = exec.Command("sleep", "2")
	err := cmd.Start()
	c.Assert(err, tc.ErrorIsNil)
	pid = cmd.Process.Pid
	go func() {
		// Need this to reap the zombie process.
		_ = cmd.Wait()
	}()
	c.Assert(pid, tc.Not(tc.Equals), 0)
	waitChan := make(chan struct{})
	go func() {
		defer close(waitChan)
		waitForPID(pid)
	}()
	select {
	case <-waitChan:
	case <-time.After(testhelpers.LongWait):
		c.Errorf("waited too long for waitForPID")
	}
}
