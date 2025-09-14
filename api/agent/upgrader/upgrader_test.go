// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"path"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/upgrader"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type machineUpgraderSuite struct {
	testing.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	st *upgrader.State
}

func TestMachineUpgraderSuite(t *tctesting.T) {
	tc.Run(t, &machineUpgraderSuite{})
}

func (s *machineUpgraderSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	// Create the upgrader facade.
	s.st = upgrader.NewState(s.stateAPI)
	c.Assert(s.st, tc.NotNil)
}

// Note: This is really meant as a unit-test, this isn't a test that should
//
//	need all of the setup we have for this test suite
func (s *machineUpgraderSuite) TestNew(c *tc.C) {
	upgrader := upgrader.NewState(s.stateAPI)
	c.Assert(upgrader, tc.NotNil)
}

func (s *machineUpgraderSuite) TestSetVersionWrongMachine(c *tc.C) {
	err := s.st.SetVersion("machine-42", coretesting.CurrentVersion())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *machineUpgraderSuite) TestSetVersionNotMachine(c *tc.C) {
	err := s.st.SetVersion("foo-42", coretesting.CurrentVersion())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *machineUpgraderSuite) TestSetVersion(c *tc.C) {
	current := coretesting.CurrentVersion()
	agentTools, err := s.rawMachine.AgentTools()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(agentTools, tc.IsNil)
	err = s.st.SetVersion(s.rawMachine.Tag().String(), current)
	c.Assert(err, tc.ErrorIsNil)
	s.rawMachine.Refresh()
	agentTools, err = s.rawMachine.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agentTools.Version, tc.Equals, current)
}

func (s *machineUpgraderSuite) TestToolsWrongMachine(c *tc.C) {
	tools, err := s.st.Tools("machine-42")
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, tc.IsNil)
}

func (s *machineUpgraderSuite) TestToolsNotMachine(c *tc.C) {
	tools, err := s.st.Tools("foo-42")
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(tools, tc.IsNil)
}

func (s *machineUpgraderSuite) TestTools(c *tc.C) {
	current := coretesting.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)

	stateToolsList, err := s.st.Tools(s.rawMachine.Tag().String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(stateToolsList, tc.HasLen, 1)
	stateTools := stateToolsList[0]
	c.Assert(stateTools.Version, tc.Equals, current)
	url := s.stateAPI.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "model", coretesting.ModelTag.Id(), "tools", current.String())
	c.Assert(stateTools.URL, tc.Equals, url.String())
}

func (s *machineUpgraderSuite) TestWatchAPIVersion(c *tc.C) {
	w, err := s.st.WatchAPIVersion(s.rawMachine.Tag().String())
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event
	wc.AssertOneChange()

	// One change noticing the new version
	vers := version.MustParse("10.20.34")
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Setting the version to the same value doesn't trigger a change
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Another change noticing another new version
	vers = version.MustParse("10.20.35")
	err = statetesting.SetAgentVersion(s.BackingState, vers)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *machineUpgraderSuite) TestDesiredVersion(c *tc.C) {
	current := coretesting.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)

	stateVersion, err := s.st.DesiredVersion(s.rawMachine.Tag().String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stateVersion, tc.Equals, current.Number)
}
