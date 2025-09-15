// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jujuversion "github.com/juju/juju/version"
)

type unitUpgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawUnit    *state.Unit
	upgrader   *upgrader.UnitUpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

func TestUnitUpgraderSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &unitUpgraderSuite{})
}

func (s *unitUpgraderSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	// Create a machine and unit to work with
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.rawUnit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Assign the unit to the machine.
	s.rawMachine, err = s.rawUnit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)

	// The default auth is as the unit agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawUnit.Tag(),
	}
	s.upgrader, err = upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitUpgraderSuite) TearDownTest(c *tc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *unitUpgraderSuite) TestWatchAPIVersionNothing(c *tc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestWatchAPIVersion(c *tc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	results, err := s.upgrader.WatchAPIVersion(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Check(results.Results[0].Error, tc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Check(resource, tc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	err = s.rawMachine.SetAgentVersion(version.MustParseBinary("3.4.567.8-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitUpgraderSuite) TestUpgraderAPIRefusesNonUnitAgent(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("7")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, tc.NotNil)
	c.Check(anUpgrader, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *unitUpgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *tc.C) {
	// We are a unit agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.WatchAPIVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, tc.Equals, "")
	c.Assert(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsNothing(c *tc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Tools(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestToolsRefusesWrongAgent(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.Tools(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestToolsForAgent(c *tc.C) {
	agent := params.Entity{Tag: s.rawUnit.Tag().String()}

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and OSType without
	// having to pass it in again
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{agent}}
	results, err := s.upgrader.Tools(args)
	c.Assert(err, tc.ErrorIsNil)
	assertTools := func() {
		c.Check(results.Results, tc.HasLen, 1)
		c.Assert(results.Results[0].Error, tc.IsNil)
		agentTools := results.Results[0].ToolsList[0]
		c.Check(agentTools.Version.Number, tc.DeepEquals, jujuversion.Current)
		c.Assert(agentTools.URL, tc.NotNil)
	}
	assertTools()
}

func (s *unitUpgraderSuite) TestSetToolsNothing(c *tc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(params.EntitiesVersion{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestSetToolsRefusesWrongAgent(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag().String(),
			Tools: &params.Version{
				Version: testing.CurrentVersion(),
			},
		}},
	}

	results, err := anUpgrader.SetTools(args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestSetTools(c *tc.C) {
	cur := testing.CurrentVersion()
	_, err := s.rawUnit.AgentTools()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawUnit.Tag().String(),
			Tools: &params.Version{
				Version: cur,
			}},
		},
	}
	results, err := s.upgrader.SetTools(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	// Check that the new value actually got set, we must Refresh because
	// it was set on a different Machine object
	err = s.rawUnit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	realTools, err := s.rawUnit.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(realTools.Version.Arch, tc.Equals, cur.Arch)
	c.Check(realTools.Version.Release, tc.Equals, cur.Release)
	c.Check(realTools.Version.Major, tc.Equals, cur.Major)
	c.Check(realTools.Version.Minor, tc.Equals, cur.Minor)
	c.Check(realTools.Version.Patch, tc.Equals, cur.Patch)
	c.Check(realTools.Version.Build, tc.Equals, cur.Build)
	c.Check(realTools.URL, tc.Equals, "")
}

func (s *unitUpgraderSuite) TestDesiredVersionNothing(c *tc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *unitUpgraderSuite) TestDesiredVersionRefusesWrongAgent(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("wordpress/12354")
	anUpgrader, err := upgrader.NewUnitUpgraderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}},
	}
	results, err := anUpgrader.DesiredVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *unitUpgraderSuite) TestDesiredVersionNoticesMixedAgents(c *tc.C) {
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawUnit.Tag().String()},
		{Tag: "unit-wordpress-12345"},
	}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, jujuversion.Current)

	c.Assert(results.Results[1].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(results.Results[1].Version, tc.IsNil)

}

func (s *unitUpgraderSuite) TestDesiredVersionForAgent(c *tc.C) {
	current := testing.CurrentVersion()
	err := s.rawMachine.SetAgentVersion(current)
	c.Assert(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawUnit.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, jujuversion.Current)
}
