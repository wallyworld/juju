// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type statePoolSuite struct {
	statetesting.StateSuite
	State1, State2                    *state.State
	ModelUUID, ModelUUID1, ModelUUID2 string
}

func TestStatePoolSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &statePoolSuite{})
}

func (s *statePoolSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.ModelUUID = s.State.ModelUUID()

	s.State1 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { s.State1.Close() })
	s.ModelUUID1 = s.State1.ModelUUID()

	s.State2 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { s.State2.Close() })
	s.ModelUUID2 = s.State2.ModelUUID()
}

func (s *statePoolSuite) TestGet(c *tc.C) {
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st1.ModelUUID(), tc.Equals, s.ModelUUID1)

	st2, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st2.ModelUUID(), tc.Equals, s.ModelUUID2)

	// Check that the same instances are returned
	// when a State for the same model is re-requested.
	st1_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st1_.State, tc.Equals, st1.State)

	st2_, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st2_.State, tc.Equals, st2.State)
}

func (s *statePoolSuite) TestGetWithControllerModel(c *tc.C) {
	// When a State for the controller model is requested, the same
	// State that was original passed in should be returned.
	st0, err := s.StatePool.Get(s.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st0.State, tc.Equals, s.State)
}

func (s *statePoolSuite) TestGetSystemState(c *tc.C) {
	st0, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st0, tc.Equals, s.State)
}

func (s *statePoolSuite) TestSystemStateErrorPoolIsClosed(c *tc.C) {
	err := s.StatePool.Close()
	c.Assert(err, tc.ErrorIsNil)
	_, errSysState := s.StatePool.SystemState()
	c.Assert(errSysState, tc.ErrorMatches, "pool is closed")
}

func (s *statePoolSuite) TestClose(c *tc.C) {
	// Get some State instances.
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)

	st2, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, tc.ErrorIsNil)

	// Now close them.
	err = s.StatePool.Close()
	c.Assert(err, tc.ErrorIsNil)

	assertStateClosed := func(st *state.State) {
		err := st.Ping()
		c.Assert(err, tc.ErrorMatches, "Closed explicitly")
	}

	assertStateClosed(s.State)
	assertStateClosed(st1.State)
	assertStateClosed(st2.State)

	// Requests to Get and GetModel now return errors.
	st1_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorMatches, "pool is closed")
	c.Assert(st1_, tc.IsNil)

	st2_, err := s.StatePool.Get(s.ModelUUID2)
	c.Assert(err, tc.ErrorMatches, "pool is closed")
	c.Assert(st2_, tc.IsNil)
}

func (s *statePoolSuite) TestTooManyReleases(c *tc.C) {
	st1, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	// Get a second reference to the same model
	st2, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)

	// Try to call the first releaser twice.
	st1.Release()
	st1.Release()

	removed, err := s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.IsFalse)

	// Not closed because r2 has not been called.
	assertNotClosed(c, st1.State)

	removed = st2.Release()
	c.Assert(removed, tc.IsTrue)
	assertClosed(c, st1.State)
}

func (s *statePoolSuite) TestReleaseOnSystemStateUUID(c *tc.C) {
	st, err := s.StatePool.Get(s.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	removed := st.Release()
	c.Assert(removed, tc.IsFalse)
	assertNotClosed(c, st.State)
}

func (s *statePoolSuite) TestRemoveSystemStateUUID(c *tc.C) {
	removed, err := s.StatePool.Remove(s.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.IsFalse)
	assertNotClosed(c, s.State)
}

func assertNotClosed(c *tc.C, st *state.State) {
	_, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
}

func assertClosed(c *tc.C, st *state.State) {
	w := state.GetInternalWorkers(st)
	c.Check(workertest.CheckKilled(c, w), tc.ErrorIsNil)
}

func (s *statePoolSuite) TestRemoveWithNoRefsCloses(c *tc.C) {
	st, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)

	// Confirm the state is closed because there are no references. Calling
	// pool.Get will recreate the state again.
	removed := st.Release()
	c.Assert(removed, tc.IsTrue)
	assertClosed(c, st.State)

	removed, err = s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.IsTrue)

	assertClosed(c, st.State)
}

func (s *statePoolSuite) TestRemoveWithRefsClosesOnLastRelease(c *tc.C) {
	st, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	st2, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	// Now there are two references to the state.
	// Sanity check!
	assertNotClosed(c, st.State)

	// Doesn't close while there are refs still held.
	removed, err := s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.IsFalse)
	assertNotClosed(c, st.State)

	removed = st.Release()
	// Hasn't been closed - still one outstanding reference.
	c.Assert(removed, tc.IsFalse)
	assertNotClosed(c, st.State)

	// Should be closed when it's released back into the pool.
	removed = st2.Release()
	c.Assert(removed, tc.IsTrue)
	assertClosed(c, st.State)
}

func (s *statePoolSuite) TestGetRemovedNotAllowed(c *tc.C) {
	_, err := s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.StatePool.Remove(s.ModelUUID1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.StatePool.Get(s.ModelUUID1)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("model %v has been removed", s.ModelUUID1))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *statePoolSuite) TestReport(c *tc.C) {
	report := s.StatePool.Report()
	c.Check(report, tc.HasLen, 3)
}
