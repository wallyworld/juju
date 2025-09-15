// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type MachineStatusSuite struct {
	testhelpers.IsolationSuite
	ctx     common.ModelPresenceContext
	machine *mockMachine
}

func TestMachineStatusSuite(t *tctesting.T) {
	tc.Run(t, &MachineStatusSuite{})
}

func (s *MachineStatusSuite) SetUpTest(c *tc.C) {
	s.machine = &mockMachine{
		id:     "666",
		status: status.Started,
	}
	s.ctx = common.ModelPresenceContext{
		Presence: agentAlive(names.NewMachineTag(s.machine.id).String()),
	}
}

func (s *MachineStatusSuite) checkUntouched(c *tc.C) {
	agent, err := s.ctx.MachineStatus(s.machine)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(agent.Status, tc.DeepEquals, s.machine.status)
}

func (s *MachineStatusSuite) TestNormal(c *tc.C) {
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestErrors(c *tc.C) {
	s.machine.statusErr = errors.New("status error")

	_, err := s.ctx.MachineStatus(s.machine)
	c.Assert(err, tc.ErrorMatches, "status error")
}

func (s *MachineStatusSuite) TestDown(c *tc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	agent, err := s.ctx.MachineStatus(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agent, tc.DeepEquals, status.StatusInfo{
		Status:  status.Down,
		Message: "agent is not communicating with the server",
	})
}

func (s *MachineStatusSuite) TestDownAndDead(c *tc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	s.machine.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestPresenceError(c *tc.C) {
	s.ctx.Presence = presenceError(names.NewMachineTag(s.machine.Id()).String())
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestNotDownIfPending(c *tc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	s.machine.status = status.Pending
	s.checkUntouched(c)
}
