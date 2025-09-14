// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type leaderGetSuite struct {
	testing.BaseSuite
	command cmd.Command
}

func TestLeaderGetSuite(t *tctesting.T) {
	tc.Run(t, &leaderGetSuite{})
}

func (s *leaderGetSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	var err error
	s.command, err = jujuc.NewLeaderGetCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.command = jujuc.NewJujucCommandWrappedForTest(s.command)
}

func (s *leaderGetSuite) TestInitError(c *tc.C) {
	err := s.command.Init([]string{"x=x"})
	c.Assert(err, tc.ErrorMatches, `invalid key "x=x"`)
}

func (s *leaderGetSuite) TestInitKey(c *tc.C) {
	err := s.command.Init([]string{"some-key"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaderGetSuite) TestInitAll(c *tc.C) {
	err := s.command.Init([]string{"-"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaderGetSuite) TestInitEmpty(c *tc.C) {
	err := s.command.Init(nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaderGetSuite) TestFormatError(c *tc.C) {
	runContext := cmdtesting.Context(c)
	code := cmd.Main(s.command, runContext, []string{"--format", "bad"})
	c.Check(code, tc.Equals, 2)
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, `ERROR invalid value "bad" for option --format: unknown format "bad"`+"\n")
}

func (s *leaderGetSuite) TestSettingsError(c *tc.C) {
	jujucContext := newLeaderGetContext(errors.New("zap"))
	command, err := jujuc.NewLeaderGetCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, nil)
	c.Check(code, tc.Equals, 1)
	c.Check(jujucContext.called, tc.IsTrue)
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, "ERROR cannot read leadership settings: zap\n")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultMissingKey(c *tc.C) {
	s.testOutput(c, []string{"unknown"}, "")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultKey(c *tc.C) {
	s.testOutput(c, []string{"key"}, "value\n")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultAll(c *tc.C) {
	s.testParseOutput(c, []string{"-"}, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatDefaultEmpty(c *tc.C) {
	s.testParseOutput(c, nil, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatSmartMissingKey(c *tc.C) {
	s.testOutput(c, []string{"--format", "smart", "unknown"}, "")
}

func (s *leaderGetSuite) TestSettingsFormatSmartKey(c *tc.C) {
	s.testOutput(c, []string{"--format", "smart", "key"}, "value\n")
}

func (s *leaderGetSuite) TestSettingsFormatSmartAll(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "smart", "-"}, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatSmartEmpty(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "smart"}, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatJSONMissingKey(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "json", "unknown"}, tc.JSONEquals, nil)
}

func (s *leaderGetSuite) TestSettingsFormatJSONKey(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "json", "key"}, tc.JSONEquals, "value")
}

func (s *leaderGetSuite) TestSettingsFormatJSONAll(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "json", "-"}, tc.JSONEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatJSONEmpty(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "json"}, tc.JSONEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatYAMLMissingKey(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "unknown"}, tc.YAMLEquals, nil)
}

func (s *leaderGetSuite) TestSettingsFormatYAMLKey(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "key"}, tc.YAMLEquals, "value")
}

func (s *leaderGetSuite) TestSettingsFormatYAMLAll(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "-"}, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatYAMLEmpty(c *tc.C) {
	s.testParseOutput(c, []string{"--format", "yaml"}, tc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) testOutput(c *tc.C, args []string, expect string) {
	s.testParseOutput(c, args, tc.Equals, expect)
}

func (s *leaderGetSuite) testParseOutput(c *tc.C, args []string, checker tc.Checker, expect interface{}) {
	jujucContext := newLeaderGetContext(nil)
	command, err := jujuc.NewLeaderGetCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, args)
	c.Check(code, tc.Equals, 0)
	c.Check(jujucContext.called, tc.IsTrue)
	c.Check(bufferString(runContext.Stdout), checker, expect)
	c.Check(bufferString(runContext.Stderr), tc.Equals, "")
}

func leaderGetSettings() map[string]string {
	return map[string]string{
		"key":    "value",
		"sample": "settings",
	}
}

func newLeaderGetContext(err error) *leaderGetContext {
	if err != nil {
		return &leaderGetContext{err: err}
	}
	return &leaderGetContext{settings: leaderGetSettings()}
}

type leaderGetContext struct {
	jujuc.Context
	called   bool
	settings map[string]string
	err      error
}

func (c *leaderGetContext) LeaderSettings() (map[string]string, error) {
	c.called = true
	return c.settings, c.err
}
