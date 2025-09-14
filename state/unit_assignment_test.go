// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
)

type UnitAssignmentSuite struct {
	ConnSuite
}

func TestUnitAssignmentSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &UnitAssignmentSuite{})
}

func (s *UnitAssignmentSuite) testAddApplicationUnitAssignment(c *tc.C) (*state.Application, []state.UnitAssignment) {
	charm := s.AddTestingCharm(c, "dummy")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "dummy", Charm: charm, NumUnits: 2,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		Placement: []*instance.Placement{{s.State.ModelUUID(), "abc"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 2)
	for _, u := range units {
		_, err := u.AssignedMachineId()
		c.Assert(err, tc.Satisfies, errors.IsNotAssigned)
	}

	assignments, err := s.State.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(assignments, tc.SameContents, []state.UnitAssignment{
		{Unit: "dummy/0", Scope: s.State.ModelUUID(), Directive: "abc"},
		{Unit: "dummy/1"},
	})
	return app, assignments
}

func (s *UnitAssignmentSuite) TestAddApplicationUnitAssignment(c *tc.C) {
	s.testAddApplicationUnitAssignment(c)
}

func (s *UnitAssignmentSuite) TestAssignStagedUnits(c *tc.C) {
	app, _ := s.testAddApplicationUnitAssignment(c)

	results, err := s.State.AssignStagedUnits([]string{
		"dummy/0", "dummy/1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.SameContents, []state.UnitAssignmentResult{
		{Unit: "dummy/0"},
		{Unit: "dummy/1"},
	})

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 2)
	for _, u := range units {
		_, err := u.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
	}

	// There should be no staged assignments now.
	assignments, err := s.State.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(assignments, tc.HasLen, 0)
}

func (s *UnitAssignmentSuite) TestAssignUnitWithPlacementMakesContainerInNewMachine(c *tc.C) {
	// Enables juju deploy <charm> --to <container-type>
	// It creates a new machine with a new container of that type.
	// https://bugs.launchpad.net/juju-core/+bug/1590960
	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: "lxd"}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits:  1,
		Placement: []*instance.Placement{&placement},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)
	unit := units[0]

	err = s.State.AssignUnitWithPlacement(unit, &placement)
	c.Assert(err, tc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)
	parentId, isContainer := machine.ParentId()
	c.Assert(isContainer, tc.IsTrue)
	_, err = s.State.Machine(parentId)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UnitAssignmentSuite) TestAssignUnitWithPlacementNewMachinesHaveBindingsAsConstraints(c *tc.C) {
	specialSpace, err := s.State.AddSpace("special-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: "lxd"}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits:  1,
		Placement: []*instance.Placement{&placement},
		EndpointBindings: map[string]string{
			"": specialSpace.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)
	unit := units[0]

	err = s.State.AssignUnitWithPlacement(unit, &placement)
	c.Assert(err, tc.ErrorIsNil)

	guestID, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)

	guest, err := s.State.Machine(guestID)
	c.Assert(err, tc.ErrorIsNil)

	hostID, _ := guest.ParentId()
	host, err := s.State.Machine(hostID)
	c.Assert(err, tc.ErrorIsNil)

	for _, m := range []*state.Machine{guest, host} {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(cons.IncludeSpaces(), tc.DeepEquals, []string{"special-space"})
	}
}

func (s *UnitAssignmentSuite) TestAssignUnitWithPlacementNewMachinesHaveBindingsAsConstraintsMerged(c *tc.C) {
	boundSpace, err := s.State.AddSpace("bound-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	constrainedSpace, err := s.State.AddSpace("constrained-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: "lxd"}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits:  1,
		Placement: []*instance.Placement{&placement},
		// Same space used in both bindings and constraints to test merging.
		Constraints: constraints.MustParse("spaces=bound-space,constrained-space"),
		EndpointBindings: map[string]string{
			"": boundSpace.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)
	unit := units[0]

	err = s.State.AssignUnitWithPlacement(unit, &placement)
	c.Assert(err, tc.ErrorIsNil)

	guestID, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)

	guest, err := s.State.Machine(guestID)
	c.Assert(err, tc.ErrorIsNil)

	hostID, _ := guest.ParentId()
	host, err := s.State.Machine(hostID)
	c.Assert(err, tc.ErrorIsNil)

	for _, m := range []*state.Machine{guest, host} {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(cons.IncludeSpaces(), tc.SameContents, []string{boundSpace.Name(), constrainedSpace.Name()})
	}
}

func (s *UnitAssignmentSuite) TestAssignUnitWithPlacementDirective(c *tc.C) {
	// Enables juju deploy <charm> --to <container-type>
	// It creates a new machine with a new container of that type.
	// https://bugs.launchpad.net/juju-core/+bug/1590960
	charm := s.AddTestingCharm(c, "dummy")
	placement := instance.Placement{Scope: s.State.ModelUUID(), Directive: "zone=test"}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits:  1,
		Placement: []*instance.Placement{&placement},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)
	unit := units[0]

	err = s.State.AssignUnitWithPlacement(unit, &placement)
	c.Assert(err, tc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Placement(), tc.Equals, "zone=test")
}

func (s *UnitAssignmentSuite) TestAssignUnitCleanMachineUpgradeSeriesLockError(c *tc.C) {
	s.addLockedMachine(c, true)

	charm := s.AddTestingCharm(c, "dummy")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		NumUnits: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	unit := units[0]
	_, err = unit.AssignToCleanEmptyMachine()
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)
}

func (s *UnitAssignmentSuite) TestAssignUnitMachinePlacementUpgradeSeriesLockError(c *tc.C) {
	machine, _ := s.addLockedMachine(c, false)
	// As in --to 0
	s.testPlacementUpgradeSeriesLockError(c, &instance.Placement{Scope: "#", Directive: machine.Id()})
}

func (s *UnitAssignmentSuite) TestAssignUnitContainerOnMachinePlacementUpgradeSeriesLockError(c *tc.C) {
	machine, _ := s.addLockedMachine(c, false)
	// As in --to lxd:0
	s.testPlacementUpgradeSeriesLockError(c, &instance.Placement{Scope: "lxd", Directive: machine.Id()})
}

func (s *UnitAssignmentSuite) TestAssignUnitExtantContainerOnMachinePlacementUpgradeSeriesLockError(c *tc.C) {
	_, child := s.addLockedMachine(c, true)

	// As in --to 0/lxd/0
	s.testPlacementUpgradeSeriesLockError(c, &instance.Placement{Scope: "#", Directive: child.Id()})
}

func (s *UnitAssignmentSuite) testPlacementUpgradeSeriesLockError(c *tc.C, placement *instance.Placement) {
	charm := s.AddTestingCharm(c, "dummy")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
		NumUnits:  1,
		Placement: []*instance.Placement{placement},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	unit := units[0]
	err = s.State.AssignUnitWithPlacement(unit, placement)
	c.Assert(err, tc.ErrorMatches, ".* is locked for series upgrade")
}

func (s *UnitAssignmentSuite) addLockedMachine(c *tc.C, addContainer bool) (*state.Machine, *state.Machine) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	var child *state.Machine
	if addContainer {
		template := state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		}
		child, err = s.State.AddMachineInsideMachine(template, machine.Id(), "lxd")
		c.Assert(err, tc.ErrorIsNil)
	}

	c.Assert(machine.CreateUpgradeSeriesLock(nil, state.UbuntuBase("22.04")), tc.ErrorIsNil)
	return machine, child
}
