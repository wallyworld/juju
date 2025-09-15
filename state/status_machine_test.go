// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type MachineStatusSuite struct {
	ConnSuite
	machine *state.Machine
}

func TestMachineStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MachineStatusSuite{})
}

func (s *MachineStatusSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.machine = s.Factory.MakeMachine(c, nil)
}

func (s *MachineStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.machine.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Pending)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *MachineStatusSuite) TestSetErrorStatusWithoutInfo(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetDownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Down,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "down"`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Assert(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusAlive(c *tc.C) {
	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := machine.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Started)
	c.Check(statusInfo.Message, tc.Equals, "blah")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *MachineStatusSuite) TestGetSetStatusDying(c *tc.C) {
	err := s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusDead(c *tc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusGone(c *tc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "not really",
		Since:   &now,
	}
	err = s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: machine not found`)

	statusInfo, err := s.machine.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: machine not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *MachineStatusSuite) TestSetStatusPendingProvisioned(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "pending"`)
}

func (s *MachineStatusSuite) TestSetStatusPendingUnprovisioned(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = machine.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)
}
