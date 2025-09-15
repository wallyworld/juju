// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type MachineRemovalSuite struct {
	ConnSuite
}

func TestMachineRemovalSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MachineRemovalSuite{})
}

func (s *MachineRemovalSuite) TestMarkingAndCompletingMachineRemoval(c *tc.C) {
	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)

	err := m1.MarkForRemoval()
	c.Assert(err, tc.ErrorIsNil)
	err = m2.MarkForRemoval()
	c.Assert(err, tc.ErrorIsNil)

	// Check marking a machine multiple times is ok.
	err = m1.MarkForRemoval()
	c.Assert(err, tc.ErrorIsNil)

	// Check machines haven't been removed.
	_, err = s.State.Machine(m1.Id())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, tc.ErrorIsNil)

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removals, tc.SameContents, []string{m1.Id(), m2.Id()})

	err = s.State.CompleteMachineRemovals(m1.Id(), "100")
	c.Assert(err, tc.ErrorIsNil)
	removals2, err := s.State.AllMachineRemovals()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removals2, tc.SameContents, []string{m2.Id()})

	_, err = s.State.Machine(m1.Id())
	c.Assert(err, tc.ErrorMatches, "machine 0 not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	// But m2 is still there.
	_, err = s.State.Machine(m2.Id())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestMarkForRemovalRequiresDeadness(c *tc.C) {
	m := s.makeMachine(c, false)
	err := m.MarkForRemoval()
	c.Assert(err, tc.ErrorMatches, "cannot remove machine 0: machine is not dead")
}

func (s *MachineRemovalSuite) TestMarkForRemovalAssertsMachineStillExists(c *tc.C) {
	m := s.makeMachine(c, true)
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(m.Remove(), tc.IsNil)
	}).Check()
	err := m.MarkForRemoval()
	c.Assert(err, tc.ErrorMatches, "cannot remove machine 0: machine 0 not found")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsRequiresMark(c *tc.C) {
	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)
	err := s.State.CompleteMachineRemovals(m1.Id(), m2.Id())
	c.Assert(err, tc.ErrorMatches, "cannot remove machines 0, 1: not marked for removal")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsRequiresMarkSingular(c *tc.C) {
	m1 := s.makeMachine(c, true)
	err := s.State.CompleteMachineRemovals(m1.Id())
	c.Assert(err, tc.ErrorMatches, "cannot remove machine 0: not marked for removal")
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsIgnoresNonexistent(c *tc.C) {
	err := s.State.CompleteMachineRemovals("0", "1")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineRemovalSuite) TestCompleteMachineRemovalsInvalid(c *tc.C) {
	err := s.State.CompleteMachineRemovals("A", "0/lxd/1", "B")
	c.Assert(err, tc.ErrorMatches, "Invalid machine ids: A, B")
}

func (s *MachineRemovalSuite) TestWatchMachineRemovals(c *tc.C) {
	w, wc := s.createRemovalWatcher(c, s.State)
	wc.AssertOneChange() // Initial event.

	m1 := s.makeMachine(c, true)
	m2 := s.makeMachine(c, true)

	err := m1.MarkForRemoval()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	err = m2.MarkForRemoval()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	s.State.CompleteMachineRemovals(m1.Id(), m2.Id())
	wc.AssertOneChange()

	testing.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MachineRemovalSuite) createRemovalWatcher(c *tc.C, st *state.State) (
	state.NotifyWatcher, testing.NotifyWatcherC,
) {
	w := st.WatchMachineRemovals()
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, w) })
	return w, testing.NewNotifyWatcherC(c, w)
}

func (s *MachineRemovalSuite) makeMachine(c *tc.C, deadAlready bool) *state.Machine {
	m, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	if deadAlready {
		deadenMachine(c, m)
	}
	return m
}

func deadenMachine(c *tc.C, m *state.Machine) {
	c.Assert(m.EnsureDead(), tc.ErrorIsNil)
}
