// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type leaderSetSuite struct {
	testhelpers.IsolationSuite
	command cmd.Command
}

func TestLeaderSetSuite(t *tctesting.T) {
	tc.Run(t, &leaderSetSuite{})
}

func (s *leaderSetSuite) SetUpTest(c *tc.C) {
	var err error
	s.command, err = jujuc.NewLeaderSetCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.command = jujuc.NewJujucCommandWrappedForTest(s.command)
}

func (s *leaderSetSuite) TestInitEmpty(c *tc.C) {
	err := s.command.Init(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitValues(c *tc.C) {
	err := s.command.Init([]string{"foo=bar", "baz=qux"})
	c.Check(err, tc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitError(c *tc.C) {
	err := s.command.Init([]string{"nonsense"})
	c.Check(err, tc.ErrorMatches, `expected "key=value", got "nonsense"`)
}

func (s *leaderSetSuite) TestWriteEmpty(c *tc.C) {
	jujucContext := &leaderSetContext{}
	command, err := jujuc.NewLeaderSetCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, nil)
	c.Check(code, tc.Equals, 0)
	c.Check(jujucContext.gotSettings, tc.DeepEquals, map[string]string{})
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, "")
}

func (s *leaderSetSuite) TestWriteValues(c *tc.C) {
	jujucContext := &leaderSetContext{}
	command, err := jujuc.NewLeaderSetCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"foo=bar", "baz=qux"})
	c.Check(code, tc.Equals, 0)
	c.Check(jujucContext.gotSettings, tc.DeepEquals, map[string]string{
		"foo": "bar",
		"baz": "qux",
	})
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, "")
}

func (s *leaderSetSuite) TestWriteError(c *tc.C) {
	jujucContext := &leaderSetContext{err: errors.New("splat")}
	command, err := jujuc.NewLeaderSetCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"foo=bar"})
	c.Check(code, tc.Equals, 1)
	c.Check(jujucContext.gotSettings, tc.DeepEquals, map[string]string{"foo": "bar"})
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, "ERROR cannot write leadership settings: splat\n")
}

type leaderSetContext struct {
	jujuc.Context
	gotSettings map[string]string
	err         error
}

func (s *leaderSetContext) WriteLeaderSettings(settings map[string]string) error {
	s.gotSettings = settings
	return s.err
}
