// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// uniterSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type uniterGoalStateSuite struct {
	uniterSuiteBase

	machine2 *state.Machine
	logging  *state.Application
}

func TestUniterGoalStateSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &uniterGoalStateSuite{})
}

func (s *uniterGoalStateSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.setupState(c)

	s.machine2 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})

	loggingCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
	})
	s.logging = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming the MySQL unit has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.mysqlUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.uniter = s.newUniterAPI(c, s.State, s.authorizer)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
}

var (
	timestamp          = time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)
	expectedUnitStatus = params.GoalStateStatus{
		Status: "waiting",
		Since:  &timestamp,
	}
	expectedRelationStatus = params.GoalStateStatus{
		Status: "joining",
		Since:  &timestamp,
	}
	expected2UnitsMysql = params.UnitsGoalState{
		"mysql/0": expectedUnitStatus,
		"mysql/1": expectedUnitStatus,
	}
	expectedUnitMysql = params.UnitsGoalState{
		"mysql/0": expectedUnitStatus,
	}
)

// TestGoalStatesNoRelation tests single application with single unit.
func (s *uniterGoalStateSuite) TestGoalStatesNoRelation(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitMysql,
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesNoRelationTwoUnits adds a new unit to wordpress application.
func (s *uniterGoalStateSuite) TestPeerUnitsNoRelation(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}

	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsMysql,
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesSingleRelation tests structure with two different
// application units and one relation between the units.
func (s *uniterGoalStateSuite) TestGoalStatesSingleRelation(c *tc.C) {

	err := s.addRelationEnterScope(c, s.mysqlUnit, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitMysql,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":   expectedRelationStatus,
							"wordpress/0": expectedUnitStatus,
						},
					},
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesDeadUnitsExcluded tests dead units should not show in the GoalState result.
func (s *uniterGoalStateSuite) TestGoalStatesDeadUnitsExcluded(c *tc.C) {

	err := s.addRelationEnterScope(c, s.wordpressUnit, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	newMysqlUnit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsMysql,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":   expectedRelationStatus,
							"wordpress/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})

	err = newMysqlUnit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: params.UnitsGoalState{
						"mysql/0": expectedUnitStatus,
					},
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":   expectedRelationStatus,
							"wordpress/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
}

// preventUnitDestroyRemove sets a non-allocating status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *tc.C, u *state.Unit) {
	// To have a non-allocating status, a unit needs to
	// be assigned to a machine.
	_, err := u.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		err = u.AssignToNewMachine()
	}
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
}

// TestGoalStatesSingleRelationDyingUnits tests dying units showing dying status in the GoalState result.
func (s *uniterGoalStateSuite) TestGoalStatesSingleRelationDyingUnits(c *tc.C) {
	mysqlUnit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	err := s.addRelationEnterScope(c, mysqlUnit, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsMysql,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":   expectedRelationStatus,
							"wordpress/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
	preventUnitDestroyRemove(c, mysqlUnit)
	err = mysqlUnit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: params.UnitsGoalState{
						"mysql/0": expectedUnitStatus,
						"mysql/1": params.GoalStateStatus{
							Status: "dying",
							Since:  &timestamp,
						},
					},
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":   expectedRelationStatus,
							"wordpress/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
}

// TestGoalStatesCrossModelRelation tests remote relation application shows URL as key.
func (s *uniterGoalStateSuite) TestGoalStatesCrossModelRelation(c *tc.C) {
	err := s.addRelationEnterScope(c, s.mysqlUnit, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{Results: []params.GoalStateResult{
		{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"mysql/0": expectedUnitStatus,
				},
				Relations: map[string]params.UnitsGoalState{
					"server": {
						"wordpress":   expectedRelationStatus,
						"wordpress/0": expectedUnitStatus,
					},
				},
			},
		},
	}})
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "metrics-remote",
		URL:         "ctrl1:admin/default.metrics",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "metrics",
			Name:      "metrics",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("mysql", "metrics-remote")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	testGoalStates(c, s.uniter, args, params.GoalStateResults{Results: []params.GoalStateResult{
		{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"mysql/0": expectedUnitStatus,
				},
				Relations: map[string]params.UnitsGoalState{
					"server": {
						"wordpress":   expectedRelationStatus,
						"wordpress/0": expectedUnitStatus,
					},
					"metrics-client": {
						"ctrl1:admin/default.metrics": expectedRelationStatus,
					},
				},
			},
		},
	}})
}

// TestGoalStatesMultipleRelations tests GoalStates with three
// applications one application has two units and each unit is related
// to a different application unit.
func (s *uniterGoalStateSuite) TestGoalStatesMultipleRelations(c *tc.C) {

	// Add another wordpress unit on machine 1.
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine1,
	})

	// And add another wordpress.
	wordpress2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress2",
		Charm: s.wpCharm,
	})
	wordpressUnit2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: wordpress2,
		Machine:     s.machine1,
	})

	mysqlCharm1 := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	mysql1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql1",
		Charm: mysqlCharm1,
	})

	mysqlUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: mysql1,
		Machine:     s.machine2,
	})

	err := s.addRelationEnterScope(c, s.wordpressUnit, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	err = s.addRelationEnterScope(c, wordpressUnit2, "mysql")
	c.Assert(err, tc.ErrorIsNil)

	err = s.addRelationEnterScope(c, s.mysqlUnit, "logging")
	c.Assert(err, tc.ErrorIsNil)
	err = s.addRelationEnterScope(c, mysqlUnit1, "logging")
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitMysql,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"wordpress":    expectedRelationStatus,
							"wordpress/0":  expectedUnitStatus,
							"wordpress/1":  expectedUnitStatus,
							"wordpress2":   expectedRelationStatus,
							"wordpress2/0": expectedUnitStatus,
						},
						"juju-info": {
							"logging":   expectedRelationStatus,
							"logging/0": expectedUnitStatus,
						},
					},
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}

	testGoalStates(c, s.uniter, args, expected)
}

func (s *uniterGoalStateSuite) addRelationEnterScope(c *tc.C, unit1 *state.Unit, app2 string) error {
	app1, err := unit1.Application()
	c.Assert(err, tc.ErrorIsNil)

	relation := s.addRelation(c, app1.Name(), app2)
	relationUnit, err := relation.Unit(unit1)
	c.Assert(err, tc.ErrorIsNil)

	err = relationUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertInScope(c, relationUnit, true)
	return err
}

func (s *uniterGoalStateSuite) addRelation(c *tc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	return rel
}

func (s *uniterGoalStateSuite) assertInScope(c *tc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.Equals, inScope)
}

// goalStates call uniter.GoalStates API and compares the output with the
// expected result.
func testGoalStates(c *tc.C, thisUniter *uniter.UniterAPI, args params.Entities, expected params.GoalStateResults) {
	result, err := thisUniter.GoalStates(args)
	c.Assert(err, tc.ErrorIsNil)
	for i := range result.Results {
		if result.Results[i].Error != nil {
			break
		}
		setSinceToNil(c, result.Results[i].Result)
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

// setSinceToNil will set the field `since` to nil in order to
// avoid a time check which otherwise would be impossible to pass.
func setSinceToNil(c *tc.C, goalState *params.GoalState) {

	for i, u := range goalState.Units {
		c.Assert(u.Since, tc.NotNil)
		u.Since = &timestamp
		goalState.Units[i] = u
	}
	for endPoint, gs := range goalState.Relations {
		for key, m := range gs {
			c.Assert(m.Since, tc.NotNil)
			m.Since = &timestamp
			gs[key] = m
		}
		goalState.Relations[endPoint] = gs
	}
}
