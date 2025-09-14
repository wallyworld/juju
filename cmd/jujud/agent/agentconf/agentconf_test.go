// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconf_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestAgentConfSuite(t *tctesting.T) {
	tc.Run(t, &agentConfSuite{})
}

type agentConfSuite struct {
	coretesting.BaseSuite
}

func (s *agentConfSuite) TestChangeConfigSuccess(c *tc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := agentconf.NewAgentConfForTest(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcsw.WriteCalled, tc.IsTrue)
}

func (s *agentConfSuite) TestChangeConfigMutateFailure(c *tc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := agentconf.NewAgentConfForTest(c.MkDir(), mcsw)

	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return errors.New("blam")
	})

	c.Assert(err, tc.ErrorMatches, "blam")
	c.Assert(mcsw.WriteCalled, tc.IsFalse)
}

func (s *agentConfSuite) TestChangeConfigWriteFailure(c *tc.C) {
	mcsw := &mockConfigSetterWriter{
		WriteError: errors.New("boom"),
	}
	conf := agentconf.NewAgentConfForTest(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, tc.ErrorMatches, "cannot write agent configuration: boom")
	c.Assert(mcsw.WriteCalled, tc.IsTrue)
}

type mockConfigSetterWriter struct {
	agent.ConfigSetterWriter
	WriteError  error
	WriteCalled bool
}

func (c *mockConfigSetterWriter) Write() error {
	c.WriteCalled = true
	return c.WriteError
}
