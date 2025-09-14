// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/actions"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	statetesting "github.com/juju/juju/state/testing"
)

type ActionSuite struct {
	ConnSuite
	charm                 *state.Charm
	actionlessCharm       *state.Charm
	application           *state.Application
	actionlessApplication *state.Application
	unit                  *state.Unit
	unit2                 *state.Unit
	charmlessUnit         *state.Unit
	actionlessUnit        *state.Unit
	model                 *state.Model
}

func TestActionSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ActionSuite{})
}

func (s *ActionSuite) SetUpTest(c *tc.C) {
	var err error

	s.ConnSuite.SetUpTest(c)

	s.charm = s.AddTestingCharm(c, "dummy")
	s.actionlessCharm = s.AddTestingCharm(c, "actionless")

	s.application = s.AddTestingApplication(c, "dummy", s.charm)
	c.Assert(err, tc.ErrorIsNil)
	s.actionlessApplication = s.AddTestingApplication(c, "actionless", s.actionlessCharm)
	c.Assert(err, tc.ErrorIsNil)

	sURL, _ := s.application.CharmURL()
	c.Assert(sURL, tc.NotNil)
	actionlessSURL, _ := s.actionlessApplication.CharmURL()
	c.Assert(actionlessSURL, tc.NotNil)

	s.unit, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.unit.Base(), tc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})

	err = s.unit.SetCharmURL(*sURL)
	c.Assert(err, tc.ErrorIsNil)

	s.unit2, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.unit2.Base(), tc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})

	err = s.unit2.SetCharmURL(*sURL)
	c.Assert(err, tc.ErrorIsNil)

	s.charmlessUnit, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.charmlessUnit.Base(), tc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})

	s.actionlessUnit, err = s.actionlessApplication.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.actionlessUnit.Base(), tc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})

	err = s.actionlessUnit.SetCharmURL(*actionlessSURL)
	c.Assert(err, tc.ErrorIsNil)

	s.model, err = s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ActionSuite) TestActionTag(c *tc.C) {
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	action, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	tag := action.Tag()
	c.Assert(tag.String(), tc.Equals, "action-"+action.Id())

	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, tc.ErrorIsNil)

	actions, err := s.unit.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(actions), tc.Equals, 1)

	actionResult := actions[0]
	c.Assert(actionResult, tc.DeepEquals, result)

	tag = actionResult.Tag()
	c.Assert(tag.String(), tc.Equals, "action-"+actionResult.Id())
}

func (s *ActionSuite) TestAddAction(c *tc.C) {
	for i, t := range []struct {
		should         string
		name           string
		params         map[string]interface{}
		parallel       bool
		executionGroup string
		whichUnit      *state.Unit
		expectedErr    string
	}{{
		should:         "enqueue normally",
		name:           "snapshot",
		whichUnit:      s.unit,
		params:         map[string]interface{}{"outfile": "outfile.tar.bz2"},
		parallel:       true,
		executionGroup: "group",
	}, {
		should:      "fail on actionless charms",
		name:        "something",
		whichUnit:   s.actionlessUnit,
		expectedErr: "no actions defined on charm \"local:quantal/quantal-actionless-1\"",
	}, {
		should:      "fail on action not defined in schema",
		whichUnit:   s.unit,
		name:        "something-nonexistent",
		expectedErr: "action \"something-nonexistent\" not defined on unit \"dummy/0\"",
	}, {
		should:    "invalidate with bad params",
		whichUnit: s.unit,
		name:      "snapshot",
		params: map[string]interface{}{
			"outfile": 5.0,
		},
		expectedErr: "validation failed: \\(root\\)\\.outfile : must be of type string, given 5",
	}} {
		c.Logf("Test %d: should %s", i, t.should)
		before := state.NowToTheSecond(s.State)
		later := before.Add(coretesting.LongWait)

		// Copy params over into empty premade map for comparison later
		params := make(map[string]interface{})
		for k, v := range t.params {
			params[k] = v
		}

		// Verify we can add an Action
		operationID, err := s.Model.EnqueueOperation("a test", 1)
		c.Assert(err, tc.ErrorIsNil)
		a, err := s.Model.AddAction(t.whichUnit, operationID, t.name, params, &t.parallel, &t.executionGroup)

		if t.expectedErr == "" {
			c.Assert(err, tc.ErrorIsNil)
			curl := t.whichUnit.CharmURL()
			c.Assert(curl, tc.NotNil)
			ch, _ := s.State.Charm(*curl)
			schema := ch.Actions()
			c.Logf("Schema for unit %q:\n%#v", t.whichUnit.Name(), schema)

			// verify we can get it back out by Id
			model, err := s.State.Model()
			c.Assert(err, tc.ErrorIsNil)

			action, err := model.Action(a.Id())
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(action, tc.NotNil)
			c.Check(action.Id(), tc.Equals, a.Id())
			c.Check(state.ActionOperationId(action), tc.Equals, operationID)

			// verify we get out what we put in
			c.Check(action.Name(), tc.Equals, t.name)
			c.Check(action.Parameters(), tc.DeepEquals, params)
			c.Check(action.Parallel(), tc.Equals, t.parallel)
			c.Check(action.ExecutionGroup(), tc.Equals, t.executionGroup)

			// Enqueued time should be within a reasonable time of the beginning
			// of the test
			now := state.NowToTheSecond(s.State)
			c.Check(action.Enqueued(), tc.TimeBetween(before, now))
			c.Check(action.Enqueued(), tc.TimeBetween(before, later))
			continue
		}

		c.Check(err, tc.ErrorMatches, t.expectedErr)
	}
}

func (s *ActionSuite) TestAddActionInsertsDefaults(c *tc.C) {
	units := make(map[string]*state.Unit)
	schemas := map[string]string{
		"simple": `
act:
  params:
    val:
      type: string
      default: somestr
`[1:],
		"complicated": `
act:
  params:
    val:
      type: object
      properties:
        foo:
          type: string
        bar:
          type: object
          properties:
            baz:
              type: string
              default: woz
`[1:],
		"none": `
act:
  params:
    val:
      type: string
`[1:]}

	// Prepare the units for this test
	makeUnits(c, s, units, schemas)

	for i, t := range []struct {
		should         string
		params         map[string]interface{}
		schema         string
		expectedParams map[string]interface{}
	}{{
		should:         "do nothing with no defaults",
		params:         map[string]interface{}{},
		schema:         "none",
		expectedParams: map[string]interface{}{},
	}, {
		should: "insert a simple default value",
		params: map[string]interface{}{"foo": "bar"},
		schema: "simple",
		expectedParams: map[string]interface{}{
			"foo": "bar",
			"val": "somestr",
		},
	}, {
		should: "insert a default value when an empty map is passed",
		params: map[string]interface{}{},
		schema: "simple",
		expectedParams: map[string]interface{}{
			"val": "somestr",
		},
	}, {
		should: "insert a default value when a nil map is passed",
		params: nil,
		schema: "simple",
		expectedParams: map[string]interface{}{
			"val": "somestr",
		},
	}, {
		should: "insert a nested default value",
		params: map[string]interface{}{"foo": "bar"},
		schema: "complicated",
		expectedParams: map[string]interface{}{
			"foo": "bar",
			"val": map[string]interface{}{
				"bar": map[string]interface{}{
					"baz": "woz",
				}}},
	}} {
		c.Logf("test %d: should %s", i, t.should)
		u := units[t.schema]
		// Note that AddAction will only result in errors in the case
		// of malformed schemas, and schema objects can only be
		// created from valid schemas.  The error handling for this
		// is tested in the gojsonschema package.
		operationID, err := s.Model.EnqueueOperation("a test", 1)
		c.Assert(err, tc.ErrorIsNil)
		action, err := s.Model.AddAction(u, operationID, "act", t.params, nil, nil)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(action.Parameters(), tc.DeepEquals, t.expectedParams)
		c.Check(action.Parallel(), tc.IsFalse)
		c.Check(action.ExecutionGroup(), tc.Equals, "")
	}
}

func (s *ActionSuite) TestActionBeginStartsOperation(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	anAction2, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	operation, err := s.model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
	c.Assert(operation.Started(), tc.Equals, anAction.Started())

	// Starting a second action does not affect the original start time.
	clock.Advance(5 * time.Second)
	anAction2, err = anAction2.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = operation.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
	c.Assert(operation.Started(), tc.Equals, anAction.Started())
	c.Assert(operation.Started(), tc.Not(tc.Equals), anAction2.Started())
}

func (s *ActionSuite) TestActionBeginStartsOperationRace(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	anAction2, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		clock.Advance(5 * time.Second)
		anAction2, err = anAction2.Begin()
		c.Assert(err, tc.ErrorIsNil)
	})()

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	operation, err := s.model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
	c.Assert(operation.Started(), tc.Equals, anAction2.Started())
	c.Assert(operation.Started(), tc.Not(tc.Equals), anAction.Started())
}

func (s *ActionSuite) TestLastActionFinishCompletesOperation(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	anAction2, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	clock.Advance(5 * time.Second)
	anAction2, err = anAction2.Begin()
	c.Assert(err, tc.ErrorIsNil)

	// Finishing only one action does not complete the operation.
	_, err = anAction.Finish(state.ActionResults{
		Status: state.ActionFailed,
	})
	c.Assert(err, tc.ErrorIsNil)
	operation, err := s.model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
	c.Assert(operation.Completed(), tc.Equals, time.Time{})

	clock.Advance(5 * time.Second)
	anAction2, err = anAction2.Finish(state.ActionResults{
		Status: state.ActionCompleted,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = operation.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	// Failed task precedence over completed.
	c.Assert(operation.Status(), tc.Equals, state.ActionFailed)
	c.Assert(operation.Completed(), tc.Equals, anAction2.Completed())
}

func (s *ActionSuite) TestLastActionFinishCompletesOperationMany(c *tc.C) {
	numActions := 50

	operationID, err := s.Model.EnqueueOperation("a test", numActions)
	c.Assert(err, tc.ErrorIsNil)

	wg := sync.WaitGroup{}
	var actions []state.Action
	for i := 0; i < numActions; i++ {
		anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
		c.Assert(err, tc.ErrorIsNil)

		anAction, err = anAction.Begin()
		c.Assert(err, tc.ErrorIsNil)
		actions = append(actions, anAction)
		wg.Add(1)
	}

	completeCount := int32(0)
	for i := 0; i < numActions; i++ {
		go func(a int) {
			defer func() {
				atomic.AddInt32(&completeCount, 1)
				wg.Done()
			}()
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
			if atomic.LoadInt32(&completeCount) < int32(numActions) {
				operation, err := s.model.Operation(operationID)
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(operation.Status(), tc.Not(tc.Equals), state.ActionCompleted)
			}

			_, err := actions[a].Finish(state.ActionResults{
				Status: state.ActionCompleted,
			})
			c.Assert(err, tc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	operation, err := s.model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionCompleted)
}

func (s *ActionSuite) TestLastActionFinishCompletesOperationRace(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	anAction2, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	clock.Advance(5 * time.Second)
	anAction2, err = anAction2.Begin()
	c.Assert(err, tc.ErrorIsNil)

	operation, err := s.model.Operation(operationID)
	defer state.SetBeforeHooks(c, s.State, func() {
		//clock.Advance(5 * time.Second)
		anAction2, err = anAction2.Finish(state.ActionResults{
			Status: state.ActionCancelled,
		})
		c.Assert(err, tc.ErrorIsNil)
		err = operation.Refresh()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
		c.Assert(operation.Completed(), tc.Equals, time.Time{})
	})()

	// Finishing does complete the operation due to the other action
	// finishing during the update of this one.
	anAction, err = anAction.Finish(state.ActionResults{
		Status: state.ActionCompleted,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = operation.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionCancelled)
	c.Assert(operation.Completed(), tc.Equals, anAction.Completed())
}

func (s *ActionSuite) TestActionMessages(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(anAction.Messages(), tc.HasLen, 0)

	// Cannot log messages until action is running.
	err = anAction.Log("hello")
	c.Assert(err, tc.ErrorMatches, `cannot log message to task "2" with status pending`)

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	messages := []string{"one", "two", "three"}
	for i, msg := range messages {
		err = anAction.Log(msg)
		c.Assert(err, tc.ErrorIsNil)

		a, err := s.Model.Action(anAction.Id())
		c.Assert(err, tc.ErrorIsNil)
		obtained := a.Messages()
		c.Assert(obtained, tc.HasLen, i+1)
		for j, am := range obtained {
			c.Assert(am.Timestamp(), tc.Equals, clock.Now().UTC())
			c.Assert(am.Message(), tc.Equals, messages[j])
		}
	}

	// Cannot log messages after action finishes.
	_, err = anAction.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, tc.ErrorIsNil)
	err = anAction.Log("hello")
	c.Assert(err, tc.ErrorMatches, `cannot log message to task "2" with status completed`)
}

func (s *ActionSuite) TestActionLogMessageRace(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	anAction, err := s.Model.AddAction(s.unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(anAction.Messages(), tc.HasLen, 0)

	anAction, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err = anAction.Finish(state.ActionResults{Status: state.ActionCompleted})
		c.Assert(err, tc.ErrorIsNil)
	})()

	err = anAction.Log("hello")
	c.Assert(err, tc.ErrorMatches, `cannot log message to task "2" with status completed`)
}

// makeUnits prepares units with given Action schemas
func makeUnits(c *tc.C, s *ActionSuite, units map[string]*state.Unit, schemas map[string]string) {
	// A few dummy charms that haven't been used yet
	freeCharms := map[string]string{
		"simple":      "mysql",
		"complicated": "mysql-alternative",
		"none":        "wordpress",
	}

	for name, schema := range schemas {
		appName := name + "-defaults-application"

		// Add a testing application
		ch := s.AddActionsCharm(c, freeCharms[name], schema, 1)
		app := s.AddTestingApplication(c, appName, ch)

		// Get its charm URL
		sURL, _ := app.CharmURL()
		c.Assert(sURL, tc.NotNil)

		// Add a unit
		var err error
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(u.Base(), tc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})
		err = u.SetCharmURL(*sURL)
		c.Assert(err, tc.ErrorIsNil)

		units[name] = u
	}
}

func (s *ActionSuite) TestEnqueueAction(c *tc.C) {
	// verify can not enqueue an Action without a name
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	params := map[string]interface{}{"foo": "bar"}
	a, err := s.model.EnqueueAction(operationID, s.unit.Tag(), "test", params, true, "group", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a.Name(), tc.Equals, "test")
	c.Assert(a.Parameters(), tc.DeepEquals, params)
	c.Assert(a.Receiver(), tc.Equals, s.unit.Name())
	c.Assert(a.Parallel(), tc.IsTrue)
	c.Assert(a.ExecutionGroup(), tc.Equals, "group")
}

func (s *ActionSuite) TestEnqueueActionRequiresName(c *tc.C) {
	name := ""

	// verify can not enqueue an Action without a name
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.model.EnqueueAction(operationID, s.unit.Tag(), name, nil, false, "", nil)
	c.Assert(err, tc.ErrorMatches, "action name required")
}

func (s *ActionSuite) TestEnqueueActionRequiresValidOperation(c *tc.C) {
	_, err := s.model.EnqueueAction("666", s.unit.Tag(), "test", nil, false, "", nil)
	c.Assert(err, tc.ErrorMatches, `operation "666" not found`)
}

func (s *ActionSuite) TestAddActionAcceptsDuplicateNames(c *tc.C) {
	name := "snapshot"
	params1 := map[string]interface{}{"outfile": "outfile.tar.bz2"}
	params2 := map[string]interface{}{"infile": "infile.zip"}

	// verify can add two actions with same name
	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	a1, err := s.Model.AddAction(s.unit, operationID, name, params1, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	a2, err := s.Model.AddAction(s.unit, operationID, name, params2, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(a1.Id(), tc.Not(tc.Equals), a2.Id())

	// verify both actually got added
	actions, err := s.unit.PendingActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(actions), tc.Equals, 2)

	// verify we can Fail one, retrieve the other, and they're not mixed up
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	action1, err := model.Action(a1.Id())
	c.Assert(err, tc.ErrorIsNil)
	_, err = action1.Finish(state.ActionResults{Status: state.ActionFailed})
	c.Assert(err, tc.ErrorIsNil)

	action2, err := model.Action(a2.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(action2.Parameters(), tc.DeepEquals, params2)

	// verify only one left, and it's the expected one
	actions, err = s.unit.PendingActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(actions), tc.Equals, 1)
	c.Assert(actions[0].Id(), tc.Equals, a2.Id())
}

func (s *ActionSuite) TestAddActionLifecycle(c *tc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	// make unit state Dying
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// can add action to a dying unit
	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.Model.AddAction(unit, operationID, "snapshot", map[string]interface{}{}, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// make sure unit is dead
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// cannot add action to a dead unit
	_, err = s.Model.AddAction(unit, operationID, "snapshot", map[string]interface{}{}, nil, nil)
	c.Assert(err, tc.Equals, stateerrors.ErrDead)
}

func (s *ActionSuite) TestAddActionFailsOnDeadUnitInTransaction(c *tc.C) {
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	killUnit := jujutxn.TestHook{
		Before: func() {
			c.Assert(unit.Destroy(), tc.IsNil)
			c.Assert(unit.EnsureDead(), tc.IsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, killUnit).Check()

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.Model.AddAction(unit, operationID, "snapshot", map[string]interface{}{}, nil, nil)
	c.Assert(err, tc.Equals, stateerrors.ErrDead)
}

func (s *ActionSuite) TestFail(c *tc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	a, err := s.Model.AddAction(unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	action, err := model.Action(a.Id())
	c.Assert(err, tc.ErrorIsNil)

	// ensure no action results for this action
	results, err := unit.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results), tc.Equals, 0)

	// fail the action, and verify that it succeeds
	reason := "test fail reason"
	result, err := action.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, tc.ErrorIsNil)

	// ensure we now have a result for this action
	results, err = unit.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.DeepEquals, result)

	c.Assert(results[0].Name(), tc.Equals, action.Name())
	c.Assert(results[0].Status(), tc.Equals, state.ActionFailed)

	// Verify the Action Completed time was within a reasonable
	// time of the Enqueued time.
	diff := results[0].Completed().Sub(action.Enqueued())
	c.Assert(diff >= 0, tc.IsTrue)
	c.Assert(diff < coretesting.LongWait, tc.IsTrue)

	res, errstr := results[0].Results()
	c.Assert(errstr, tc.Equals, reason)
	c.Assert(res, tc.DeepEquals, map[string]interface{}{})

	// validate that a pending action is no longer returned by UnitActions.
	actions, err := unit.PendingActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(actions), tc.Equals, 0)
}

func (s *ActionSuite) TestErrorAfterEnqueuingFail(c *tc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	unit2, err := s.State.Unit(s.unit2.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit2)

	operationID, err := s.Model.EnqueueOperation("enqueuing test", 3)
	c.Assert(err, tc.ErrorIsNil)
	a, err := s.Model.AddAction(unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	a2, err := s.Model.AddAction(unit2, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.model.FailOperationEnqueuing(operationID, "fail for test", 2)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	action, err := model.Action(a.Id())
	c.Assert(err, tc.ErrorIsNil)
	action2, err := model.Action(a2.Id())
	c.Assert(err, tc.ErrorIsNil)

	// complete the action, and verify that it succeeds
	output := map[string]interface{}{"output": "action ran successfully"}
	_, err = action.Finish(state.ActionResults{Status: state.ActionCompleted, Results: output})
	c.Assert(err, tc.ErrorIsNil)
	_, err = action2.Finish(state.ActionResults{Status: state.ActionCompleted, Results: output})
	c.Assert(err, tc.ErrorIsNil)

	operation, err := s.model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionError)
	c.Assert(operation.Fail(), tc.Equals, "fail for test")
}

func (s *ActionSuite) TestComplete(c *tc.C) {
	// get unit, add an action, retrieve that action
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	a, err := s.Model.AddAction(unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	action, err := model.Action(a.Id())
	c.Assert(err, tc.ErrorIsNil)

	// ensure no action results for this action
	results, err := unit.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results), tc.Equals, 0)

	// complete the action, and verify that it succeeds
	output := map[string]interface{}{"output": "action ran successfully"}
	result, err := action.Finish(state.ActionResults{Status: state.ActionCompleted, Results: output})
	c.Assert(err, tc.ErrorIsNil)

	// ensure we now have a result for this action
	results, err = unit.CompletedActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.DeepEquals, result)

	c.Assert(results[0].Name(), tc.Equals, action.Name())
	c.Assert(results[0].Status(), tc.Equals, state.ActionCompleted)
	res, errstr := results[0].Results()
	c.Assert(errstr, tc.Equals, "")
	c.Assert(res, tc.DeepEquals, output)

	// validate that a pending action is no longer returned by UnitActions.
	actions, err := unit.PendingActions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(actions), tc.Equals, 0)
}

func (s *ActionSuite) TestFindActionsByName(c *tc.C) {
	actions := []struct {
		Name       string
		Parameters map[string]interface{}
	}{
		{Name: "action-1", Parameters: map[string]interface{}{}},
		{Name: "fake", Parameters: map[string]interface{}{"yeah": true, "take": nil}},
		{Name: "action-1", Parameters: map[string]interface{}{"yeah": true, "take": nil}},
		{Name: "action-9", Parameters: map[string]interface{}{"district": 9}},
		{Name: "blarney", Parameters: map[string]interface{}{"conversation": []string{"what", "now"}}},
	}

	operationID, err := s.Model.EnqueueOperation("a test", len(actions))
	c.Assert(err, tc.ErrorIsNil)
	for _, action := range actions {
		_, err := s.model.EnqueueAction(operationID, s.unit.Tag(), action.Name, action.Parameters, false, "", nil)
		c.Assert(err, tc.Equals, nil)
	}

	results, err := s.model.FindActionsByName("action-1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(results), tc.Equals, 2)
	for _, result := range results {
		c.Check(result.Name(), tc.Equals, "action-1")
	}
}

func (s *ActionSuite) TestActionsWatcherEmitsInitialChanges(c *tc.C) {
	// LP-1391914 :: idPrefixWatcher fails watcher contract to send
	// initial Change event
	//
	// state/idPrefixWatcher does not send an initial event in response
	// to the first time Changes() is called if all of the pending
	// events are removed before the first consumption of Changes().
	// The watcher contract specifies that the first call to Changes()
	// should always return at a minimum an empty change set to notify
	// clients of it's initial state

	// preamble
	app := s.AddTestingApplication(c, "dummy3", s.charm)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	u, err := s.State.Unit(unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, u)

	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	// queue up actions
	a1, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	a2, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// start watcher but don't consume Changes() yet
	w := u.WatchPendingActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	// remove actions
	reason := "removed"
	_, err = a1.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, tc.ErrorIsNil)
	_, err = a2.Finish(state.ActionResults{Status: state.ActionFailed, Message: reason})
	c.Assert(err, tc.ErrorIsNil)

	// per contract, there should be at minimum an initial empty Change() result
	wc.AssertChangeMaybeIncluding(expectActionIds(a1, a2)...)
	wc.AssertNoChange()
}

func (s *ActionSuite) TestUnitWatchActionNotifications(c *tc.C) {
	// get units
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit1)

	unit2, err := s.State.Unit(s.unit2.Name())
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit2)

	// queue some actions before starting the watcher
	operationID, err := s.Model.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	fa1, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa2, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	// set up watcher on first unit
	w := unit1.WatchPendingActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	// make sure the previously pending actions are sent on the watcher
	expect := expectActionIds(fa1, fa2)
	wc.AssertChange(expect...)
	wc.AssertNoChange()

	// add watcher on unit2
	w2 := unit2.WatchPendingActionNotifications()
	defer statetesting.AssertStop(c, w2)
	wc2 := statetesting.NewStringsWatcherC(c, w2)
	wc2.AssertChange()
	wc2.AssertNoChange()

	// add action on unit2 and makes sure unit1 watcher doesn't trigger
	// and unit2 watcher does
	fa3, err := s.Model.AddAction(unit2, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	expect2 := expectActionIds(fa3)
	wc2.AssertChange(expect2...)
	wc2.AssertNoChange()

	// add a couple actions on unit1 and make sure watcher sees events
	fa4, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa5, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	expect = expectActionIds(fa4, fa5)
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}

func (s *ActionSuite) TestMergeIds(c *tc.C) {
	var tests = []struct {
		changes  string
		adds     string
		removes  string
		expected string
	}{
		{changes: "", adds: "a0,a1", removes: "", expected: "a0,a1"},
		{changes: "a0,a1", adds: "", removes: "a0", expected: "a1"},
		{changes: "a0,a1", adds: "a2", removes: "a0", expected: "a1,a2"},

		{changes: "", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "", adds: "a0,a1,a2", removes: "a0,a1,a2", expected: ""},

		{changes: "a0", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "a1", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},
		{changes: "a2", adds: "a0,a1,a2", removes: "a0,a2", expected: "a1"},

		{changes: "a3,a4", adds: "a1,a4,a5", removes: "a1,a3", expected: "a4,a5"},
		{changes: "a0,a1,a2", adds: "a1,a4,a5", removes: "a1,a3", expected: "a0,a2,a4,a5"},
	}

	prefix := state.DocID(s.State, "")

	for ix, test := range tests {
		updates := mapify(prefix, test.adds, test.removes)
		changes := sliceify("", test.changes)
		expected := sliceify("", test.expected)

		c.Log(fmt.Sprintf("test number %d %#v", ix, test))
		err := state.WatcherMergeIds(&changes, updates, state.MakeActionIdConverter(s.State))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(changes, tc.SameContents, expected)
	}
}

func (s *ActionSuite) TestMergeIdsErrors(c *tc.C) {

	var tests = []struct {
		name string
		key  interface{}
	}{
		{name: "bool", key: true},
		{name: "int", key: 0},
		{name: "chan string", key: make(chan string)},
	}

	for _, test := range tests {
		changes, updates := []string{}, map[interface{}]bool{}
		updates[test.key] = true
		err := state.WatcherMergeIds(&changes, updates, state.MakeActionIdConverter(s.State))
		c.Assert(err, tc.ErrorMatches, "id is not of type string, got "+test.name)
	}
}

func (s *ActionSuite) TestEnsureSuffix(c *tc.C) {
	marker := "-marker-"
	fn := state.WatcherEnsureSuffixFn(marker)
	c.Assert(fn, tc.Not(tc.IsNil))

	var tests = []struct {
		given  string
		expect string
	}{
		{given: marker, expect: marker},
		{given: "", expect: "" + marker},
		{given: "asdf", expect: "asdf" + marker},
		{given: "asdf" + marker, expect: "asdf" + marker},
		{given: "asdf" + marker + "qwerty", expect: "asdf" + marker + "qwerty" + marker},
	}

	for _, test := range tests {
		c.Assert(fn(test.given), tc.Equals, test.expect)
	}
}

func (s *ActionSuite) TestMakeIdFilter(c *tc.C) {
	marker := "-marker-"
	badmarker := "-bad-"
	fn := state.WatcherMakeIdFilter(s.State, marker)
	c.Assert(fn, tc.IsNil)

	ar1 := mockAR{id: "mock/1"}
	ar2 := mockAR{id: "mock/2"}
	fn = state.WatcherMakeIdFilter(s.State, marker, ar1, ar2)
	c.Assert(fn, tc.Not(tc.IsNil))

	var tests = []struct {
		id    string
		match bool
	}{
		{id: "mock/1" + marker + "", match: true},
		{id: "mock/1" + marker + "asdf", match: true},
		{id: "mock/2" + marker + "", match: true},
		{id: "mock/2" + marker + "asdf", match: true},

		{id: "mock/1" + badmarker + "", match: false},
		{id: "mock/1" + badmarker + "asdf", match: false},
		{id: "mock/2" + badmarker + "", match: false},
		{id: "mock/2" + badmarker + "asdf", match: false},

		{id: "mock/1" + marker + "0", match: true},
		{id: "mock/10" + marker + "0", match: false},
		{id: "mock/2" + marker + "0", match: true},
		{id: "mock/20" + marker + "0", match: false},
		{id: "mock" + marker + "0", match: false},

		{id: "" + marker + "0", match: false},
		{id: "mock/1-0", match: false},
		{id: "mock/1-0", match: false},
	}

	for _, test := range tests {
		c.Assert(fn(state.DocID(s.State, test.id)), tc.Equals, test.match)
	}
}

func (s *ActionSuite) TestWatchActionNotifications(c *tc.C) {
	app := s.AddTestingApplication(c, "dummy2", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	w := u.WatchPendingActionNotifications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// add 3 actions
	operationID, err := s.Model.EnqueueOperation("a test", 3)
	c.Assert(err, tc.ErrorIsNil)
	fa1, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa2, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa3, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	// TODO(quiescence): this is a bit racey due to the unpredictable nature of mongo change streams
	// once we have some quiescence built into the watcher, we can re-enable this.
	// fail the middle one
	_ = model
	// action, err := model.Action(fa2.Id())
	// c.Assert(err, jc.ErrorIsNil)
	// _, err = action.Finish(state.ActionResults{Status: state.ActionFailed, Message: "die scum"})
	// c.Assert(err, jc.ErrorIsNil)

	// we expect them all even though the second one has already failed.
	// TODO(quiescence): reimplement some quiescence on the PendingActionNotications watcher
	expect := expectActionIds(fa1, fa2, fa3)
	wc.AssertChange(expect...)
	wc.AssertNoChange()
}

func expectActionIds(actions ...state.Action) []string {
	ids := make([]string, len(actions))
	for i, action := range actions {
		ids[i] = action.Id()
	}
	return ids
}

func (s *ActionSuite) TestWatchActionLogs(c *tc.C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	// queue some actions before starting the watcher
	fa1, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa1, err = fa1.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = fa1.Log("first")
	c.Assert(err, tc.ErrorIsNil)

	// Ensure no cross contamination - add another action.
	fa2, err := s.Model.AddAction(unit1, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	fa2, err = fa2.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = fa2.Log("another")
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	startNow := time.Now().UTC()
	makeTimestamp := func(offset time.Duration) time.Time {
		return time.Unix(0, startNow.UnixNano()).Add(offset).UTC()
	}

	checkExpected := func(wc statetesting.StringsWatcherC, expected []actions.ActionMessage) {
		var ch []string
		for len(ch) < len(expected) {
			select {
			case changes := <-wc.Watcher.Changes():
				ch = append(ch, changes...)
			case <-time.After(coretesting.LongWait):
				c.Fatalf("watcher did not send change")
			}
		}
		var msg []actions.ActionMessage
		for i, chStr := range ch {
			var gotMessage actions.ActionMessage
			err := json.Unmarshal([]byte(chStr), &gotMessage)
			c.Assert(err, tc.ErrorIsNil)
			// We can't control the actual time so check for
			// not nil and then assigned to a known value.
			c.Assert(gotMessage.Timestamp, tc.NotNil)
			gotMessage.Timestamp = makeTimestamp(time.Duration(i) * time.Second)
			msg = append(msg, gotMessage)
		}
		c.Assert(msg, tc.DeepEquals, expected)
		wc.AssertNoChange()
	}

	w := s.State.WatchActionLogs(fa1.Id())
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	// make sure the previously pending actions are sent on the watcher
	expected := []actions.ActionMessage{{
		Timestamp: startNow,
		Message:   "first",
	}}
	checkExpected(wc, expected)

	// Add 3 more messages; we should only see those on this watcher.
	err = fa1.Log("another")
	c.Assert(err, tc.ErrorIsNil)

	err = fa1.Log("yet another")
	c.Assert(err, tc.ErrorIsNil)

	expected = []actions.ActionMessage{{
		Timestamp: startNow,
		Message:   "another",
	}, {
		Timestamp: makeTimestamp(1 * time.Second),
		Message:   "yet another",
	}}
	checkExpected(wc, expected)

	// Add the 3rd message separately to ensure the
	// tracking of already reported messages works.
	err = fa1.Log("and yet another")
	c.Assert(err, tc.ErrorIsNil)
	expected = []actions.ActionMessage{{
		Timestamp: makeTimestamp(0 * time.Second),
		Message:   "and yet another",
	}}
	checkExpected(wc, expected)

	// But on a new watcher we see all 3 events.
	w2 := s.State.WatchActionLogs(fa1.Id())
	defer statetesting.AssertStop(c, w)
	wc2 := statetesting.NewStringsWatcherC(c, w2)
	// Make sure the previously pending actions are sent on the watcher.
	expected = []actions.ActionMessage{{
		Timestamp: startNow,
		Message:   "first",
	}, {
		Timestamp: makeTimestamp(1 * time.Second),
		Message:   "another",
	}, {
		Timestamp: makeTimestamp(2 * time.Second),
		Message:   "yet another",
	}, {
		Timestamp: makeTimestamp(3 * time.Second),
		Message:   "and yet another",
	}}
	checkExpected(wc2, expected)
}

// mapify is a convenience method, also to make reading the tests
// easier. It combines two comma delimited strings representing
// additions and removals and turns it into the map[interface{}]bool
// format needed
func mapify(prefix, adds, removes string) map[interface{}]bool {
	m := map[interface{}]bool{}
	for _, v := range sliceify(prefix, adds) {
		m[v] = true
	}
	for _, v := range sliceify(prefix, removes) {
		m[v] = false
	}
	return m
}

// sliceify turns a comma separated list of strings into a slice
// trimming white space and excluding empty strings.
func sliceify(prefix, csvlist string) []string {
	slice := []string{}
	if csvlist == "" {
		return slice
	}
	for _, entry := range strings.Split(csvlist, ",") {
		clean := strings.TrimSpace(entry)
		if clean != "" {
			slice = append(slice, prefix+clean)
		}
	}
	return slice
}

// mockAR is an implementation of ActionReceiver that can be used for
// testing that requires the ActionReceiver.Tag() call to return a
// names.Tag
type mockAR struct {
	id string
}

var _ state.ActionReceiver = (*mockAR)(nil)

func (r mockAR) PrepareActionPayload(_ string, _ map[string]interface{}, _ *bool, _ *string) (map[string]interface{}, bool, string, error) {
	return nil, false, "", nil
}
func (r mockAR) CancelAction(_ state.Action) (state.Action, error)     { return nil, nil }
func (r mockAR) WatchActionNotifications() state.StringsWatcher        { return nil }
func (r mockAR) WatchPendingActionNotifications() state.StringsWatcher { return nil }
func (r mockAR) Actions() ([]state.Action, error)                      { return nil, nil }
func (r mockAR) CompletedActions() ([]state.Action, error)             { return nil, nil }
func (r mockAR) PendingActions() ([]state.Action, error)               { return nil, nil }
func (r mockAR) RunningActions() ([]state.Action, error)               { return nil, nil }
func (r mockAR) Tag() names.Tag                                        { return names.NewUnitTag(r.id) }

type ActionPruningSuite struct {
	statetesting.StateWithWallClockSuite
}

func TestActionPruningSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ActionPruningSuite{})
}

func (s *ActionPruningSuite) TestPruneOperationsBySize(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	// PrimeOperations generates the operations and tasks to be pruned.
	const numOperationEntries = 15 // At slightly > 500kB per entry
	const tasksPerOperation = 2
	const maxLogSize = 5 //MB
	state.PrimeOperations(c, clock.Now(), unit, numOperationEntries, tasksPerOperation)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, tasksPerOperation*numOperationEntries)
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, numOperationEntries)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 0, maxLogSize)
	c.Assert(err, tc.ErrorIsNil)

	actions, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	ops, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	// The test here is to see if the remaining count is relatively close to
	// the max log size x 2. I would expect the number of remaining entries to
	// be no greater than 2 x 1.5 x the max log size in MB since each entry is
	// about 500kB (in memory) in size. 1.5x is probably good enough to ensure
	// this test doesn't flake.
	c.Assert(float64(len(actions)), tc.LessThan, 2.0*maxLogSize*1.5)
	c.Assert(float64(len(ops)), tc.Equals, float64(len(actions))/2.0)
}

func (s *ActionPruningSuite) TestPruneOperationsBySizeOldestFirst(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	const numOperationEntriesOlder = 5
	const numOperationEntriesYounger = 5
	const tasksPerOperation = 3
	const numOperationEntries = numOperationEntriesOlder + numOperationEntriesYounger
	const maxLogSize = 5 //MB

	olderTime := clock.Now().Add(-1 * time.Hour)
	youngerTime := clock.Now()

	state.PrimeOperations(c, olderTime, unit, numOperationEntriesOlder, tasksPerOperation)
	state.PrimeOperations(c, youngerTime, unit, numOperationEntriesYounger, tasksPerOperation)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, tasksPerOperation*numOperationEntries)
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, numOperationEntries)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 0, maxLogSize)
	c.Assert(err, tc.ErrorIsNil)

	actions, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)

	var olderEntries []time.Time
	var youngerEntries []time.Time
	for _, entry := range actions {
		if entry.Completed().Before(youngerTime.Round(time.Second)) {
			olderEntries = append(olderEntries, entry.Completed())
		} else {
			youngerEntries = append(youngerEntries, entry.Completed())
		}
	}
	c.Assert(len(youngerEntries), tc.GreaterThan, len(olderEntries))

	_, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	olderEntries = nil
	youngerEntries = nil
	for _, entry := range actions {
		if entry.Completed().Before(youngerTime.Round(time.Second)) {
			olderEntries = append(olderEntries, entry.Completed())
		} else {
			youngerEntries = append(youngerEntries, entry.Completed())
		}
	}
	c.Assert(len(youngerEntries), tc.GreaterThan, len(olderEntries))
}

func (s *ActionPruningSuite) TestPruneOperationsBySizeKeepsIncomplete(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	const numOperationEntriesOlder = 5
	const numOperationEntriesYounger = 5
	const numOperationEntriesIncomplete = 5
	const tasksPerOperation = 3
	const numOperationEntries = numOperationEntriesOlder + numOperationEntriesYounger + numOperationEntriesIncomplete
	const maxLogSize = 5 //MB

	olderTime := clock.Now().Add(-1 * time.Hour)
	youngerTime := clock.Now()

	state.PrimeOperations(c, time.Time{}, unit, numOperationEntriesIncomplete, tasksPerOperation)
	state.PrimeOperations(c, olderTime, unit, numOperationEntriesOlder, tasksPerOperation)
	state.PrimeOperations(c, youngerTime, unit, numOperationEntriesYounger, tasksPerOperation)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, tasksPerOperation*numOperationEntries)
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, numOperationEntries)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 0, maxLogSize)
	c.Assert(err, tc.ErrorIsNil)

	actions, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)

	var olderEntries []time.Time
	var youngerEntries []time.Time
	var incompleteEntries []time.Time
	zero := time.Time{}
	for _, entry := range actions {
		if entry.Completed() == zero {
			incompleteEntries = append(incompleteEntries, entry.Completed())
		} else if entry.Completed().Before(youngerTime.Round(time.Second)) {
			olderEntries = append(olderEntries, entry.Completed())
		} else {
			youngerEntries = append(youngerEntries, entry.Completed())
		}
	}
	c.Assert(youngerEntries, tc.HasLen, 0)
	c.Assert(olderEntries, tc.HasLen, 0)
	c.Assert(len(incompleteEntries), tc.Not(tc.Equals), 0)

	ops, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	// The test here is to see if the remaining count is relatively close to
	// the max log size x 2. I would expect the number of remaining entries to
	// be no greater than 2 x 1.5 x the max log size in MB since each entry is
	// about 500kB (in memory) in size. 1.5x is probably good enough to ensure
	// this test doesn't flake.
	c.Assert(float64(len(actions)), tc.LessThan, 2.0*maxLogSize*1.5)
	c.Assert(float64(len(ops)), tc.Equals, float64(len(actions))/3.0)
}

func (s *ActionPruningSuite) TestPruneOperationsByAge(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	const numCurrentOperationEntries = 5
	const numExpiredOperationEntries = 5
	const tasksPerOperation = 3
	const ageOfExpired = 10 * time.Hour

	state.PrimeOperations(c, clock.Now(), unit, numCurrentOperationEntries, tasksPerOperation)
	state.PrimeOperations(c, clock.Now().Add(-1*ageOfExpired), unit, numExpiredOperationEntries, tasksPerOperation)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, tasksPerOperation*(numCurrentOperationEntries+numExpiredOperationEntries))
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, numCurrentOperationEntries+numExpiredOperationEntries)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 1*time.Hour, 0)
	c.Assert(err, tc.ErrorIsNil)

	actions, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	ops, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	c.Log(actions)
	c.Assert(actions, tc.HasLen, tasksPerOperation*numCurrentOperationEntries)
	c.Assert(ops, tc.HasLen, numCurrentOperationEntries)
}

// Pruner should not prune operations with age of epoch time since the epoch is a
// special value denoting an incomplete operation.
func (s *ActionPruningSuite) TestDoNotPruneIncompleteOperations(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	// Completed times with the zero value are designated not complete
	const numZeroValueEntries = 5
	const tasksPerOperation = 3
	state.PrimeOperations(c, time.Time{}, unit, numZeroValueEntries, tasksPerOperation)

	_, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 1*time.Hour, 0)
	c.Assert(err, tc.ErrorIsNil)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(actions), tc.Equals, tasksPerOperation*numZeroValueEntries)
	c.Assert(len(ops), tc.Equals, numZeroValueEntries)
}

func (s *ActionPruningSuite) TestPruneLegacyActions(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})

	const numCurrentOperationEntries = 5
	const numExpiredOperationEntries = 5
	const tasksPerOperation = 3
	const ageOfExpired = 10 * time.Hour

	state.PrimeOperations(c, clock.Now(), unit, numCurrentOperationEntries, tasksPerOperation)
	state.PrimeOperations(c, clock.Now().Add(-1*ageOfExpired), unit, numExpiredOperationEntries, tasksPerOperation)
	state.PrimeLegacyActions(c, clock.Now().Add(-1*ageOfExpired), unit, numExpiredOperationEntries)

	actions, err := unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actions, tc.HasLen, tasksPerOperation*(numCurrentOperationEntries+numExpiredOperationEntries)+numExpiredOperationEntries)
	ops, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, numCurrentOperationEntries+numExpiredOperationEntries)

	var stop <-chan struct{}
	err = state.PruneOperations(stop, s.State, 1*time.Hour, 0)
	c.Assert(err, tc.ErrorIsNil)

	actions, err = unit.Actions()
	c.Assert(err, tc.ErrorIsNil)
	ops, err = s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)

	c.Log(actions)
	c.Assert(actions, tc.HasLen, tasksPerOperation*numCurrentOperationEntries)
	c.Assert(ops, tc.HasLen, numCurrentOperationEntries)
}
