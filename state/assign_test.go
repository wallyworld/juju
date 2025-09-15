// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strconv"
	tctesting "testing"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
)

type AssignSuite struct {
	ConnSuite
	wordpress *state.Application
}

func TestAssignSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &AssignSuite{})
}

func (s *AssignSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
	)
	s.wordpress = wordpress
}

func (s *AssignSuite) addSubordinate(c *tc.C, principal *state.Unit) *state.Unit {
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(principal)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)
	return subUnit
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	err = unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)

	// Check that the unit has no machine assigned.
	_, err = unit.AssignedMachineId()
	c.Assert(err, tc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToMachineAgainFails(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine.
	machineOne, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	machineTwo, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, tc.ErrorIsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, tc.ErrorIsNil)

	// Assigning the unit to a different machine should fail.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to machine 1: unit is already assigned to a machine`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineId, tc.Equals, "0")
}

func (s *AssignSuite) TestAssignedMachineIdWhenNotAlive(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	testWhenDying(c, unit, noErr, noErr,
		func() error {
			_, err = unit.AssignedMachineId()
			return err
		})
}

func (s *AssignSuite) TestAssignedMachineIdWhenPrincipalNotAlive(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	subUnit := s.addSubordinate(c, unit)
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	mid, err := subUnit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, machine.Id())
}

func (s *AssignSuite) TestUnassignUnitFromMachineWithChangingState(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Check that unassigning while the state changes fails nicely.
	// Remove the unit for the tests.
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, tc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)

	err = s.wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorMatches, `cannot unassign unit "wordpress/0" from machine: .*`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, tc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
}

func (s *AssignSuite) TestAssignSubordinatesToMachine(c *tc.C) {
	// Check that assigning a principal unit assigns its subordinates too.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Units need to be assigned to a machine before the subordinates
	// are created in order for the subordinate to get the machine ID.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	subUnit := s.addSubordinate(c, unit)

	// None of the direct unit assign methods work on subordinates.
	err = subUnit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "logging/0" to machine 0: unit is a subordinate`)
	_, err = subUnit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "logging/0" to clean machine: unit is a subordinate`)
	_, err = subUnit.AssignToCleanEmptyMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "logging/0" to clean, empty machine: unit is a subordinate`)
	err = subUnit.AssignToNewMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "logging/0" to new machine: unit is a subordinate`)

	// Subordinates know the machine they're indirectly assigned to.
	id, err := subUnit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, machine.Id())
}

func (s *AssignSuite) TestDirectAssignIgnoresConstraints(c *tc.C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	econs := constraints.MustParse("mem=4G cores=2")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// Machine will take model constraints on creation.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// Unit will take combined application/model constraints on creation.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Machine keeps its original constraints on direct assignment.
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, econs)
}

func (s *AssignSuite) TestAssignBadSeries(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("22.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to machine 0: base does not match.*`)
}

func (s *AssignSuite) TestAssignMachineWhenDying(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)
	assignTest := func() error {
		err := unit.AssignToMachine(machine)
		c.Assert(unit.UnassignFromMachine(), tc.IsNil)
		if subUnit != nil {
			err := subUnit.EnsureDead()
			c.Assert(err, tc.ErrorIsNil)
			err = subUnit.Remove()
			c.Assert(err, tc.ErrorIsNil)
			subUnit = nil
		}
		return err
	}
	expect := ".*: unit is not found or not alive"
	testWhenDying(c, unit, expect, expect, assignTest)

	expect = ".*: machine is not found or not alive"
	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	testWhenDying(c, machine, expect, expect, assignTest)
}

func (s *AssignSuite) TestAssignMachineDifferentSeries(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorMatches,
		`cannot assign unit "wordpress/0" to machine 0: base does not match.*`)
}

func (s *AssignSuite) TestPrincipals(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	principals := machine.Principals()
	c.Assert(principals, tc.DeepEquals, []string{})

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	principals = machine.Principals()
	c.Assert(principals, tc.DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) TestAssignMachinePrincipalsChange(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	checkPrincipals := func() []string {
		err := machine.Refresh()
		c.Assert(err, tc.ErrorIsNil)
		return machine.Principals()
	}
	c.Assert(checkPrincipals(), tc.DeepEquals, []string{"wordpress/0", "wordpress/1"})

	err = subUnit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = subUnit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(checkPrincipals(), tc.DeepEquals, []string{"wordpress/0"})
}

func (s *AssignSuite) assertAssignedUnit(c *tc.C, unit *state.Unit) string {
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	// Check that the principal is set on the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)
	machineUnits, err := machine.Units()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineUnits, tc.HasLen, 1)
	// Make sure it is the right unit.
	c.Assert(machineUnits[0].Name(), tc.Equals, unit.Name())
	return machineId
}

func (s *AssignSuite) TestAssignUnitToNewMachine(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignedUnit(c, unit)
}

func (s *AssignSuite) assertAssignUnitToNewMachineContainerConstraint(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	machineId := s.assertAssignedUnit(c, unit)
	c.Assert(container.ParentId(machineId), tc.Not(tc.Equals), "")
	c.Assert(container.ContainerTypeFromId(machineId), tc.Equals, instance.LXD)
}

func (s *AssignSuite) TestAssignUnitToNewMachineContainerConstraint(c *tc.C) {
	// Set up application constraints.
	scons := constraints.MustParse("container=lxd")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitToNewMachineDefaultContainerConstraint(c *tc.C) {
	// Set up model constraints.
	econs := constraints.MustParse("container=lxd")
	err := s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignUnitToNewMachineContainerConstraint(c)
}

func (s *AssignSuite) TestAssignToNewMachineMakesDirty(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Clean(), tc.IsFalse)
}

func (s *AssignSuite) TestAssignUnitToNewMachineSetsConstraints(c *tc.C) {
	// Set up constraints.
	scons := constraints.MustParse("mem=2G cpu-power=400")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	econs := constraints.MustParse("mem=4G cores=2")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// Unit will take combined application/model constraints on creation.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Change application/model constraints before assigning, to verify this.
	scons = constraints.MustParse("mem=6G cpu-power=800")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	econs = constraints.MustParse("cores=4")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// The new machine takes the original combined unit constraints.
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, tc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	expect := constraints.MustParse("arch=amd64 mem=2G cores=2 cpu-power=400")
	c.Assert(mcons, tc.DeepEquals, expect)
}

func (s *AssignSuite) TestAssignUnitToNewMachineCleanAvailable(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Add a clean machine.
	clean, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Id(), tc.Not(tc.Equals), clean.Id())
}

func (s *AssignSuite) TestAssignUnitToNewMachineAlreadyAssigned(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Make the unit assigned
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	// Try to assign it again
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is already assigned to a machine`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitNotAlive(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	subUnit := s.addSubordinate(c, unit)

	// Try to assign a dying unit...
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not found or not alive`)

	// ...and a dead one.
	err = subUnit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = subUnit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit is not found or not alive`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineUnitRemoved(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/0" to new machine: unit not found`)
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesDirty(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// Create some units and a clean machine.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	anotherUnit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	makeDirty := jujutxn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(machine), tc.IsNil) },
	}
	defer state.SetTestHooks(c, s.State, makeDirty).Check()

	err = anotherUnit.AssignToNewMachineOrContainer()
	c.Assert(err, tc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, "1")

	mid, err = anotherUnit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, "2/lxd/0")
}

func (s *AssignSuite) TestAssignUnitToNewMachineBecomesHost(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// Create a unit and a clean machine.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	addContainer := jujutxn.TestHook{
		Before: func() {
			_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
				Base: state.UbuntuBase("12.10"),
				Jobs: []state.MachineJob{state.JobHostUnits},
			}, machine.Id(), instance.LXD)
			c.Assert(err, tc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, addContainer).Check()

	err = unit.AssignToNewMachineOrContainer()
	c.Assert(err, tc.ErrorIsNil)

	mid, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, "2/lxd/0")
}

func (s *AssignSuite) TestAssignUnitBadPolicy(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Check nonsensical policy
	err = s.State.AssignUnit(unit, state.AssignmentPolicy("random"))
	c.Assert(err, tc.ErrorMatches, `.*unknown unit assignment policy: "random"`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, tc.NotNil)
	assertMachineCount(c, s.State, 0)
}

func (s *AssignSuite) TestAssignUnitLocalPolicy(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		err = s.State.AssignUnit(unit, state.AssignLocal)
		c.Assert(err, tc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mid, tc.Equals, m.Id())
		assertMachineCount(c, s.State, 1)
	}
}

func (s *AssignSuite) assertAssignUnitNewPolicyNoContainer(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits) // available machine
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, tc.ErrorIsNil)
	assertMachineCount(c, s.State, 2)
	id, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(container.ParentId(id), tc.Equals, "")
}

func (s *AssignSuite) TestAssignUnitNewPolicy(c *tc.C) {
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraintIgnoresNone(c *tc.C) {
	scons := constraints.MustParse("container=none")
	err := s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignUnitNewPolicyNoContainer(c)
}

func (s *AssignSuite) assertAssignUnitNewPolicyWithContainerConstraint(c *tc.C) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignNew)
	c.Assert(err, tc.ErrorIsNil)
	assertMachineCount(c, s.State, 3)
	id, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, "1/lxd/0")
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithContainerConstraint(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	// Set up application constraints.
	scons := constraints.MustParse("container=lxd")
	err = s.wordpress.SetConstraints(scons)
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitNewPolicyWithDefaultContainerConstraint(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	// Set up model constraints.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)
	s.assertAssignUnitNewPolicyWithContainerConstraint(c)
}

func (s *AssignSuite) TestAssignUnitWithSubordinate(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Check cannot assign subordinates to machines
	subUnit := s.addSubordinate(c, unit)
	for _, policy := range []state.AssignmentPolicy{
		state.AssignLocal, state.AssignNew, state.AssignClean, state.AssignCleanEmpty,
	} {
		err = s.State.AssignUnit(subUnit, policy)
		c.Assert(err, tc.ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
	}
}

func assertMachineCount(c *tc.C, st *state.State, expect int) {
	ms, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ms, tc.HasLen, expect, tc.Commentf("%v", ms))
}

// assignCleanSuite has tests for assigning units to 1. clean, and 2. clean&empty machines.
type assignCleanSuite struct {
	ConnSuite
	policy    state.AssignmentPolicy
	wordpress *state.Application
}

func TestAssignCleanEmptySuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &assignCleanSuite{ConnSuite{}, state.AssignCleanEmpty, nil})
}

func TestAssignCleanSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &assignCleanSuite{ConnSuite{}, state.AssignClean, nil})
}

func (s *assignCleanSuite) SetUpTest(c *tc.C) {
	c.Logf("assignment policy for this test: %q", s.policy)
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.wordpress = wordpress
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *assignCleanSuite) errorMessage(msg string) string {
	context := "clean"
	if s.policy == state.AssignCleanEmpty {
		context += ", empty"
	}
	return fmt.Sprintf(msg, context)
}

func (s *assignCleanSuite) assignUnit(unit *state.Unit) (*state.Machine, error) {
	if s.policy == state.AssignCleanEmpty {
		return unit.AssignToCleanEmptyMachine()
	}
	return unit.AssignToCleanMachine()
}

func (s *assignCleanSuite) assertMachineEmpty(c *tc.C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(containers), tc.Equals, 0)
}

func (s *assignCleanSuite) assertMachineNotEmpty(c *tc.C, machine *state.Machine) {
	containers, err := machine.Containers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(containers), tc.Not(tc.Equals), 0)
}

// setupMachines creates a combination of machines with which to test.
func (s *assignCleanSuite) setupMachines(c *tc.C) (hostMachine *state.Machine, container *state.Machine, cleanEmptyMachine *state.Machine) {
	amdArch := "amd64"
	hwChar := &instance.HardwareCharacteristics{
		Arch: &amdArch,
	}

	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)

	// Add some units to another application and allocate them to machines
	app1 := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		u, err := app1.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, tc.ErrorIsNil)
		err = u.AssignToMachine(m)
		c.Assert(err, tc.ErrorIsNil)
		units[i] = u
	}

	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	instId := instance.Id("i-host-machine")
	err = hostMachine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	container, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostMachine.Clean(), tc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	instId = instance.Id("i-container")
	err = container.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	// Create a new, clean, empty machine.
	cleanEmptyMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cleanEmptyMachine.Clean(), tc.IsTrue)
	s.assertMachineEmpty(c, cleanEmptyMachine)

	instId = instance.Id("i-clean-empty-machine")
	err = cleanEmptyMachine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	return hostMachine, container, cleanEmptyMachine
}

func (s *assignCleanSuite) assertAssignUnit(c *tc.C, expectedMachine *state.Machine) {
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	reusedMachine, err := s.assignUnit(unit)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reusedMachine.Id(), tc.Equals, expectedMachine.Id())
	c.Assert(reusedMachine.Clean(), tc.IsFalse)
}

func (s *assignCleanSuite) TestAssignUnit(c *tc.C) {
	hostMachine, container, cleanEmptyMachine := s.setupMachines(c)
	// Check that AssignToClean(Empty)Machine finds a newly created, clean (maybe empty) machine.
	if s.policy == state.AssignCleanEmpty {
		// The first clean, empty machine is the container.
		s.assertAssignUnit(c, container)
		// The next deployment will use the remaining clean, empty machine.
		s.assertAssignUnit(c, cleanEmptyMachine)
	} else {
		s.assertAssignUnit(c, hostMachine)
	}
}

func (s *assignCleanSuite) TestAssignUnitTwiceFails(c *tc.C) {
	s.setupMachines(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Assign the first time.
	_, err = s.assignUnit(unit)
	c.Assert(err, tc.ErrorIsNil)

	// Check that it fails when called again, even when there's an available machine
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, tc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine: unit is already assigned to a machine`))
	c.Assert(m.EnsureDead(), tc.IsNil)
	c.Assert(m.Remove(), tc.IsNil)
}

const eligibleMachinesInUse = ".*: all eligible machines in use"

func (s *assignCleanSuite) TestAssignToMachineNoneAvailable(c *tc.C) {
	// Try to assign a unit to a clean (maybe empty) machine and check that we can't.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.assignUnit(unit)
	c.Assert(m, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)

	// Add a state management machine which can host units and check it is not chosen.
	// Note that this must the first machine added, as AddMachine can only
	// be used to add state-manager machines for the bootstrap machine.
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)

	// Add a dying machine and check that it is not chosen.
	m, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = m.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)

	node, err := s.State.ControllerNode("0")
	c.Assert(err, tc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, tc.ErrorIsNil)

	// Add two controller machines and check they are not chosen.
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	c.Assert(changes.Maintained, tc.HasLen, 1)

	m, err = s.assignUnit(unit)
	c.Assert(m, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)

	// Add a machine with the wrong series and check it is not chosen.
	m, err = s.State.AddMachine(state.UbuntuBase("22.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	m, err = s.assignUnit(unit)
	c.Assert(m, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)
}

var assignUsingConstraintsTests = []struct {
	unitConstraints         string
	hardwareCharacteristics string
	assignOk                bool
}{
	{
		// 0
		unitConstraints:         "",
		hardwareCharacteristics: "arch=amd64",
		assignOk:                true,
	}, {
		// 1
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "none",
		assignOk:                false,
	}, {
		// 2
		unitConstraints:         "arch=amd64",
		hardwareCharacteristics: "cores=1",
		assignOk:                false,
	}, {
		// 3
		unitConstraints:         "",
		hardwareCharacteristics: "arch=s390x",
		assignOk:                false,
	}, {
		// 4
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "none",
		assignOk:                false,
	}, {
		// 5
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "cores=1",
		assignOk:                false,
	}, {
		// 6
		unitConstraints:         "arch=amd64 mem=4G",
		hardwareCharacteristics: "arch=amd64 mem=4G",
		assignOk:                true,
	}, {
		// 7
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "arch=amd64 mem=4G",
		assignOk:                true,
	}, {
		// 8
		unitConstraints:         "arch=amd64 mem=4G",
		hardwareCharacteristics: "arch=amd64 mem=2G",
		assignOk:                false,
	}, {
		// 9
		unitConstraints:         "mem=4G",
		hardwareCharacteristics: "mem=2G",
		assignOk:                false,
	}, {
		// 10
		unitConstraints:         "arch=amd64 cores=2",
		hardwareCharacteristics: "arch=amd64 cores=2",
		assignOk:                true,
	}, {
		// 11
		unitConstraints:         "cores=2",
		hardwareCharacteristics: "arch=amd64 cores=2",
		assignOk:                true,
	}, {
		// 12
		unitConstraints:         "arch=amd64 cores=2",
		hardwareCharacteristics: "arch=amd64 cores=1",
		assignOk:                false,
	}, {
		// 13
		unitConstraints:         "cores=2",
		hardwareCharacteristics: "cores=1",
		assignOk:                false,
	}, {
		// 14
		unitConstraints:         "arch=amd64 cores=2",
		hardwareCharacteristics: "arch=amd64 mem=4G",
		assignOk:                false,
	}, {
		// 15
		unitConstraints:         "cores=2",
		hardwareCharacteristics: "mem=4G",
		assignOk:                false,
	}, {
		// 16
		unitConstraints:         "arch=amd64 cpu-power=50",
		hardwareCharacteristics: "arch=amd64 cpu-power=50",
		assignOk:                true,
	}, {
		// 17
		unitConstraints:         "cpu-power=50",
		hardwareCharacteristics: "arch=amd64 cpu-power=50",
		assignOk:                true,
	}, {
		// 18
		unitConstraints:         "arch=amd64 cpu-power=100",
		hardwareCharacteristics: "arch=amd64 cpu-power=50",
		assignOk:                false,
	}, {
		// 19
		unitConstraints:         "cpu-power=100",
		hardwareCharacteristics: "cpu-power=50",
		assignOk:                false,
	}, {
		// 20
		unitConstraints:         "arch=amd64 cpu-power=50",
		hardwareCharacteristics: "arch=amd64 mem=4G",
		assignOk:                false,
	}, {
		// 21
		unitConstraints:         "cpu-power=50",
		hardwareCharacteristics: "mem=4G",
		assignOk:                false,
	}, {
		// 22
		unitConstraints:         "arch=amd64 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 cpu-power=50",
		assignOk:                false,
	}, {
		// 23
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "cpu-power=50",
		assignOk:                false,
	}, {
		// 24
		unitConstraints:         "arch=amd64 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 root-disk=4096",
		assignOk:                false,
	}, {
		// 25
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "root-disk=4096",
		assignOk:                false,
	}, {
		// 26
		unitConstraints:         "arch=amd64 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 root-disk=8192",
		assignOk:                true,
	}, {
		// 27
		unitConstraints:         "root-disk=8192",
		hardwareCharacteristics: "arch=amd64 root-disk=8192",
		assignOk:                true,
	}, {
		// 28
		unitConstraints:         "root-disk-source=place1",
		hardwareCharacteristics: "root-disk-source=place2",
		assignOk:                false,
	}, {
		// 29
		unitConstraints:         "arch=amd64 root-disk-source=place1",
		hardwareCharacteristics: "arch=amd64 root-disk-source=place1",
		assignOk:                true,
	}, {
		// 30
		unitConstraints:         "arch=amd64 mem=4G cores=2 root-disk=8192",
		hardwareCharacteristics: "arch=amd64 mem=8G cores=2 root-disk=8192 root-disk-source=donk cpu-power=50",
		assignOk:                true,
	}, {
		// 31
		unitConstraints:         "arch=amd64 mem=4G cores=2 root-disk=8192 root-disk-source=donk",
		hardwareCharacteristics: "arch=amd64 mem=8G cores=1 root-disk=4096 root-disk-source=donk cpu-power=50",
		assignOk:                false,
	},
}

func (s *assignCleanSuite) TestAssignUsingConstraintsToMachine(c *tc.C) {
	for i, t := range assignUsingConstraintsTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.unitConstraints)
		err := s.State.SetModelConstraints(cons)
		c.Assert(err, tc.ErrorIsNil)

		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)

		m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, tc.ErrorIsNil)
		if t.hardwareCharacteristics != "none" {
			hc := instance.MustParseHardware(t.hardwareCharacteristics)
			err = m.SetProvisioned("inst-id", "", "fake_nonce", &hc)
			c.Assert(err, tc.ErrorIsNil)
		}

		um, err := s.assignUnit(unit)
		if t.assignOk {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(um.Id(), tc.Equals, m.Id())
		} else {
			c.Assert(um, tc.IsNil)
			c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)
			// Destroy the machine so it can't be used for the next test.
			err = m.Destroy()
			c.Assert(err, tc.ErrorIsNil)
		}
	}
}

func (s *assignCleanSuite) TestAssignUnitWithRemovedApplication(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Fail if application is removed.
	removeAllUnits(c, s.wordpress)
	err = s.wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.assignUnit(unit)
	c.Assert(err, tc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.* not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWithRemovedUnit(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Fail if unit is removed.
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.assignUnit(unit)
	c.Assert(err, tc.ErrorMatches, s.errorMessage(`cannot assign unit "wordpress/0" to %s machine.*: unit not found`))
}

func (s *assignCleanSuite) TestAssignUnitToMachineWorksWithMachine0(c *tc.C) {
	amdArch := "amd64"
	hwChar := &instance.HardwareCharacteristics{
		Arch: &amdArch,
	}

	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0")

	instId := instance.Id("i-host-machine")
	err = m.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	assignedTo, err := s.assignUnit(unit)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(assignedTo.Id(), tc.Equals, "0")
}

func (s *assignCleanSuite) setupSingleStorage(c *tc.C, kind, pool string) (*state.Application, *state.Unit, names.StorageTag) {
	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	return application, unit, storageTag
}

func (s *assignCleanSuite) TestAssignToMachine(c *tc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	filesystemAttachments, err := sb.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystemAttachments, tc.HasLen, 1)
}

func (s *assignCleanSuite) TestAssignToMachineErrors(c *tc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(
		err, tc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: "static" storage provider does not support dynamic storage`,
	)

	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(container)
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "storage-filesystem/0" to machine 0/lxd/0: adding storage of type "static" to lxd container not supported`)
}

func (s *assignCleanSuite) TestAssignUnitWithNonDynamicStorageCleanAvailable(c *tc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, tc.Not(tc.Equals), clean.Id())
}

func (s *assignCleanSuite) TestAssignUnitWithNonDynamicStorageAndMachinePlacementDirective(c *tc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	placement := &instance.Placement{
		instance.MachineScope, clean.Id(),
	}
	err = s.State.AssignUnitWithPlacement(unit, placement)
	c.Assert(
		err, tc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: "static" storage provider does not support dynamic storage`,
	)
}

func (s *assignCleanSuite) TestAssignUnitWithNonDynamicStorageAndZonePlacementDirective(c *tc.C) {
	_, unit, _ := s.setupSingleStorage(c, "filesystem", "static")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty. Since
	// the unit has non dynamic storage instances associated,
	// it will be forced onto a new machine.
	placement := &instance.Placement{
		s.State.ModelUUID(), "zone=test",
	}
	err = s.State.AssignUnitWithPlacement(unit, placement)
	c.Assert(err, tc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, tc.Not(tc.Equals), clean.Id())
}

func (s *assignCleanSuite) TestAssignUnitWithDynamicStorageCleanAvailable(c *tc.C) {
	amdArch := "amd64"
	hwChar := &instance.HardwareCharacteristics{
		Arch: &amdArch,
	}

	_, unit, _ := s.setupSingleStorage(c, "filesystem", "loop-pool")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)

	// Add a clean machine.
	clean, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	instId := instance.Id("i-host-machine")
	err = clean.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	// assign the unit to a machine, requesting clean/empty
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	// Check the machine on the unit is set.
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	// Check that the machine isn't our clean one.
	c.Assert(machineId, tc.Equals, clean.Id())

	// Check that a volume attachments were added to the machine.
	machine, err := s.State.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 1)

	volume, err := sb.Volume(volumeAttachments[0].Volume())
	c.Assert(err, tc.ErrorIsNil)
	volumeStorageInstance, err := volume.StorageInstance()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeStorageInstance, tc.Equals, storageAttachments[0].StorageInstance())
}

func (s *assignCleanSuite) TestAssignUnitPolicy(c *tc.C) {
	amdArch := "amd64"
	hwChar := &instance.HardwareCharacteristics{
		Arch: &amdArch,
	}

	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)

	// Check unassigned placements with no clean and/or empty machines.
	for i := 0; i < 10; i++ {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, tc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mid, tc.Equals, strconv.Itoa(1+i))
		assertMachineCount(c, s.State, i+2)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, tc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(units, tc.HasLen, 1)
		c.Assert(units[0].Name(), tc.Equals, unit.Name())
		c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	}

	// Remove units from alternate machines. These machines will still be
	// considered as dirty so will continue to be ignored by the policy.
	for i := 1; i < 11; i += 2 {
		mid := strconv.Itoa(i)
		m, err := s.State.Machine(mid)
		c.Assert(err, tc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(units, tc.HasLen, 1)
		unit := units[0]
		err = unit.UnassignFromMachine()
		c.Assert(err, tc.ErrorIsNil)
		err = unit.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}

	var expectedMachines []string
	// Create a new, clean machine but add containers so it is not empty.
	hostMachine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	instId := instance.Id("i-host-machine")
	err = hostMachine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostMachine.Clean(), tc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	instId = instance.Id("i-container")
	err = container.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	if s.policy == state.AssignClean {
		expectedMachines = append(expectedMachines, hostMachine.Id())
	}
	expectedMachines = append(expectedMachines, container.Id())

	// Add some more clean machines
	for i := 0; i < 4; i++ {
		m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, tc.ErrorIsNil)

		instId = instance.Id(fmt.Sprintf("i-machine-%d", i))
		err = m.SetProvisioned(instId, "", "fake-nonce", hwChar)
		c.Assert(err, tc.ErrorIsNil)

		expectedMachines = append(expectedMachines, m.Id())
	}

	// Assign units to all the expectedMachines machines.
	var got []string
	for range expectedMachines {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, tc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		got = append(got, mid)
	}
	sort.Strings(expectedMachines)
	sort.Strings(got)
	c.Assert(got, tc.DeepEquals, expectedMachines)
}

func (s *assignCleanSuite) TestAssignUnitPolicyWithContainers(c *tc.C) {
	amdArch := "amd64"
	hwChar := &instance.HardwareCharacteristics{
		Arch: &amdArch,
	}

	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)

	// Create a machine and add a new container.
	hostMachine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	instId := instance.Id("i-host-machine")
	err = hostMachine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, hostMachine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	err = hostMachine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostMachine.Clean(), tc.IsTrue)
	s.assertMachineNotEmpty(c, hostMachine)

	instId = instance.Id("i-container")
	err = container.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	// Set up constraints to specify we want to install into a container.
	econs := constraints.MustParse("container=lxd")
	err = s.State.SetModelConstraints(econs)
	c.Assert(err, tc.ErrorIsNil)

	// Check the first placement goes into the newly created, clean container above.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, tc.ErrorIsNil)
	mid, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, container.Id())

	assertContainerPlacement := func(expectedNumUnits int) {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.State.AssignUnit(unit, s.policy)
		c.Assert(err, tc.ErrorIsNil)
		mid, err := unit.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mid, tc.Equals, fmt.Sprintf("%d/lxd/0", expectedNumUnits+1))
		assertMachineCount(c, s.State, 2*expectedNumUnits+3)

		// Sanity check that the machine knows about its assigned unit and was
		// created with the appropriate series.
		m, err := s.State.Machine(mid)
		c.Assert(err, tc.ErrorIsNil)
		units, err := m.Units()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(units, tc.HasLen, 1)
		c.Assert(units[0].Name(), tc.Equals, unit.Name())
		c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	}

	// Check unassigned placements with no clean and/or empty machines cause a new container to be created.
	assertContainerPlacement(1)
	assertContainerPlacement(2)

	// Create a new, clean instance and check that the next container creation uses it.
	hostMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	instId = instance.Id("i-host-machine")
	err = hostMachine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.AssignUnit(unit, s.policy)
	c.Assert(err, tc.ErrorIsNil)
	mid, err = unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, hostMachine.Id()+"/lxd/0")
}

func (s *assignCleanSuite) TestAssignUnitPolicyConcurrently(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel) // bootstrap machine
	c.Assert(err, tc.ErrorIsNil)
	unitCount := 50
	if raceDetector {
		unitCount = 10
	}
	us := make([]*state.Unit, unitCount)
	for i := range us {
		us[i], err = s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
	}
	type result struct {
		u   *state.Unit
		err error
	}
	done := make(chan result)
	for i, u := range us {
		i, u := i, u
		go func() {
			// Start the AssignUnit at different times
			// to increase the likeliness of a race.
			time.Sleep(time.Duration(i) * time.Millisecond / 2)
			err := s.State.AssignUnit(u, s.policy)
			done <- result{u, err}
		}()
	}
	assignments := make(map[string][]*state.Unit)
	for range us {
		r := <-done
		if !c.Check(r.err, tc.IsNil) {
			continue
		}
		id, err := r.u.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		assignments[id] = append(assignments[id], r.u)
	}
	for id, us := range assignments {
		if len(us) != 1 {
			c.Errorf("machine %s expected one unit, got %q", id, us)
		}
	}
	c.Assert(assignments, tc.HasLen, len(us))
}
