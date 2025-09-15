// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	corecontainer "github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
)

type MachineSuite struct {
	ConnSuite
	machine0 *state.Machine
	machine  *state.Machine
}

func TestMachineSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MachineSuite{})
}

func (s *MachineSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	var err error
	s.machine0, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	controllerNode, err := s.State.ControllerNode(s.machine0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerNode.HasVote(), tc.IsFalse)
	c.Assert(controllerNode.SetHasVote(true), tc.ErrorIsNil)
	s.machine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.ControllerNode(s.machine.Id())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestSetRebootFlagDeadMachine(c *tc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetRebootFlag(true)
	c.Assert(err, tc.Equals, mgo.ErrNotFound)

	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsFalse)

	err = s.machine.SetRebootFlag(false)
	c.Assert(err, tc.ErrorIsNil)

	action, err := s.machine.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(action, tc.Equals, state.ShouldDoNothing)
}

func (s *MachineSuite) TestSetRebootFlagDeadMachineRace(c *tc.C) {
	setFlag := jujutxn.TestHook{
		Before: func() {
			err := s.machine.EnsureDead()
			c.Assert(err, tc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, setFlag).Check()

	err := s.machine.SetRebootFlag(true)
	c.Assert(err, tc.Equals, mgo.ErrNotFound)
}

func (s *MachineSuite) TestSetRebootFlag(c *tc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)

	rebootFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootFlag, tc.IsTrue)
}

func (s *MachineSuite) TestSetUnsetRebootFlag(c *tc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)

	rebootFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootFlag, tc.IsTrue)

	err = s.machine.SetRebootFlag(false)
	c.Assert(err, tc.ErrorIsNil)

	rebootFlag, err = s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootFlag, tc.IsFalse)
}

func (s *MachineSuite) TestRecordAgentStartInformation(c *tc.C) {
	now := s.Clock.Now().Truncate(time.Minute)
	err := s.machine.RecordAgentStartInformation("thundering-herds")
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.AgentStartTime().Truncate(time.Minute), tc.Equals, now)
	c.Assert(s.machine.Hostname(), tc.Equals, "thundering-herds")

	// Passing an empty hostname should be ignored
	err = s.machine.RecordAgentStartInformation("")
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.AgentStartTime().Truncate(time.Minute), tc.Equals, now)
	c.Assert(s.machine.Hostname(), tc.Equals, "thundering-herds", tc.Commentf("expected the host name not be changed"))
}

func (s *MachineSuite) TestSetKeepInstance(c *tc.C) {
	err := s.machine.SetProvisioned("1234", "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.SetKeepInstance(true)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	keep, err := m.KeepInstance()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keep, tc.IsTrue)
}

func (s *MachineSuite) TestAddMachineInsideMachineModelDying(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorMatches, `model "testmodel" is dying`)
}

func (s *MachineSuite) TestAddMachineInsideMachineModelMigrating(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.SetMigrationMode(state.MigrationModeExporting), tc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorMatches, `model "testmodel" is being migrated`)
}

func (s *MachineSuite) TestShouldShutdownOrReboot(c *tc.C) {
	// Add first container.
	c1, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	// Add second container.
	c2, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, c1.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	err = c2.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)

	rAction, err := s.machine.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldDoNothing)

	rAction, err = c1.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldDoNothing)

	rAction, err = c2.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldReboot)

	// // Reboot happens on the root node
	err = c2.SetRebootFlag(false)
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)

	rAction, err = s.machine.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldReboot)

	rAction, err = c1.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldShutdown)

	rAction, err = c2.ShouldRebootOrShutdown()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, state.ShouldShutdown)
}

func (s *MachineSuite) TestContainerDefaults(c *tc.C) {
	c.Assert(string(s.machine.ContainerType()), tc.Equals, "")
	containers, err := s.machine.Containers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.DeepEquals, []string(nil))
}

func (s *MachineSuite) TestParentId(c *tc.C) {
	parentId, ok := s.machine.ParentId()
	c.Assert(parentId, tc.Equals, "")
	c.Assert(ok, tc.IsFalse)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	parentId, ok = container.ParentId()
	c.Assert(parentId, tc.Equals, s.machine.Id())
	c.Assert(ok, tc.IsTrue)
}

func (s *MachineSuite) TestMachineIsManager(c *tc.C) {
	c.Assert(s.machine0.IsManager(), tc.IsTrue)
	c.Assert(s.machine.IsManager(), tc.IsFalse)
}

func (s *MachineSuite) TestMachineIsManualBootstrap(c *tc.C) {
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Type(), tc.Not(tc.Equals), "null")
	c.Assert(s.machine.Id(), tc.Equals, "1")
	manual, err := s.machine0.IsManual()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manual, tc.IsFalse)
	attrs := map[string]interface{}{"type": "null"}
	err = s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, tc.ErrorIsNil)
	manual, err = s.machine0.IsManual()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manual, tc.IsTrue)
}

func (s *MachineSuite) TestMachineIsManual(c *tc.C) {
	tests := []struct {
		instanceId instance.Id
		nonce      string
		isManual   bool
	}{
		{instanceId: "x", nonce: "y", isManual: false},
		{instanceId: "manual:", nonce: "y", isManual: false},
		{instanceId: "x", nonce: "manual:", isManual: true},
		{instanceId: "x", nonce: "manual:y", isManual: true},
		{instanceId: "x", nonce: "manual", isManual: false},
	}
	for _, test := range tests {
		m, err := s.State.AddOneMachine(state.MachineTemplate{
			Base:       state.UbuntuBase("12.10"),
			Jobs:       []state.MachineJob{state.JobHostUnits},
			InstanceId: test.instanceId,
			Nonce:      test.nonce,
		})
		c.Assert(err, tc.ErrorIsNil)
		isManual, err := m.IsManual()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(isManual, tc.Equals, test.isManual)
	}
}

func (s *MachineSuite) TestMachineIsContainer(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machine.IsContainer(), tc.IsFalse)
	c.Assert(container.IsContainer(), tc.IsTrue)
}

func (s *MachineSuite) TestLifeJobController(c *tc.C) {
	m := s.machine0
	err := m.Destroy()
	c.Assert(err, tc.ErrorMatches, "controller 0 is the only controller")
	controllerNode, err := s.State.ControllerNode(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerNode.HasVote(), tc.IsTrue)
	err = m.EnsureDead()
	c.Assert(err, tc.ErrorMatches, "machine 0 is still a voting controller member")
	controllerNode, err = s.State.ControllerNode(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerNode.HasVote(), tc.IsTrue)

	// Since this is the only controller machine, we cannot even force destroy it
	err = m.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorMatches, "controller 0 is the only controller")
	err = m.EnsureDead()
	c.Assert(err, tc.ErrorMatches, "machine 0 is still a voting controller member")
}

func (s *MachineSuite) TestLifeJobManageModelWithControllerCharm(c *tc.C) {
	cons := constraints.Value{
		Mem: newUint64(100),
	}
	changes, err := s.State.EnableHA(3, cons, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)

	m2, err := s.State.Machine("2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m2.Jobs(), tc.DeepEquals, []state.MachineJob{
		state.JobHostUnits,
		state.JobManageModel,
	})

	ch := s.AddTestingCharmWithSeries(c, "juju-controller", "")
	app := s.AddTestingApplicationForBase(c, state.UbuntuBase("12.10"), "controller", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetCharmURL(ch.URL())
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(m2)
	c.Assert(err, tc.ErrorIsNil)
	err = m2.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(m2.Life(), tc.Equals, state.Alive)
	c.Assert(err, tc.ErrorIsNil)

	for i := 0; i < 3; i++ {
		needsCleanup, err := s.State.NeedsCleanup()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(needsCleanup, tc.IsTrue)
	}
	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needsCleanup, tc.IsTrue)

	err = m2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	// Force remove of controller machines will first clean up units and
	// after that it will pass the machine life to Dying.
	c.Assert(m2.Life(), tc.Equals, state.Alive)

	cn2, err := s.State.ControllerNode(m2.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.RemoveControllerReference(cn2)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)

	err = m2.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m2.Life(), tc.Equals, state.Dead)
}

func (s *MachineSuite) TestLifeMachineWithContainer(c *tc.C) {
	// A machine hosting a container must not advance lifecycle.
	_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Destroy()
	c.Assert(errors.Is(err, stateerrors.HasContainersError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `machine 1 is hosting containers "1/lxd/0"`)

	err = s.machine.EnsureDead()
	c.Assert(errors.Is(err, stateerrors.HasContainersError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `machine 1 is hosting containers "1/lxd/0"`)

	c.Assert(s.machine.Life(), tc.Equals, state.Alive)
}

func (s *MachineSuite) TestLifeMachineLockedForSeriesUpgrade(c *tc.C) {
	err := s.machine.CreateUpgradeSeriesLock(nil, state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorMatches, `machine 1 is locked for series upgrade`)

	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorMatches, `machine 1 is locked for series upgrade`)
	c.Assert(s.machine.Life(), tc.Equals, state.Alive)
}

func (s *MachineSuite) TestLifeJobHostUnits(c *tc.C) {
	// A machine with an assigned unit must not advance lifecycle.
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `machine 1 has unit "wordpress/0" assigned`)

	err = s.machine.EnsureDead()
	c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `machine 1 has unit "wordpress/0" assigned`)

	c.Assert(s.machine.Life(), tc.Equals, state.Alive)

	// Once no unit is assigned, lifecycle can advance.
	err = unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(s.machine.Life(), tc.Equals, state.Dying)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.Life(), tc.Equals, state.Dead)

	// A machine that has never had units assigned can advance lifecycle.
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = m.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)
	err = m.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dead)
}

func (s *MachineSuite) TestDestroyRemovePorts(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)

	state.MustOpenUnitPortRange(c, s.State, s.machine, unit.Name(), allEndpoints, network.MustParsePortRange("8080/tcp"))

	machPortRanges, err := s.machine.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), tc.HasLen, 1)

	err = unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	// once the machine is destroyed, there should be no ports documents present for it
	machPortRanges, err = s.machine.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	assertMachinePortsPersisted(c, machPortRanges, false) // we should get back a blank fresh doc
}

func (s *MachineSuite) TestDestroyOps(c *tc.C) {
	m := s.Factory.MakeMachine(c, nil)
	ops, err := state.ForceDestroyMachineOps(m)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.NotNil)
}

func (s *MachineSuite) TestDestroyOpsForManagerFails(c *tc.C) {
	// s.Factory does not allow us to make a manager machine, so we grab one
	// from State ...
	machines, err := s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(machines), tc.GreaterThan, 0)
	m := machines[0]
	c.Assert(m.IsManager(), tc.IsTrue)

	// ... and assert that we cannot get the destroy ops for it.
	ops, err := state.ForceDestroyMachineOps(m)
	c.Assert(err, tc.ErrorMatches, `controller 0 is the only controller`)
	c.Assert(ops, tc.IsNil)
}

func (s *MachineSuite) TestDestroyAbort(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Destroy(), tc.IsNil)
	}).Check()
	err := s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestDestroyCancel(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), tc.IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), tc.IsTrue)
}

func (s *MachineSuite) TestDestroyContention(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	perturb := jujutxn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(s.machine), tc.IsNil) },
		After:  func() { c.Assert(unit.UnassignFromMachine(), tc.IsNil) },
	}
	state.SetMaxTxnAttempts(c, s.State, 3)
	defer state.SetTestHooks(c, s.State, perturb, perturb, perturb).Check()

	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorMatches, "machine 1 cannot advance lifecycle: state changing too quickly; try again soon")
}

func (s *MachineSuite) TestDestroyWithApplicationDestroyPending(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// Machine is still advanced to Dying.
	life := s.machine.Life()
	c.Assert(life, tc.Equals, state.Dying)
}

func (s *MachineSuite) TestDestroyFailsWhenNewUnitAdded(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		anotherApp := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
		anotherUnit, err := anotherApp.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = anotherUnit.AssignToMachine(s.machine)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err = s.machine.Destroy()
	c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), tc.IsTrue)
	life := s.machine.Life()
	c.Assert(life, tc.Equals, state.Alive)
}

func (s *MachineSuite) TestDestroyWithUnitDestroyPending(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)

	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// Machine is still advanced to Dying.
	life := s.machine.Life()
	c.Assert(life, tc.Equals, state.Dying)
}

func (s *MachineSuite) TestDestroyFailsWhenNewContainerAdded(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		}, s.machine.Id(), instance.LXD)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err = s.machine.Destroy()
	c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), tc.IsTrue)
	life := s.machine.Life()
	c.Assert(life, tc.Equals, state.Alive)
}

func (s *MachineSuite) TestRemove(c *tc.C) {
	arch := arch.DefaultArchitecture
	char := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	err := s.machine.SetProvisioned("umbrella/0", "snowflake", "fake_nonce", char)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SetSSHHostKeys(s.machine.MachineTag(), state.SSHHostKeys{"rsa", "dsa"})
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Remove()
	c.Assert(err, tc.ErrorMatches, "cannot remove machine 1: machine is not dead")

	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.machine.HardwareCharacteristics()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.machine.Containers()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.State.GetSSHHostKeys(s.machine.MachineTag())
	c.Assert(errors.IsNotFound(err), tc.IsTrue)

	// Removing an already removed machine is OK.
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestRemoveAbort(c *tc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Remove(), tc.IsNil)
	}).Check()
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestTag(c *tc.C) {
	tag := s.machine.MachineTag()
	c.Assert(tag.Kind(), tc.Equals, names.MachineTagKind)
	c.Assert(tag.Id(), tc.Equals, "1")

	// To keep gccgo happy, don't compare an interface with a struct.
	var asTag names.Tag = tag
	c.Assert(s.machine.Tag(), tc.Equals, asTag)
}

func (s *MachineSuite) TestSetMongoPassword(c *tc.C) {
	testSetMongoPassword(c, func(st *state.State, id string) (mongoPasswordSetter, error) {
		return st.Machine("0")
	}, s.State.ControllerTag(), s.modelTag, s.Session)
}

type mongoPasswordSetter interface {
	SetMongoPassword(password string) error
	Tag() names.Tag
}

type mongoPasswordSetterGetter func(st *state.State, id string) (mongoPasswordSetter, error)

func testSetMongoPassword(
	c *tc.C, entityFunc mongoPasswordSetterGetter,
	controllerTag names.ControllerTag, modelTag names.ModelTag, mgoSession *mgo.Session,
) {
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       mgoSession,
	})
	c.Assert(err, tc.ErrorIsNil)
	st, err := pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		// Remove the admin password so that the test harness can reset the state.
		err := st.SetAdminMongoPassword("")
		c.Check(err, tc.ErrorIsNil)
		err = pool.Close()
		c.Check(err, tc.ErrorIsNil)
	}()

	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, tc.ErrorIsNil)
	err = st.MongoSession().DB("admin").Login("admin", "admin-secret")
	c.Assert(err, tc.ErrorIsNil)

	// Set the password for the entity
	ent, err := entityFunc(st, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = ent.SetMongoPassword("foo")
	c.Assert(err, tc.ErrorIsNil)

	// Check that we cannot log in with the wrong password.
	info := testing.NewMongoInfo()
	info.Tag = ent.Tag()
	info.Password = "bar"
	err = tryOpenState(modelTag, controllerTag, info)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsUnauthorized)
	c.Check(err, tc.ErrorMatches, `cannot log in to admin database as "(machine|controller)-0": unauthorized mongo access: .*`)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	pool1, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer pool1.Close()
	st1, err := pool1.SystemState()
	c.Assert(err, tc.ErrorIsNil)

	// Change the password with an entity derived from the newly
	// opened and authenticated state.
	ent, err = entityFunc(st1, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = ent.SetMongoPassword("bar")
	c.Assert(err, tc.ErrorIsNil)

	// Check that we cannot log in with the old password.
	info.Password = "foo"
	err = tryOpenState(modelTag, controllerTag, info)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsUnauthorized)
	c.Check(err, tc.ErrorMatches, `cannot log in to admin database as "(machine|controller)-0": unauthorized mongo access: .*`)

	// Check that we can log in with the correct password.
	info.Password = "bar"
	err = tryOpenState(modelTag, controllerTag, info)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the administrator can still log in.
	info.Tag, info.Password = nil, "admin-secret"
	err = tryOpenState(modelTag, controllerTag, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestSetPassword(c *tc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$set", bson.D{{"instanceid", bson.D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
	c.Assert(iid, tc.Equals, instance.Id(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *tc.C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
	c.Assert(string(iid), tc.Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$set", bson.D{{"instanceid", ""}}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
	c.Assert(string(iid), tc.Equals, "")
}

func (s *MachineSuite) TestMachineSetProvisionedStoresAndInstanceNamesReturnsDisplayName(c *tc.C) {
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsFalse)
	err := s.machine.SetProvisioned("umbrella/0", "snowflake", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	iid, iname, err := s.machine.InstanceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(iid), tc.Equals, "umbrella/0")
	c.Assert(iname, tc.Equals, "snowflake")

	all, err := s.Model.AllInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	iid, iname = all.InstanceNames(s.machine.Id())
	c.Assert(string(iid), tc.Equals, "umbrella/0")
	c.Assert(iname, tc.Equals, "snowflake")
}

func (s *MachineSuite) TestMachineInstanceNamesReturnsIsNotProvisionedWhenNotProvisioned(c *tc.C) {
	iid, iname, err := s.machine.InstanceNames()
	c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
	c.Assert(string(iid), tc.Equals, "")
	c.Assert(iname, tc.Equals, "")
}

func (s *MachineSuite) TestMachineSetProvisionedUpdatesCharacteristics(c *tc.C) {
	// Before provisioning, there is no hardware characteristics.
	_, err := s.machine.HardwareCharacteristics()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	arch := arch.DefaultArchitecture
	mem := uint64(4096)
	expected := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", expected)
	c.Assert(err, tc.ErrorIsNil)
	md, err := s.machine.HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*md, tc.DeepEquals, *expected)

	// Reload machine and check again.
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	md, err = s.machine.HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*md, tc.DeepEquals, *expected)

	all, err := s.Model.AllInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	md = all.HardwareCharacteristics(s.machine.Id())
	c.Assert(*md, tc.DeepEquals, *expected)
}

func (s *MachineSuite) TestMachineCharmProfiles(c *tc.C) {
	hwc := &instance.HardwareCharacteristics{}
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", hwc)
	c.Assert(err, tc.ErrorIsNil)

	profiles := []string{"secure", "magic"}
	err = s.machine.SetCharmProfiles(profiles)
	c.Assert(err, tc.ErrorIsNil)

	saved, err := s.machine.CharmProfiles()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved, tc.SameContents, profiles)

	all, err := s.Model.AllInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	saved = all.CharmProfiles(s.machine.Id())
	c.Assert(saved, tc.SameContents, profiles)

	// set to nil
	err = s.machine.SetCharmProfiles(nil)
	c.Assert(err, tc.ErrorIsNil)

	saved, err = s.machine.CharmProfiles()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved, tc.HasLen, 0)

	all, err = s.Model.AllInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	saved = all.CharmProfiles(s.machine.Id())
	c.Assert(saved, tc.HasLen, 0)

	// set back to non-nil
	err = s.machine.SetCharmProfiles(profiles)
	c.Assert(err, tc.ErrorIsNil)

	saved, err = s.machine.CharmProfiles()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved, tc.SameContents, profiles)

	all, err = s.Model.AllInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	saved = all.CharmProfiles(s.machine.Id())
	c.Assert(saved, tc.SameContents, profiles)
}

func (s *MachineSuite) TestMachineAvailabilityZone(c *tc.C) {
	zone := "a_zone"
	hwc := &instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", hwc)
	c.Assert(err, tc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(zone, tc.Equals, "a_zone")
}

func (s *MachineSuite) TestContainerAvailabilityZone(c *tc.C) {
	zone := "a_zone"
	hwc := &instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", hwc)
	c.Assert(err, tc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(zone, tc.Equals, "a_zone")

	// now add a container to that machine
	container := s.Factory.MakeMachineNested(c, s.machine.Id(), nil)
	c.Assert(err, tc.ErrorIsNil)

	containerAvailabilityZone, err := container.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containerAvailabilityZone, tc.Equals, "")
}

func (s *MachineSuite) TestMachineAvailabilityZoneEmpty(c *tc.C) {
	zone := ""
	hwc := &instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", hwc)
	c.Assert(err, tc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(zone, tc.Equals, "")
}

func (s *MachineSuite) TestMachineAvailabilityZoneMissing(c *tc.C) {
	hwc := &instance.HardwareCharacteristics{}
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", hwc)
	c.Assert(err, tc.ErrorIsNil)

	zone, err := s.machine.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(zone, tc.Equals, "")
}

func (s *MachineSuite) TestMachineSetCheckProvisioned(c *tc.C) {
	// Check before provisioning.
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsFalse)

	// Either one should not be empty.
	err := s.machine.SetProvisioned("umbrella/0", "", "", nil)
	c.Assert(err, tc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned(instance.Id(""), "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned(instance.Id(""), "", "", nil)
	c.Assert(err, tc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)

	err = s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	id, err := m.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(id), tc.Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsTrue)
	id, err = s.machine.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(id), tc.Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsTrue)

	// Try it twice, it should fail.
	err = s.machine.SetProvisioned(instance.Id("doesn't-matter"), "", "phony", nil)
	c.Assert(err, tc.ErrorMatches, `cannot set instance data for machine "1": already set`)

	// Check it with invalid nonce.
	c.Assert(s.machine.CheckProvisioned("not-really"), tc.IsFalse)
}

func (s *MachineSuite) TestSetProvisionedDupInstanceId(c *tc.C) {
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("dupe-test", &logWriter), tc.IsNil)
	s.AddCleanup(func(*tc.C) {
		_, _ = loggo.RemoveWriter("dupe-test")
	})

	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	anotherMachine, _ := s.Factory.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})
	err = anotherMachine.SetProvisioned("umbrella/0", "", "another_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	found := false
	for _, le := range logWriter.Log() {
		if found = strings.Contains(le.Message, "duplicate instance id"); found == true {
			break
		}
	}
	c.Assert(found, tc.IsTrue)
}

func (s *MachineSuite) TestMachineSetInstanceInfoFailureDoesNotProvision(c *tc.C) {
	assertNotProvisioned := func() {
		c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsFalse)
	}

	assertNotProvisioned()

	invalidVolumes := map[names.VolumeTag]state.VolumeInfo{
		names.NewVolumeTag("1065"): {VolumeId: "vol-ume"},
	}
	err := s.machine.SetInstanceInfo("umbrella/0", "", "fake_nonce", nil, nil, nil, invalidVolumes, nil, nil)
	c.Assert(err, tc.ErrorMatches, `cannot set info for volume \"1065\": volume \"1065\" not found`)
	assertNotProvisioned()

	invalidVolumes = map[names.VolumeTag]state.VolumeInfo{
		names.NewVolumeTag("1065"): {},
	}
	err = s.machine.SetInstanceInfo("umbrella/0", "", "fake_nonce", nil, nil, nil, invalidVolumes, nil, nil)
	c.Assert(err, tc.ErrorMatches, `cannot set info for volume \"1065\": volume ID not set`)
	assertNotProvisioned()

	// TODO(axw) test invalid volume attachment
}

func (s *MachineSuite) addVolume(c *tc.C, params state.VolumeParams, machineId string) names.VolumeTag {
	ops, tag, err := state.AddVolumeOps(s.State, params, machineId)
	c.Assert(err, tc.ErrorIsNil)
	state.RunTransaction(c, s.State, ops)
	return tag
}

func (s *MachineSuite) TestMachineSetInstanceInfoSuccess(c *tc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)

	// Must create the requested block device prior to SetInstanceInfo.
	volumeTag := s.addVolume(c, state.VolumeParams{Size: 1000, Pool: "loop-pool"}, "123")
	c.Assert(volumeTag, tc.Equals, names.NewVolumeTag("123/0"))

	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsFalse)
	volumeInfo := state.VolumeInfo{
		VolumeId: "storage-123",
		Size:     1234,
	}
	volumes := map[names.VolumeTag]state.VolumeInfo{volumeTag: volumeInfo}
	err = s.machine.SetInstanceInfo("umbrella/0", "", "fake_nonce", nil, nil, nil, volumes, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), tc.IsTrue)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	volume, err := sb.Volume(volumeTag)
	c.Assert(err, tc.ErrorIsNil)
	info, err := volume.Info()
	c.Assert(err, tc.ErrorIsNil)
	volumeInfo.Pool = "loop-pool" // taken from params
	c.Assert(info, tc.Equals, volumeInfo)
	volumeStatus, err := volume.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeStatus.Status, tc.Equals, status.Attaching)
}

func (s *MachineSuite) TestMachineSetProvisionedWhenNotAlive(c *tc.C) {
	err := s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestMachineSetProvisionedWhenDead(c *tc.C) {
	err := s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorMatches, ".* neither alive nor dying")
}

func (s *MachineSuite) TestMachineSetInstanceStatus(c *tc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "alive",
		Since:   &now,
	}
	err = s.machine.SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	machineStatus, err := s.machine.InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineStatus.Status, tc.DeepEquals, status.Running)
	c.Assert(machineStatus.Message, tc.DeepEquals, "alive")
}

func (s *MachineSuite) TestMachineSetModificationStatus(c *tc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Applied,
		Message: "applied",
		Since:   &now,
	}
	err = s.machine.SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	machineStatus, err := s.machine.ModificationStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineStatus.Status, tc.DeepEquals, status.Applied)
	c.Assert(machineStatus.Message, tc.DeepEquals, "applied")
}

func (s *MachineSuite) TestMachineRefresh(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	oldTools, _ := m0.AgentTools()
	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = m0.SetAgentVersion(version.MustParseBinary("0.0.3-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	newTools, _ := m0.AgentTools()

	m1Tools, _ := m1.AgentTools()
	c.Assert(m1Tools, tc.DeepEquals, oldTools)
	err = m1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	m1Tools, _ = m1.AgentTools()
	c.Assert(*m1Tools, tc.Equals, *newTools)

	err = m0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = m0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = m0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestRefreshWhenNotAlive(c *tc.C) {
	// Refresh should work regardless of liveness status.
	testWhenDying(c, s.machine, noErr, noErr, func() error {
		return s.machine.Refresh()
	})
}

func (s *MachineSuite) TestMachinePrincipalUnits(c *tc.C) {
	// Check that Machine.Units and st.UnitsFor work correctly.

	// Make three machines, three applications and three units for each application;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	m3, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0 := s.AddTestingApplication(c, "s0", dummy)
	s1 := s.AddTestingApplication(c, "s1", dummy)
	s2 := s.AddTestingApplication(c, "s2", dummy)
	s3 := s.AddTestingApplication(c, "s3", logging)

	units := make([][]*state.Unit, 4)
	for i, app := range []*state.Application{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = app.AddUnit(state.AddUnitParams{})
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	// Principals must be assigned to a machine before then
	// enter scope to create subordinates.
	assignments := []struct {
		machine      *state.Machine
		units        []*state.Unit
		subordinates []*state.Unit
	}{
		{m1, []*state.Unit{units[0][0]}, nil},
		{m2, []*state.Unit{units[0][1], units[1][0], units[1][1], units[2][0]}, nil},
		{m3, []*state.Unit{units[2][2]}, nil},
	}

	for _, a := range assignments {
		for _, u := range a.units {
			err := u.AssignToMachine(a.machine)
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	// Add the logging units subordinate to the s2 units.
	eps, err := s.State.InferEndpoints("s2", "s3")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	for _, u := range units[2] {
		ru, err := rel.Unit(u)
		c.Assert(err, tc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, tc.ErrorIsNil)
	}
	units[3], err = s3.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sortedUnitNames(units[3]), tc.DeepEquals, []string{"s3/0", "s3/1", "s3/2"})
	assignments[1].subordinates = []*state.Unit{units[3][0]}
	assignments[2].subordinates = []*state.Unit{units[3][2]}

	for i, a := range assignments {
		c.Logf("test %d", i)
		expect := sortedUnitNames(append(a.units, a.subordinates...))

		// The units can be retrieved from the machine model.
		got, err := a.machine.Units()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(sortedUnitNames(got), tc.DeepEquals, expect)

		// The units can be retrieved from the machine id.
		got, err = s.State.UnitsFor(a.machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(sortedUnitNames(got), tc.DeepEquals, expect)
	}
}

func sortedUnitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	sort.Strings(names)
	return names
}

func (s *MachineSuite) assertMachineDirtyAfterAddingUnit(c *tc.C) (*state.Machine, *state.Application, *state.Unit) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Clean(), tc.IsTrue)

	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Clean(), tc.IsFalse)
	return m, app, unit
}

func (s *MachineSuite) TestMachineDirtyAfterAddingUnit(c *tc.C) {
	s.assertMachineDirtyAfterAddingUnit(c)
}

func (s *MachineSuite) TestMachineDirtyAfterUnassigningUnit(c *tc.C) {
	m, _, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Clean(), tc.IsFalse)
}

func (s *MachineSuite) TestMachineDirtyAfterRemovingUnit(c *tc.C) {
	m, app, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Clean(), tc.IsFalse)
}

func (s *MachineSuite) TestWatchMachine(c *tc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.machine.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("m-foo"), "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = machine.SetAgentVersion(version.MustParseBinary("0.0.3-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove machine, start new watch, check single event.
	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = s.machine.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *MachineSuite) TestWatchDiesOnStateClose(c *tc.C) {
	// This test is testing logic in watcher.entityWatcher, which
	// is also used by:
	//  Machine.WatchInstanceData
	//  Application.Watch
	//  Unit.Watch
	//  State.WatchForModelConfigChanges
	//  Unit.WatchConfigSettings
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		w := m.Watch()
		select {
		case <-w.Changes():
		case <-time.After(coretesting.LongWait):
			c.Errorf("timeout waiting for Changes() to trigger")
		}
		return w
	})
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *tc.C) {
	// TODO(mjs) - MODELUUID - test with multiple models with
	// identically named units and ensure there's no leakage.

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine, and create a unit independently; no change.
	err := s.machine.SetProvisioned("cheese", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign that unit (to a separate machine instance); change detected.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = mysql0.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	c.Logf("assigning unit and destroying other")
	mysql1, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; no change.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, tc.ErrorIsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Change the subordinate; no change.
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = logging0.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// Start a fresh watcher; check both principals reported.
	w = s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Unassign the unit; check change.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/1")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchPrincipalUnitsDiesOnStateClose(c *tc.C) {
	// This test is testing logic in watcher.unitsWatcher, which
	// is also used by Unit.WatchSubordinateUnits.
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		w := m.WatchPrincipalUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchUnits(c *tc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine; no change.
	err := s.machine.SetProvisioned("cheese", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign a unit (to a separate instance); change detected.
	c.Logf("assigning mysql to machine %s", s.machine.Id())
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	c.Logf("changing unit mysql/0")
	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = mysql0.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	c.Logf("assigning mysql/1, destroying mysql/0")
	mysql1, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; change detected.
	c.Logf("adding subordinate logging/0")
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, tc.ErrorIsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Change the subordinate; no change.
	c.Logf("changing subordinate")
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = logging0.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	c.Logf("ensuring mysql/0 is Dead")
	err = mysql0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check all units reported.
	c.Logf("starting new watcher")
	w = s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql/0", "mysql/1", "logging/0")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	c.Logf("removing Dead unit mysql/0")
	err = mysql0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; change detected.
	c.Logf("destroying subordinate logging/0")
	err = logging0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Unassign the principal; check subordinate departure also reported.
	c.Logf("unassigning mysql/1")
	err = mysql1.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/1", "logging/0")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchUnitsHandlesDeletedEntries(c *tc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine; no change.
	err := s.machine.SetProvisioned("cheese", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign a unit (to a separate instance); change detected.
	c.Logf("assigning mysql to machine %s", s.machine.Id())
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Destroy the instance before checking the change
	err = mysql0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestApplicationNames(c *tc.C) {
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	mysql1, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	worpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	err = worpress0.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	apps, err := machine.ApplicationNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, []string{"mysql", "wordpress"})
}

func (s *MachineSuite) TestWatchUnitsDiesOnStateClose(c *tc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		w := m.WatchUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchMachineStartTimes(c *tc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	quiesceInterval := 10 * time.Second
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.State.WatchModelMachineStartTimes(quiesceInterval)

	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)

	// Get initial set of changes
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	s.Clock.Advance(quiesceInterval)
	wc.AssertChange("0", "1")
	wc.AssertNoChange()

	// Update the agent start time for the new machine and wait for quiesceInterval
	// so the change gets processed and added to a new changeset.
	err = s.machine.RecordAgentStartInformation("machine-1")
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	s.Clock.Advance(quiesceInterval)

	// Update the agent start time for machine 0 and wait for quiesceInterval
	// so the change gets processed and appended to the current changeset.
	err = s.machine0.RecordAgentStartInformation("machine-0")
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	s.Clock.Advance(quiesceInterval)

	// Fetch the pending changes
	wc.AssertChange("1", "0")
	wc.AssertNoChange()

	// Kill the machine, remove it from state and check ensure that we
	// still get back a change event.
	err = s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	s.Clock.Advance(quiesceInterval)
	wc.AssertChange("1")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestConstraintsFromModel(c *tc.C) {
	econs1 := constraints.MustParse("mem=1G")
	econs2 := constraints.MustParse("mem=2G")

	// A newly-created machine gets a copy of the model constraints.
	err := s.State.SetModelConstraints(econs1)
	c.Assert(err, tc.ErrorIsNil)
	machine1, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons1, tc.DeepEquals, econs1)

	// Change model constraints and add a new machine.
	err = s.State.SetModelConstraints(econs2)
	c.Assert(err, tc.ErrorIsNil)
	machine2, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	mcons2, err := machine2.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons2, tc.DeepEquals, econs2)

	// Check the original machine has its original constraints.
	mcons1, err = machine1.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons1, tc.DeepEquals, econs1)
}

func (s *MachineSuite) TestSetConstraints(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// Constraints can be set...
	cons1 := constraints.MustParse("mem=1G")
	err = machine.SetConstraints(cons1)
	c.Assert(err, tc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, cons1)

	// ...until the machine is provisioned, at which point they stick.
	err = machine.SetProvisioned("i-mstuck", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	cons2 := constraints.MustParse("mem=2G")
	err = machine.SetConstraints(cons2)
	c.Assert(err, tc.ErrorMatches, `updating machine "2": cannot set constraints: machine is already provisioned`)

	// Check the failed set had no effect.
	mcons, err = machine.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, cons1)

	all, err := s.Model.AllConstraints()
	c.Assert(err, tc.ErrorIsNil)
	cons := all.Machine(machine.Id())
	c.Assert(cons, tc.DeepEquals, cons1)
}

func (s *MachineSuite) TestSetAmbiguousConstraints(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err = machine.SetConstraints(cons)
	c.Assert(err, tc.ErrorMatches, `updating machine "2": cannot set constraints: ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *MachineSuite) TestSetUnsupportedConstraintsWarning(c *tc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw), tc.IsNil)

	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("mem=4G cpu-power=10")
	err = machine.SetConstraints(cons)
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{{
		Level:   loggo.WARNING,
		Message: `setting constraints on machine "2": unsupported constraints: cpu-power`,
	}})
	mcons, err := machine.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, cons)
}

func (s *MachineSuite) TestConstraintsLifecycle(c *tc.C) {
	cons := constraints.MustParse("mem=1G")
	cannotSet := `updating machine "1": cannot set constraints: machine is not found or not alive`
	testWhenDying(c, s.machine, cannotSet, cannotSet, func() error {
		err := s.machine.SetConstraints(cons)
		mcons, err1 := s.machine.Constraints()
		c.Assert(err1, tc.IsNil)
		c.Assert(&mcons, tc.Satisfies, constraints.IsEmpty)
		return err
	})

	err := s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.SetConstraints(cons)
	c.Assert(err, tc.ErrorMatches, cannotSet)
	_, err = s.machine.Constraints()
	c.Assert(err, tc.ErrorMatches, `constraints not found`)
}

func (s *MachineSuite) TestSetProviderAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	addresses := network.SpaceAddresses{
		network.NewSpaceAddress("127.0.0.1"),
		{
			MachineAddress: network.MachineAddress{
				Value: "8.8.8.8",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			SpaceID: "1",
		},
	}
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	sort.Sort(addresses)
	c.Assert(machine.Addresses(), tc.DeepEquals, addresses)
}

func (s *MachineSuite) TestSetProviderAddressesWithContainers(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	// When setting all addresses the subnet addresses have to be
	// filtered out.
	addresses := network.NewSpaceAddresses(
		"127.0.0.1",
		"8.8.8.8",
	)
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(machine.Addresses(), tc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetProviderAddressesOnContainer(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	// Create an LXC container inside the machine.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)

	// When setting all addresses the subnet address has to accepted.
	addresses := network.NewSpaceAddresses("127.0.0.1")
	err = container.SetProviderAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = container.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("127.0.0.1")
	c.Assert(container.Addresses(), tc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetMachineAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	addresses := network.NewSpaceAddresses("127.0.0.1", "8.8.8.8")
	err = machine.SetMachineAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(machine.MachineAddresses(), tc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetEmptyMachineAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	// Add some machine addresses initially to make sure they're removed.
	addresses := network.NewSpaceAddresses("127.0.0.1", "8.8.8.8")
	err = machine.SetMachineAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.MachineAddresses(), tc.HasLen, 2)

	// Make call with empty address list.
	err = machine.SetMachineAddresses()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machine.MachineAddresses(), tc.HasLen, 0)
}

func (s *MachineSuite) TestMergedAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	providerAddresses := network.NewSpaceAddresses(
		"127.0.0.2",
		"8.8.8.8",
		"fc00::1",
		"::1",
		"",
		"2001:db8::1",
		"127.0.0.2",
		"example.org",
	)
	err = machine.SetProviderAddresses(providerAddresses...)
	c.Assert(err, tc.ErrorIsNil)

	machineAddresses := network.NewSpaceAddresses(
		"127.0.0.1",
		"localhost",
		"2001:db8::1",
		"192.168.0.1",
		"fe80::1",
		"::1",
		"fd00::1",
	)
	err = machine.SetMachineAddresses(machineAddresses...)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	// Before setting the addresses coming from either the provider or
	// the machine itself, they are sorted to prefer public IPs on
	// top, then hostnames, cloud-local, machine-local, link-local.
	// Duplicates are removed, then when calling Addresses() both
	// sources are merged while preservig the provider addresses
	// order.
	c.Assert(machine.Addresses(), tc.DeepEquals, network.NewSpaceAddresses(
		"8.8.8.8",
		"2001:db8::1",
		"example.org",
		"fc00::1",
		"127.0.0.2",
		"::1",
		"localhost",
		"192.168.0.1",
		"fd00::1",
		"127.0.0.1",
		"fe80::1",
	))
}

func (s *MachineSuite) TestSetProviderAddressesConcurrentChangeDifferent(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	addr0 := network.NewSpaceAddress("127.0.0.1")
	addr1 := network.NewSpaceAddress("8.8.8.8")

	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		err = machine.SetProviderAddresses(addr1, addr0)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err = machine.SetProviderAddresses(addr0, addr1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.SameContents, network.SpaceAddresses{addr0, addr1})
}

func (s *MachineSuite) TestSetProviderAddressesConcurrentChangeEqual(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)
	machineDocID := state.DocID(s.State, machine.Id())
	revno0, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, tc.ErrorIsNil)

	addr0 := network.NewSpaceAddress("127.0.0.1")
	addr1 := network.NewSpaceAddress("8.8.8.8")

	var revno1 int64
	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, tc.ErrorIsNil)
		err = machine.SetProviderAddresses(addr0, addr1)
		c.Assert(err, tc.ErrorIsNil)
		revno1, err = state.TxnRevno(s.State, "machines", machineDocID)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(revno1, tc.GreaterThan, revno0)
	}).Check()

	err = machine.SetProviderAddresses(addr0, addr1)
	c.Assert(err, tc.ErrorIsNil)

	// Doc will be updated; concurrent changes are explicitly ignored.
	revno2, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno2, tc.GreaterThan, revno1)
	c.Assert(machine.Addresses(), tc.SameContents, network.SpaceAddresses{addr0, addr1})
}

func (s *MachineSuite) TestSetProviderAddressesInvalidateMemory(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)
	machineDocID := state.DocID(s.State, machine.Id())

	addr0 := network.NewSpaceAddress("127.0.0.1")
	addr1 := network.NewSpaceAddress("8.8.8.8")

	// Set addresses to [addr0] initially. We'll get a separate Machine
	// object to update addresses, to ensure that the in-memory cache of
	// addresses does not prevent the initial Machine from updating
	// addresses back to the original value.
	err = machine.SetProviderAddresses(addr0)
	c.Assert(err, tc.ErrorIsNil)
	revno0, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, tc.ErrorIsNil)

	machine2, err := s.State.Machine(machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = machine2.SetProviderAddresses(addr1)
	c.Assert(err, tc.ErrorIsNil)
	revno1, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno1, tc.GreaterThan, revno0)
	c.Assert(machine.Addresses(), tc.SameContents, network.SpaceAddresses{addr0})
	c.Assert(machine2.Addresses(), tc.SameContents, network.SpaceAddresses{addr1})

	err = machine.SetProviderAddresses(addr0)
	c.Assert(err, tc.ErrorIsNil)
	revno2, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno2, tc.GreaterThan, revno1)
	c.Assert(machine.Addresses(), tc.SameContents, network.SpaceAddresses{addr0})
}

func (s *MachineSuite) TestPublicAddressSetOnNewMachine(c *tc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:      state.UbuntuBase("12.10"),
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Addresses: network.NewSpaceAddresses("10.0.0.1", "8.8.8.8"),
	})
	c.Assert(err, tc.ErrorIsNil)
	addr, err := m.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, network.NewSpaceAddress("8.8.8.8"))
}

func (s *MachineSuite) TestPrivateAddressSetOnNewMachine(c *tc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:      state.UbuntuBase("12.10"),
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Addresses: network.NewSpaceAddresses("10.0.0.1", "8.8.8.8"),
	})
	c.Assert(err, tc.ErrorIsNil)
	addr, err := m.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, network.NewSpaceAddress("10.0.0.1"))
}

func (s *MachineSuite) TestPublicAddressEmptyAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
	c.Assert(addr.Value, tc.Equals, "")
}

func (s *MachineSuite) TestPrivateAddressEmptyAddresses(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Addresses(), tc.HasLen, 0)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.Satisfies, network.IsNoAddressError)
	c.Assert(addr.Value, tc.Equals, "")
}

func (s *MachineSuite) TestPublicAddress(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestPrivateAddress(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressBetterMatch(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestPrivateAddressBetterMatch(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"), network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressChanges(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.4.4"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestPrivateAddressChanges(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestAddressesDeadMachine(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewSpaceAddress("10.0.0.2"), network.NewSpaceAddress("8.8.4.4"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// A dead machine should still report the last known addresses.
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestStablePrivateAddress(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.1"), network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")
}

func (s *MachineSuite) TestStablePublicAddress(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.4.4"), network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, tc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestAddressesRaceMachineFirst(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	changeAddresses := jujutxn.TestHook{
		Before: func() {
			err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.8.8"))
			c.Assert(err, tc.ErrorIsNil)
			address, err := machine.PublicAddress()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.8.8"))
			address, err = machine.PrivateAddress()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.8.8"))
		},
	}
	defer state.SetTestHooks(c, s.State, changeAddresses).Check()

	err = machine.SetMachineAddresses(network.NewSpaceAddress("8.8.4.4"))
	c.Assert(err, tc.ErrorIsNil)

	machine, err = s.State.Machine(machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	address, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.8.8"))
	address, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.8.8"))
}

func (s *MachineSuite) TestAddressesRaceProviderFirst(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	changeAddresses := jujutxn.TestHook{
		Before: func() {
			err = machine.SetMachineAddresses(network.NewSpaceAddress("10.0.0.1"))
			c.Assert(err, tc.ErrorIsNil)
			address, err := machine.PublicAddress()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("10.0.0.1"))
			address, err = machine.PrivateAddress()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("10.0.0.1"))
		},
	}
	defer state.SetTestHooks(c, s.State, changeAddresses).Check()

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.4.4"))
	c.Assert(err, tc.ErrorIsNil)

	machine, err = s.State.Machine(machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	address, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.4.4"))
	address, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.DeepEquals, network.NewSpaceAddress("8.8.4.4"))
}

func (s *MachineSuite) TestPrivateAddressPrefersProvider(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("8.8.8.8"), network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressPrefersProvider(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("8.8.8.8"), network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.4.4"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestAddressesPrefersProviderBoth(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewSpaceAddress("8.8.8.8"), network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.1")

	err = machine.SetProviderAddresses(network.NewSpaceAddress("8.8.4.4"), network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, tc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "8.8.4.4")
	addr, err = machine.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr.Value, tc.Equals, "10.0.0.2")
}

func (s *MachineSuite) addMachineWithSupportedContainer(c *tc.C, container instance.ContainerType) *state.Machine {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	containers := []instance.ContainerType{container}
	err = machine.SetSupportedContainers(containers)
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, containers)
	return machine
}

// assertSupportedContainers checks the document in memory has the specified
// containers and then reloads the document from the database to assert saved
// values match also.
func assertSupportedContainers(c *tc.C, machine *state.Machine, containers []instance.ContainerType) {
	supportedContainers, known := machine.SupportedContainers()
	c.Assert(known, tc.IsTrue)
	c.Assert(supportedContainers, tc.DeepEquals, containers)
	// Reload so we can check the saved values.
	err := machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	supportedContainers, known = machine.SupportedContainers()
	c.Assert(known, tc.IsTrue)
	c.Assert(supportedContainers, tc.DeepEquals, containers)
}

func assertSupportedContainersUnknown(c *tc.C, machine *state.Machine) {
	containers, known := machine.SupportedContainers()
	c.Assert(known, tc.IsFalse)
	c.Assert(containers, tc.HasLen, 0)
}

func (s *MachineSuite) TestSupportedContainersInitiallyUnknown(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainers(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SupportsNoContainers()
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainerTypeNoneIsError(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.NONE})
	c.Assert(err, tc.ErrorMatches, `"none" is not a valid container type`)
	assertSupportedContainersUnknown(c, machine)
	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainersOverwritesExisting(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SupportsNoContainers()
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
	// Calling it a second time should not invoke a db transaction
	defer state.SetFailIfTransaction(c, s.State).Check()
	err = machine.SupportsNoContainers()
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainersSingle(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD})
}

func (s *MachineSuite) TestSetSupportedContainersSame(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	defer state.SetFailIfTransaction(c, s.State).Check()
	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD})
}

func (s *MachineSuite) TestSetSupportedContainersNew(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipeNew(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExisting(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
	// Setting it again will be a no-op
	defer state.SetFailIfTransaction(c, s.State).Check()
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExistingInvertedOrder(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.KVM, instance.LXD})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.KVM, instance.LXD})
	// Setting it again will be a no-op
	defer state.SetFailIfTransaction(c, s.State).Check()
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.KVM, instance.LXD})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.KVM, instance.LXD})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExistingWithDifferentInstanceType(c *tc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, tc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
	// Setting it again will be a no-op
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.ContainerType("FOO")})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestSetSupportedContainersSetsUnknownToError(c *tc.C) {
	// Create a machine and add lxd and kvm containers prior to calling SetSupportedContainers
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	supportedContainer, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, tc.ErrorIsNil)

	// A supported (kvm) container will have a pending status.
	err = supportedContainer.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err := supportedContainer.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Pending)

	// An unsupported (lxd) container will have an error status.
	err = container.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err = container.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Error)
	c.Assert(statusInfo.Message, tc.Equals, "unsupported container")
	c.Assert(statusInfo.Data, tc.DeepEquals, map[string]interface{}{"type": "lxd"})
}

func (s *MachineSuite) TestSupportsNoContainersSetsAllToError(c *tc.C) {
	// Create a machine and add all container types prior to calling SupportsNoContainers
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	var containers []*state.Machine
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	for _, containerType := range instance.ContainerTypes {
		container, err := s.State.AddMachineInsideMachine(template, machine.Id(), containerType)
		c.Assert(err, tc.ErrorIsNil)
		containers = append(containers, container)
	}

	err = machine.SupportsNoContainers()
	c.Assert(err, tc.ErrorIsNil)

	// All containers should be in error state.
	for _, container := range containers {
		err = container.Refresh()
		c.Assert(err, tc.ErrorIsNil)
		statusInfo, err := container.Status()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(statusInfo.Status, tc.Equals, status.Error)
		c.Assert(statusInfo.Message, tc.Equals, "unsupported container")
		containerType := corecontainer.ContainerTypeFromId(container.Id())
		c.Assert(statusInfo.Data, tc.DeepEquals, map[string]interface{}{"type": string(containerType)})
	}
}

func (s *MachineSuite) TestMachineAgentTools(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	testAgentTools(c, m, "machine "+m.Id())
}

func (s *MachineSuite) TestMachineValidActions(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	var tests = []struct {
		actionName      string
		errString       string
		givenPayload    map[string]interface{}
		expectedPayload map[string]interface{}
	}{
		{
			actionName: "juju-exec",
			errString:  `validation failed: (root) : "command" property is missing and required, given {}; (root) : "timeout" property is missing and required, given {}`,
		},
		{
			actionName:      "juju-exec",
			givenPayload:    map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
			expectedPayload: map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
		},
		{
			actionName: "baiku",
			errString:  `cannot add action "baiku" to a machine; only predefined actions allowed`,
		},
	}

	for i, t := range tests {
		c.Logf("running test %d", i)
		operationID, err := s.Model.EnqueueOperation("a test", 1)
		c.Assert(err, tc.ErrorIsNil)
		action, err := s.Model.AddAction(m, operationID, t.actionName, t.givenPayload, nil, nil)
		if t.errString != "" {
			c.Assert(err.Error(), tc.Equals, t.errString)
			continue
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(action.Parameters(), tc.DeepEquals, t.expectedPayload)
		}
	}
}

func (s *MachineSuite) TestAddActionWithError(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.Model.AddAction(m, operationID, "benchmark", nil, nil, nil)
	c.Assert(err, tc.ErrorMatches, `cannot add action "benchmark" to a machine; only predefined actions allowed`)
	op, err := s.Model.Operation(operationID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.Status(), tc.Equals, state.ActionError)
}

func (s *MachineSuite) setupTestUpdateMachineSeries(c *tc.C) *state.Machine {
	mach, err := s.State.AddMachine(state.UbuntuBase("12.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "multi-series", ch)
	subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
	_ = state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "multi-series-subordinate", subCh)

	eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(mach)
	c.Assert(err, tc.ErrorIsNil)

	ru, err := rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	return mach
}

func (s *MachineSuite) assertMachineAndUnitSeriesChanged(c *tc.C, mach *state.Machine, base state.Base) {
	err := mach.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mach.Base().String(), tc.Equals, base.String())
	principals := mach.Principals()
	for _, p := range principals {
		u, err := s.State.Unit(p)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(u.Base().String(), tc.DeepEquals, base.String())
		subs := u.SubordinateNames()
		for _, sn := range subs {
			u, err := s.State.Unit(sn)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(u.Base().String(), tc.DeepEquals, base.String())
		}
	}
}

func (s *MachineSuite) TestUpdateMachineSeries(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries(state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, state.UbuntuBase("22.04"))
}

func (s *MachineSuite) TestUpdateMachineSeriesSameSeriesToStart(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries(state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, state.UbuntuBase("22.04"))
}

func (s *MachineSuite) TestUpdateMachineSeriesSameSeriesAfterStart(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				ops := []txn.Op{{
					C:      state.MachinesC,
					Id:     state.DocID(s.State, mach.Id()),
					Update: bson.D{{"$set", bson.D{{"series", state.UbuntuBase("22.04")}}}},
				}}
				state.RunTransaction(c, s.State, ops)
			},
			After: func() {
				err := mach.Refresh()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(mach.Base().String(), tc.Equals, "ubuntu@22.04/stable")
			},
		},
	).Check()

	err := mach.UpdateMachineSeries(state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)
	err = mach.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mach.Base().DisplayString(), tc.Equals, "ubuntu@22.04")
}

func (s *MachineSuite) TestUpdateMachineSeriesPrincipalsListChange(c *tc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(mach.Principals()), tc.Equals, 1)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				app, err := s.State.Application("multi-series")
				c.Assert(err, tc.ErrorIsNil)
				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, tc.ErrorIsNil)
				err = unit.AssignToMachine(mach)
				c.Assert(err, tc.ErrorIsNil)
			},
		},
	).Check()

	err = mach.UpdateMachineSeries(state.UbuntuBase("22.04"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, state.UbuntuBase("22.04"))
	c.Assert(len(mach.Principals()), tc.Equals, 2)
}

func (s *MachineSuite) addMachineUnit(c *tc.C, mach *state.Machine) *state.Unit {
	units, err := mach.Units()
	c.Assert(err, tc.ErrorIsNil)

	var app *state.Application
	if len(units) == 0 {
		ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
		app = state.AddTestingApplicationForBase(c, s.State, mach.Base(), "multi-series", ch)
		subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
		_ = state.AddTestingApplicationForBase(c, s.State, mach.Base(), "multi-series-subordinate", subCh)
	} else {
		app, err = units[0].Application()
		c.Assert(err, tc.ErrorIsNil)
	}

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(mach)
	c.Assert(err, tc.ErrorIsNil)
	return unit
}

func (s *MachineSuite) TestWatchAddresses(c *tc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := machine.WatchAddresses()
	defer w.Stop()
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned("i-blah", "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Set machine addresses: reported.
	err = machine.SetMachineAddresses(network.NewSpaceAddress("abc"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Set provider addresses eclipsing machine addresses: reported.
	err = machine.SetProviderAddresses(network.NewSpaceAddress("abc", network.WithScope(network.ScopePublic)))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Set same machine eclipsed by provider addresses: not reported.
	err = machine.SetMachineAddresses(network.NewSpaceAddress("abc", network.WithScope(network.ScopeCloudLocal)))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Set different machine addresses: reported.
	err = machine.SetMachineAddresses(network.NewSpaceAddress("def"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Set different provider addresses: reported.
	err = machine.SetMachineAddresses(network.NewSpaceAddress("def", network.WithScope(network.ScopePublic)))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Make it Dying: not reported.
	err = machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dead: not reported.
	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove it: watcher eventually closed and Err
	// returns an IsNotFound error.
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher not closed")
	}
	c.Assert(w.Err(), tc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestGetManualMachineArches(c *tc.C) {
	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Jobs:                    []state.MachineJob{state.JobHostUnits},
		InstanceId:              "manual:foo",
		Nonce:                   "manual:foo-nonce",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Jobs:                    []state.MachineJob{state.JobHostUnits},
		InstanceId:              "manual:bar",
		Nonce:                   "manual:bar-nonce",
		HardwareCharacteristics: instance.MustParseHardware("arch=s390x"),
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Jobs:                    []state.MachineJob{state.JobHostUnits},
		InstanceId:              "lorem",
		Nonce:                   "lorem:nonce",
		HardwareCharacteristics: instance.MustParseHardware("arch=ppc64el"),
	})
	c.Assert(err, tc.ErrorIsNil)

	manualArchSet, err := s.State.GetManualMachineArches()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manualArchSet.SortedValues(), tc.DeepEquals, []string{"amd64", "s390x"})
}
