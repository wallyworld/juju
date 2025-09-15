// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type StateTrackerSuite struct {
	statetesting.StateSuite
	stateTracker workerstate.StateTracker
}

func TestStateTrackerSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &StateTrackerSuite{})
}

func (s *StateTrackerSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.stateTracker = workerstate.NewStateTracker(s.StatePool)
}

func (s *StateTrackerSuite) TestDoneWithNoUse(c *tc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, tc.ErrorIsNil)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestTooManyDones(c *tc.C) {
	err := s.stateTracker.Done()
	c.Assert(err, tc.ErrorIsNil)
	assertStatePoolClosed(c, s.StatePool)

	err = s.stateTracker.Done()
	c.Assert(err, tc.Equals, workerstate.ErrStateClosed)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestUse(c *tc.C) {
	pool, err := s.stateTracker.Use()
	c.Assert(err, tc.ErrorIsNil)
	systemState, err := pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(systemState, tc.Equals, s.State)
	c.Check(err, tc.ErrorIsNil)

	systemState, err = pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	pool, err = s.stateTracker.Use()
	c.Check(systemState, tc.Equals, s.State)
	c.Check(err, tc.ErrorIsNil)
}

func (s *StateTrackerSuite) TestUseAndDone(c *tc.C) {
	// Ref count starts at 1 (the creator/owner)

	_, err := s.stateTracker.Use()
	// 2
	c.Check(err, tc.ErrorIsNil)

	_, err = s.stateTracker.Use()
	// 3
	c.Check(err, tc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), tc.ErrorIsNil)
	// 2
	assertStatePoolNotClosed(c, s.StatePool)

	_, err = s.stateTracker.Use()
	// 3
	c.Check(err, tc.ErrorIsNil)

	c.Check(s.stateTracker.Done(), tc.ErrorIsNil)
	// 2
	assertStatePoolNotClosed(c, s.StatePool)

	c.Check(s.stateTracker.Done(), tc.ErrorIsNil)
	// 1
	assertStatePoolNotClosed(c, s.StatePool)

	c.Check(s.stateTracker.Done(), tc.ErrorIsNil)
	// 0
	assertStatePoolClosed(c, s.StatePool)
}

func (s *StateTrackerSuite) TestUseWhenClosed(c *tc.C) {
	c.Assert(s.stateTracker.Done(), tc.ErrorIsNil)

	pool, err := s.stateTracker.Use()
	c.Check(pool, tc.IsNil)
	c.Check(err, tc.Equals, workerstate.ErrStateClosed)
}

func assertStatePoolNotClosed(c *tc.C, pool *state.StatePool) {
	systemState, err := pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(systemState, tc.NotNil)
	err = systemState.Ping()
	c.Assert(err, tc.ErrorIsNil)
}

func assertStatePoolClosed(c *tc.C, pool *state.StatePool) {
	systemState, err := pool.SystemState()
	c.Assert(err, tc.ErrorMatches, "pool is closed")
	c.Assert(systemState, tc.IsNil)
}

func isTxnLogStarted(report map[string]interface{}) bool {
	// Sometimes when we call pool.Report() not all the workers have started yet, so we check
	next := report
	var ok bool
	for _, p := range []string{"txn-watcher", "workers", "txnlog"} {
		if child, ok := next[p]; !ok {
			return false
		} else {
			next = child.(map[string]interface{})
		}
	}
	state, ok := next["state"]
	return ok && state == "started"
}

func (s *StateTrackerSuite) TestReport(c *tc.C) {
	pool, err := s.stateTracker.Use()
	c.Assert(err, tc.ErrorIsNil)
	start := time.Now()
	report := pool.Report()
	for !isTxnLogStarted(report) {
		if time.Since(start) >= testing.LongWait {
			c.Fatalf("txlog worker did not start after %v", testing.LongWait)
		}
		time.Sleep(time.Millisecond)
		report = pool.Report()
	}
	c.Check(report, tc.NotNil)
	// We don't have any State models in the pool, but we do have the
	// txn-watcher report and the system state.
	c.Check(report, tc.HasLen, 3)
	c.Check(report["pool-size"], tc.Equals, 0)
}
