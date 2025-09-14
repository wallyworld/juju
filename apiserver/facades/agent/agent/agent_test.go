// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

func TestAgentSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &agentSuite{})
}

type agentSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machine0  *state.Machine
	machine1  *state.Machine
	container *state.Machine
}

func (s *agentSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)

	s.machine1, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine1.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}
}

func (s *agentSuite) TestAgentFailsWithNonAgent(c *tc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      auth,
	})
	c.Assert(err, tc.NotNil)
	c.Assert(api, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestAgentSucceedsWithUnitAgent(c *tc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	_, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      auth,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *agentSuite) TestGetEntities(c *tc.C) {
	err := s.container.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	results := api.GetEntities(args)
	c.Assert(results, tc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "alive",
				Jobs: []model.MachineJob{model.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetEntitiesContainer(c *tc.C) {
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	err := s.container.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      auth,
	})
	c.Assert(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxd-0"},
			{Tag: "machine-42"},
		},
	}
	results := api.GetEntities(args)
	c.Assert(results, tc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{
				Life:          "dying",
				Jobs:          []model.MachineJob{model.JobHostUnits},
				ContainerType: instance.LXD,
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetEntitiesNotFound(c *tc.C) {
	// Destroy the container first, so we can destroy its parent.
	err := s.container.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.container.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.container.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine1.Remove()
	c.Assert(err, tc.ErrorIsNil)

	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	results := api.GetEntities(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

func (s *agentSuite) TestSetPasswords(c *tc.C) {
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-0", Password: "xxx-12345678901234567890"},
			{Tag: "machine-1", Password: "yyy-12345678901234567890"},
			{Tag: "machine-42", Password: "zzz-12345678901234567890"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, tc.IsTrue)
}

func (s *agentSuite) TestSetPasswordsShort(c *tc.C) {
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-1", Password: "yyy"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}

func (s *agentSuite) TestClearReboot(c *tc.C) {
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine1.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine0.Tag().String()},
		{Tag: s.machine1.Tag().String()},
	}}

	rFlag, err := s.machine1.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsTrue)

	result, err := api.ClearReboot(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
		},
	})

	rFlag, err = s.machine1.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsFalse)
}

func (s *agentSuite) TestWatchCredentials(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/fred/default")
	result, err := api.WatchCredentials(params.Entities{Entities: []params.Entity{{Tag: tag.String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{"1", nil}}})
	c.Assert(s.resources.Count(), tc.Equals, 1)

	w := s.resources.Get("1")
	defer statetesting.AssertStop(c, w)

	// Check that the Watch has consumed the initial events ("returned" in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, w.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.State.UpdateCloudCredential(tag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	wc.AssertOneChange()
}

func (s *agentSuite) TestWatchAuthError(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	api, err := agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = api.WatchCredentials(params.Entities{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(s.resources.Count(), tc.Equals, 0)
}
