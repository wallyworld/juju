// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"encoding/json"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/actions"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type baseSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	action     *action.ActionAPI
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	charm         *state.Charm
	machine0      *state.Machine
	machine1      *state.Machine
	dummy         *state.Application
	wordpress     *state.Application
	mysql         *state.Application
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

type actionSuite struct {
	baseSuite
}

func TestActionSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &actionSuite{})
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*tc.C) { s.BlockHelper.Close() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	var err error
	s.action, err = action.NewActionAPI(s.State, s.resources, s.authorizer, action.FakeLeadership{})
	c.Assert(err, tc.ErrorIsNil)

	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
	})

	s.dummy = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "dummy",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "dummy",
		}),
	})
	s.wordpress = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.charm,
	})
	s.machine0 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	mysqlCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})
	s.machine1 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
}

func (s *actionSuite) TestActions(c *tc.C) {
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{}},
			{Receiver: s.wordpressUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"foo": 1, "bar": "please"}},
			{Receiver: s.mysqlUnit.Tag().String(), Name: "fakeaction", Parameters: map[string]interface{}{"baz": true}},
		}}

	r, err := s.action.EnqueueOperation(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Actions, tc.HasLen, len(arg.Actions))

	// There's only one operation created.
	operations, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 1)
	c.Assert(operations[0].Summary(), tc.Equals, "fakeaction run on unit-wordpress-0,unit-mysql-0,unit-wordpress-0,unit-mysql-0")

	emptyActionTag := names.ActionTag{}
	for i, got := range r.Actions {
		c.Assert(got.Action, tc.NotNil)
		c.Logf("check index %d (%s: %s)", i, got.Action.Tag, arg.Actions[i].Name)
		c.Assert(got.Error, tc.Equals, (*params.Error)(nil))
		c.Assert(got.Action, tc.Not(tc.Equals), (*params.Action)(nil))
		c.Assert(got.Action.Tag, tc.Not(tc.Equals), emptyActionTag)
		c.Assert(got.Action.Name, tc.Equals, arg.Actions[i].Name)
		c.Assert(got.Action.Receiver, tc.Equals, arg.Actions[i].Receiver)
		c.Assert(got.Action.Parameters, tc.DeepEquals, arg.Actions[i].Parameters)
		c.Assert(got.Status, tc.Equals, params.ActionPending)
		c.Assert(got.Message, tc.Equals, "")
		c.Assert(got.Output, tc.IsNil)
	}
}

func (s *actionSuite) TestCancel(c *tc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// Make sure no Actions already exist on mysql Unit.
	actions, err = s.mysqlUnit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// Add Actions.
	tests := params.Actions{
		Actions: []params.Action{{
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.mysqlUnit.Tag().String(),
			Name:     "fakeaction",
		}, {
			Receiver: s.mysqlUnit.Tag().String(),
			Name:     "fakeaction",
		}},
	}

	results, err := s.action.EnqueueOperation(tests)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Actions, tc.HasLen, 4)
	for _, res := range results.Actions {
		c.Assert(res.Error, tc.IsNil)
	}

	// blocking changes should have no effect
	s.BlockAllChanges(c, "Cancel")

	// Cancel Some.
	arg := params.Entities{
		Entities: []params.Entity{
			// "wp-two"
			{Tag: results.Actions[1].Action.Tag},
			// "my-one"
			{Tag: results.Actions[2].Action.Tag},
		}}
	cancelled, err := s.action.Cancel(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cancelled.Results, tc.HasLen, 2)

	// Assert the Actions are all in the expected state.
	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Units: []string{
			s.wordpressUnit.Name(),
			s.mysqlUnit.Name(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations.Results, tc.HasLen, 1)

	resultActions := operations.Results[0].Actions
	c.Assert(resultActions, tc.HasLen, 4)
	c.Assert(resultActions[0].Action.Name, tc.Equals, "fakeaction")
	c.Assert(resultActions[0].Status, tc.Equals, params.ActionPending)
	c.Assert(resultActions[1].Action.Name, tc.Equals, "fakeaction")
	c.Assert(resultActions[1].Status, tc.Equals, params.ActionCancelled)
	c.Assert(resultActions[2].Action.Name, tc.Equals, "fakeaction")
	c.Assert(resultActions[2].Status, tc.Equals, params.ActionCancelled)
	c.Assert(resultActions[3].Action.Name, tc.Equals, "fakeaction")
	c.Assert(resultActions[3].Status, tc.Equals, params.ActionPending)
}

func (s *actionSuite) TestAbort(c *tc.C) {
	// Make sure no Actions already exist on wordpress Unit.
	actions, err := s.wordpressUnit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// Add Actions.
	tests := params.Actions{
		Actions: []params.Action{{
			Receiver: s.wordpressUnit.Tag().String(),
			Name:     "fakeaction",
		}},
	}

	results, err := s.action.EnqueueOperation(tests)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Actions, tc.HasLen, 1)
	c.Assert(results.Actions[0].Error, tc.IsNil)

	actions, err = s.wordpressUnit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 1)

	_, err = actions[0].Begin()
	c.Assert(err, tc.ErrorIsNil)

	// blocking changes should have no effect
	s.BlockAllChanges(c, "Cancel")

	// Cancel Some.
	arg := params.Entities{
		Entities: []params.Entity{
			// "wp-one"
			{Tag: results.Actions[0].Action.Tag},
		}}
	cancelled, err := s.action.Cancel(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cancelled.Results, tc.HasLen, 1)
	c.Assert(cancelled.Results[0].Action.Name, tc.Equals, "fakeaction")
	c.Assert(cancelled.Results[0].Status, tc.Equals, params.ActionAborting)

	// Assert the Actions are all in the expected state.
	operations, err := s.action.ListOperations(params.OperationQueryArgs{
		Units: []string{s.wordpressUnit.Name()},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations.Results, tc.HasLen, 1)

	wpActions := operations.Results[0].Actions
	c.Assert(wpActions, tc.HasLen, 1)
	c.Assert(wpActions[0].Action.Name, tc.Equals, "fakeaction")
	c.Assert(wpActions[0].Status, tc.Equals, params.ActionAborting)
}

func (s *actionSuite) TestApplicationsCharmsActions(c *tc.C) {
	actionSchemas := map[string]map[string]interface{}{
		"snapshot": {
			"type":        "object",
			"title":       "snapshot",
			"description": "Take a snapshot of the database.",
			"properties": map[string]interface{}{
				"outfile": map[string]interface{}{
					"description": "The file to write out to.",
					"type":        "string",
					"default":     "foo.bz2",
				},
			},
		},
		"fakeaction": {
			"type":        "object",
			"title":       "fakeaction",
			"description": "No description",
			"properties":  map[string]interface{}{},
		},
	}
	tests := []struct {
		applicationNames []string
		expectedResults  params.ApplicationsCharmActionsResults
	}{{
		applicationNames: []string{"dummy"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("dummy").String(),
					Actions: map[string]params.ActionSpec{
						"snapshot": {
							Description: "Take a snapshot of the database.",
							Params:      actionSchemas["snapshot"],
						},
					},
				},
			},
		},
	}, {
		applicationNames: []string{"wordpress"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("wordpress").String(),
					Actions: map[string]params.ActionSpec{
						"fakeaction": {
							Description: "No description",
							Params:      actionSchemas["fakeaction"],
						},
					},
				},
			},
		},
	}, {
		applicationNames: []string{"nonsense"},
		expectedResults: params.ApplicationsCharmActionsResults{
			Results: []params.ApplicationCharmActionsResult{
				{
					ApplicationTag: names.NewApplicationTag("nonsense").String(),
					Error: &params.Error{
						Message: `application "nonsense" not found`,
						Code:    "not found",
					},
				},
			},
		},
	}}

	for i, t := range tests {
		c.Logf("test %d: applications: %#v", i, t.applicationNames)

		svcTags := params.Entities{
			Entities: make([]params.Entity, len(t.applicationNames)),
		}

		for j, app := range t.applicationNames {
			svcTag := names.NewApplicationTag(app)
			svcTags.Entities[j] = params.Entity{Tag: svcTag.String()}
		}

		results, err := s.action.ApplicationsCharmsActions(svcTags)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(results.Results, tc.DeepEquals, t.expectedResults.Results)
	}
}

func assertReadyToTest(c *tc.C, receiver state.ActionReceiver) {
	// make sure there are no actions on the receiver already.
	actions, err := receiver.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// make sure there are no actions pending already.
	actions, err = receiver.PendingActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// make sure there are no actions running already.
	actions, err = receiver.RunningActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)

	// make sure there are no actions completed already.
	actions, err = receiver.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, 0)
}

func (s *actionSuite) TestWatchActionProgress(c *tc.C) {
	unit, err := s.State.Unit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	assertReadyToTest(c, unit)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	added, err := s.Model.AddAction(unit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	w, err := s.action.WatchActionsProgress(
		params.Entities{Entities: []params.Entity{{Tag: "action-2"}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w.Results, tc.HasLen, 1)
	c.Assert(w.Results[0].Error, tc.IsNil)
	c.Assert(w.Results[0].Changes, tc.HasLen, 0)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Log a message and check the watcher result.
	added, err = added.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = added.Log("hello")
	c.Assert(err, tc.ErrorIsNil)

	a, err := s.Model.Action("2")
	c.Assert(err, tc.ErrorIsNil)
	logged := a.Messages()
	c.Assert(logged, tc.HasLen, 1)
	expected, err := json.Marshal(actions.ActionMessage{
		Message:   logged[0].Message(),
		Timestamp: logged[0].Timestamp(),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(string(expected))
	wc.AssertNoChange()
}
