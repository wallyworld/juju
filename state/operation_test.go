// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type OperationSuite struct {
	ConnSuite
}

func TestOperationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &OperationSuite{})
}

func (s *OperationSuite) TestEnqueueOperation(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation", 1)
	c.Assert(err, tc.ErrorIsNil)

	operation, err := s.Model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Id(), tc.Equals, operationID)
	c.Assert(operation.Tag(), tc.Equals, names.NewOperationTag(operationID))
	c.Assert(operation.Status(), tc.Equals, state.ActionPending)
	c.Assert(operation.Enqueued(), tc.Equals, clock.Now())
	c.Assert(operation.Started(), tc.Equals, time.Time{})
	c.Assert(operation.Completed(), tc.Equals, time.Time{})
	c.Assert(operation.Summary(), tc.Equals, "an operation")
}

func (s *OperationSuite) TestFailOperationEnqueuing(c *tc.C) {
	operationID, err := s.Model.EnqueueOperation("an operation", 5)
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.FailOperationEnqueuing(operationID, "fail", 4)
	c.Assert(err, tc.ErrorIsNil)
	operation, err := s.Model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Not(tc.Equals), state.ActionError)
	c.Assert(operation.Fail(), tc.Equals, "fail")
	c.Assert(operation.SpawnedTaskCount(), tc.Equals, 4)
}

func (s *OperationSuite) TestAllOperations(c *tc.C) {
	operationID, err := s.Model.EnqueueOperation("an operation", 1)
	c.Assert(err, tc.ErrorIsNil)
	operationId2, err := s.Model.EnqueueOperation("another operation", 1)
	c.Assert(err, tc.ErrorIsNil)

	operations, err := s.Model.AllOperations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 2)

	var ids []string
	for _, op := range operations {
		ids = append(ids, op.Id())
	}
	c.Assert(ids, tc.SameContents, []string{operationID, operationId2})
}

func (s *OperationSuite) TestOperationStatus(c *tc.C) {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation", 1)
	c.Assert(err, tc.ErrorIsNil)
	clock.Advance(5 * time.Second)
	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)

	operation, err := s.Model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
	c.Assert(operation.Started(), tc.Equals, clock.Now())
	c.Assert(operation.Completed(), tc.Equals, time.Time{})
}

func (s *OperationSuite) TestRefresh(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation", 1)
	c.Assert(err, tc.ErrorIsNil)
	operation, err := s.Model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)

	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)

	err = operation.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Status(), tc.Equals, state.ActionRunning)
}

func (s *OperationSuite) setupOperations(c *tc.C) names.Tag {
	clock := testclock.NewClock(coretesting.NonZeroTime().Round(time.Second))
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("an operation", 1)
	c.Assert(err, tc.ErrorIsNil)
	operationID2, err := s.Model.EnqueueOperation("another operation", 1)
	c.Assert(err, tc.ErrorIsNil)

	clock.Advance(5 * time.Second)
	anAction, err := s.Model.EnqueueAction(operationID, unit.Tag(), "backup", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = anAction.Begin()
	c.Assert(err, tc.ErrorIsNil)
	anAction2, err := s.Model.EnqueueAction(operationID2, unit.Tag(), "restore", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	a, err := anAction2.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = a.Log("hello")
	c.Assert(err, tc.ErrorIsNil)
	_, err = anAction2.Finish(state.ActionResults{
		Status:  state.ActionCompleted,
		Message: "done",
		Results: map[string]interface{}{"foo": "bar"},
	})
	c.Assert(err, tc.ErrorIsNil)

	unit2, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	operationID3, err := s.Model.EnqueueOperation("yet another operation", 1)
	c.Assert(err, tc.ErrorIsNil)
	anAction3, err := s.Model.EnqueueAction(operationID3, unit2.Tag(), "backup", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = anAction3.Begin()
	c.Assert(err, tc.ErrorIsNil)

	return unit.Tag()
}

func (s *OperationSuite) assertActions(c *tc.C, operations []state.OperationInfo) {
	for _, operation := range operations {
		for _, a := range operation.Actions {
			c.Assert(operation.Operation.Id(), tc.Equals, state.ActionOperationId(a))
			if a.Name() == "restore" {
				c.Assert(a.Status(), tc.Equals, state.ActionCompleted)
			} else {
				c.Assert(a.Status(), tc.Equals, state.ActionRunning)
			}
			c.Assert(a.Messages(), tc.HasLen, 0)
			c.Assert(a.Messages(), tc.HasLen, 0)
			results, _ := a.Results()
			c.Assert(results, tc.HasLen, 0)
		}
	}
}

func (s *OperationSuite) TestListOperationsNoFilter(c *tc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, nil, nil, 0, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(truncated, tc.IsFalse)
	c.Assert(operations, tc.HasLen, 3)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "an operation")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	c.Assert(operations[1].Operation.Summary(), tc.Equals, "another operation")
	c.Assert(operations[1].Actions, tc.HasLen, 1)
	c.Assert(operations[2].Operation.Summary(), tc.Equals, "yet another operation")
	c.Assert(operations[2].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperations(c *tc.C) {
	unitTag := s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations([]string{"backup"}, []names.Tag{unitTag}, []state.ActionStatus{state.ActionRunning}, 0, 0)
	c.Assert(truncated, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "an operation")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByStatus(c *tc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, nil, []state.ActionStatus{state.ActionCompleted}, 0, 0)
	c.Assert(truncated, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "another operation")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByName(c *tc.C) {
	s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations([]string{"restore"}, nil, nil, 0, 0)
	c.Assert(truncated, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "another operation")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsByReceiver(c *tc.C) {
	unitTag := s.setupOperations(c)
	operations, truncated, err := s.Model.ListOperations(nil, []names.Tag{unitTag}, nil, 0, 0)
	c.Assert(truncated, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 2)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "an operation")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	c.Assert(operations[1].Operation.Summary(), tc.Equals, "another operation")
	c.Assert(operations[1].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestListOperationsSubset(c *tc.C) {
	s.setupOperations(c)
	for i := 0; i < 20; i++ {
		operationID, err := s.Model.EnqueueOperation(fmt.Sprintf("operation %d", i), 20)
		c.Assert(err, tc.ErrorIsNil)
		anAction, err := s.Model.EnqueueAction(operationID, names.NewUnitTag("dummy/0"), "backup", nil, false, "", nil)
		c.Assert(err, tc.ErrorIsNil)
		_, err = anAction.Begin()
		c.Assert(err, tc.ErrorIsNil)
	}
	operations, truncated, err := s.Model.ListOperations(nil, nil, nil, 14, 1)
	c.Assert(truncated, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operations, tc.HasLen, 1)
	c.Assert(operations[0].Operation.Summary(), tc.Equals, "operation 11")
	c.Assert(operations[0].Actions, tc.HasLen, 1)
	s.assertActions(c, operations)
}

func (s *OperationSuite) TestOperationWithActions(c *tc.C) {
	s.setupOperations(c)
	operation, err := s.Model.OperationWithActions("2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Operation.OperationTag().String(), tc.Equals, "operation-2")
	c.Assert(operation.Operation.Id(), tc.Equals, "2")
	c.Assert(operation.Operation.Summary(), tc.Equals, "another operation")
	c.Assert(operation.Operation.Status(), tc.Equals, state.ActionCompleted)
	c.Assert(operation.Operation.Enqueued(), tc.Not(tc.Equals), time.Time{})
	c.Assert(operation.Operation.Started(), tc.Not(tc.Equals), time.Time{})
	c.Assert(operation.Operation.Completed(), tc.Not(tc.Equals), time.Time{})
	c.Assert(operation.Actions, tc.HasLen, 1)
	c.Assert(operation.Actions[0].Id(), tc.Equals, "4")
	c.Assert(operation.Actions[0].Status(), tc.Equals, state.ActionCompleted)
	results, message := operation.Actions[0].Results()
	c.Assert(results, tc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Assert(message, tc.Equals, "done")
	c.Assert(operation.Actions[0].Messages(), tc.HasLen, 1)
	c.Assert(operation.Actions[0].Messages()[0].Message(), tc.Equals, "hello")
}

func (s *OperationSuite) TestOperationWithActionsNotFound(c *tc.C) {
	_, err := s.Model.OperationWithActions("1")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}
