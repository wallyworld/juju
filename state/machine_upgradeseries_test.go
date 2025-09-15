// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

func (s *MachineSuite) TestCreateUpgradeSeriesLock(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsFalse)

	unitIds := []string{"multi-series/0", "multi-series-subordinate/0"}
	err = mach.CreateUpgradeSeriesLock(unitIds, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorIsNil)

	locked, err = mach.IsLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsTrue)

	units, err := mach.UpgradeSeriesUnitStatuses()
	c.Assert(err, tc.ErrorIsNil)

	lockedUnitsIds := make([]string, len(units))
	i := 0
	for id := range units {
		lockedUnitsIds[i] = id
		i++
	}
	c.Assert(lockedUnitsIds, tc.SameContents, unitIds)
}

func (s *MachineSuite) TestIsParentLockedForSeriesUpgrade(c *tc.C) {
	parent, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("16.04"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	child, err := s.State.AddMachineInsideMachine(template, parent.Id(), "lxd")
	c.Assert(err, tc.ErrorIsNil)

	err = parent.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("18.04"))
	c.Assert(err, tc.ErrorIsNil)

	locked, err := child.IsParentLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsTrue)
}

func (s *MachineSuite) TestCreateUpgradeSeriesLockErrorsIfLockExists(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.CreateUpgradeSeriesLock([]string{"multi-series/0", "multi-series-subordinate/0"}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorIsNil)
	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorMatches, "upgrade series lock for machine \".*\" already exists")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockOnDyingMachine(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = mach.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = mach.CreateUpgradeSeriesLock([]string{""}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorMatches, "machine not found or not alive")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockOnSameSeries(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = mach.CreateUpgradeSeriesLock([]string{""}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorMatches, "machine .* already at base ubuntu@16.04/stable")
}

func (s *MachineSuite) TestDoesNotCreateUpgradeSeriesLockUnitsChanged(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	err := mach.CreateUpgradeSeriesLock([]string{"wordpress/0"}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorMatches, "Units have changed, please retry (.*)")
}

func (s *MachineSuite) TestUpgradeSeriesTarget(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	units := []string{"multi-series/0", "multi-series-subordinate/0"}
	err := mach.CreateUpgradeSeriesLock(units, state.UbuntuBase("18.04"))
	c.Assert(err, tc.ErrorIsNil)

	target, err := mach.UpgradeSeriesTarget()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(target, tc.Equals, "ubuntu@18.04/stable")
}

func (s *MachineSuite) TestRemoveUpgradeSeriesLockUnlocksMachine(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineLockedForPrepare(c, mach)

	err = mach.RemoveUpgradeSeriesLock()
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)
}

func (s *MachineSuite) TestRemoveUpgradeSeriesLockIsNoOpIfMachineIsNotLocked(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.RemoveUpgradeSeriesLock()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestForceMarksSeriesLockUnlocksMachineForCleanup(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineIsNOTLockedForPrepare(c, mach)

	err = mach.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorIsNil)
	AssertMachineLockedForPrepare(c, mach)

	err = mach.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorIsNil)

	// After a forced destroy an upgrade series lock on a machine should be
	// marked for cleanup and therefore should be cleaned up if anything
	// should trigger a state cleanup.
	s.State.Cleanup(fakeSecretDeleter)

	// The machine, since it was destroyed, its lock should have been
	// cleaned up. Checking to see if the machine is not locked, that is,
	// checking to see if no lock exist for the machine should yield a
	// positive result.
	AssertMachineIsNOTLockedForPrepare(c, mach)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldFailWhenMachineIsNotComplete(c *tc.C) {
	err := s.machine.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	assertMachineIsNotReadyForCompletion(c, err)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldSucceedWhenMachinePrepareIsComplete(c *tc.C) {
	unit0 := s.addMachineUnit(c, s.machine)
	err := s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldSetCompleteStatusOfMachine(c *tc.C) {
	err := s.machine.CreateUpgradeSeriesLock([]string{}, state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, tc.ErrorIsNil)

	sts, err := s.machine.UpgradeSeriesStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sts, tc.Equals, model.UpgradeSeriesCompleteStarted)
}

func (s *MachineSuite) TestCompleteSeriesUpgradeShouldFailIfAlreadyInCompleteState(c *tc.C) {
	unit0 := s.addMachineUnit(c, s.machine)
	err := s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.CompleteUpgradeSeries()
	assertMachineIsNotReadyForCompletion(c, err)
}

func (s *MachineSuite) TestHasUpgradeSeriesLocks(c *tc.C) {
	// Ensure we don't have any locks before testing.
	lock, err := s.State.HasUpgradeSeriesLocks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lock, tc.IsFalse)

	unit0 := s.addMachineUnit(c, s.machine)
	err = s.machine.CreateUpgradeSeriesLock([]string{unit0.Name()}, state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)

	lock, err = s.State.HasUpgradeSeriesLocks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lock, tc.IsTrue)
}

func assertMachineIsNotReadyForCompletion(c *tc.C, err error) {
	c.Assert(err, tc.ErrorMatches, "machine \"[0-9].*\" can not complete, it is either not prepared or already completed")
}

func (s *MachineSuite) TestUnitsHaveChangedFalse(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{"multi-series/0", "multi-series-subordinate/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changed, tc.IsFalse)
}

func (s *MachineSuite) TestUnitsHaveChangedTrue(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{"multi-series-subordinate/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changed, tc.IsTrue)
}

func (s *MachineSuite) TestUnitsHaveChangedFalseNoUnits(c *tc.C) {
	mach, err := s.State.AddMachine(state.UbuntuBase("16.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	changed, err := state.UnitsHaveChanged(mach, []string{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changed, tc.IsFalse)
}

func (s *MachineSuite) TestGetUpgradeSeriesMessagesMissingLockMeansFinished(c *tc.C) {
	_, finished, err := s.machine.GetUpgradeSeriesMessages()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(finished, tc.IsTrue)
}

func (s *MachineSuite) TestIsLockedIndicatesUnlockedWhenNoLockDocIsFound(c *tc.C) {
	locked, err := s.machine.IsLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsFalse)
}

func AssertMachineLockedForPrepare(c *tc.C, mach *state.Machine) {
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsTrue)
}

func AssertMachineIsNOTLockedForPrepare(c *tc.C, mach *state.Machine) {
	locked, err := mach.IsLockedForSeriesUpgrade()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(locked, tc.IsFalse)
}
