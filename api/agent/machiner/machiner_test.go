// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/machiner"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type machinerSuite struct {
	testing.JujuConnSuite
	*apitesting.APIAddresserTests

	st      api.Connection
	machine *state.Machine

	machiner *machiner.State
}

func TestMachinerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &machinerSuite{})
}

func (s *machinerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetProviderAddresses(network.NewSpaceAddress("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	s.st, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.machiner = machiner.NewState(s.st)
	c.Assert(s.machiner, tc.NotNil)
	waitForModelWatchersIdle := func(c *tc.C) {
		s.JujuConnSuite.WaitForModelWatchersIdle(c, s.BackingState.ModelUUID())
	}
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	s.APIAddresserTests = apitesting.NewAPIAddresserTests(s.machiner, systemState, s.BackingState, waitForModelWatchersIdle)
}

func (s *machinerSuite) TestMachineAndMachineTag(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("42"))
	c.Assert(err, tc.ErrorMatches, ".*permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(machine, tc.IsNil)

	machine1 := names.NewMachineTag("1")
	machine, err = s.machiner.Machine(machine1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Tag(), tc.Equals, machine1)
}

func (s *machinerSuite) TestSetStatus(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := s.machine.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Pending)
	c.Assert(statusInfo.Message, tc.Equals, "")
	c.Assert(statusInfo.Data, tc.HasLen, 0)

	err = machine.SetStatus(status.Started, "blah", nil)
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err = s.machine.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Started)
	c.Assert(statusInfo.Message, tc.Equals, "blah")
	c.Assert(statusInfo.Data, tc.HasLen, 0)
	c.Assert(statusInfo.Since, tc.NotNil)
}

func (s *machinerSuite) TestEnsureDead(c *tc.C) {
	c.Assert(s.machine.Life(), tc.Equals, state.Alive)

	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.Life(), tc.Equals, state.Dead)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.Life(), tc.Equals, state.Dead)

	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorMatches, "machine 1 not found")
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
}

func (s *machinerSuite) TestRefresh(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Life(), tc.Equals, life.Alive)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Life(), tc.Equals, life.Alive)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Life(), tc.Equals, life.Dead)
}

func (s *machinerSuite) TestSetMachineAddresses(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)

	addr := s.machine.Addresses()
	c.Assert(addr, tc.HasLen, 0)

	setAddresses := []network.MachineAddress{
		network.NewMachineAddress("8.8.8.8"),
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("10.0.0.1"),
	}
	// Before setting, the addresses are sorted to put public on top,
	// cloud-local next, machine-local last.
	expectAddresses := network.NewSpaceAddresses(
		"8.8.8.8",
		"10.0.0.1",
		"127.0.0.1",
	)
	err = machine.SetMachineAddresses(setAddresses)
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.MachineAddresses(), tc.DeepEquals, expectAddresses)
}

func (s *machinerSuite) TestSetEmptyMachineAddresses(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)

	setAddresses := []network.MachineAddress{
		network.NewMachineAddress("8.8.8.8"),
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("10.0.0.1"),
	}
	err = machine.SetMachineAddresses(setAddresses)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.MachineAddresses(), tc.HasLen, 3)

	err = machine.SetMachineAddresses(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.MachineAddresses(), tc.HasLen, 0)
}

func (s *machinerSuite) TestWatch(c *tc.C) {
	machine, err := s.machiner.Machine(names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Life(), tc.Equals, life.Alive)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	w, err := machine.Watch()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = machine.SetStatus(status.Started, "not really", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the machine dead and check it's detected.
	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *tc.C) {
	mTag := names.NewMachineTag("1")
	stMachine, err := s.State.Machine(mTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	oldStartedAt := stMachine.AgentStartTime()

	machine, err := s.machiner.Machine(mTag)
	c.Assert(err, tc.ErrorIsNil)

	err = machine.RecordAgentStartInformation("thundering-herds")
	c.Assert(err, tc.ErrorIsNil)

	err = stMachine.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(stMachine.AgentStartTime(), tc.Not(tc.Equals), oldStartedAt, tc.Commentf("expected the agent start time to be updated"))
	c.Assert(stMachine.Hostname(), tc.Equals, "thundering-herds", tc.Commentf("expected for the recorded machine hostname to be updated"))
}
