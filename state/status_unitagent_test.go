// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"
	"time" // Only used for time types.

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type StatusUnitAgentSuite struct {
	ConnSuite
	unit  *state.Unit
	agent *state.UnitAgent
}

func TestStatusUnitAgentSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &StatusUnitAgentSuite{})
}

func (s *StatusUnitAgentSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)
	s.agent = s.unit.Agent()
}

func (s *StatusUnitAgentSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.agent.Status()
	c.Check(err, tc.ErrorIsNil)
	checkInitialUnitAgentStatus(c, statusInfo)
}

func (s *StatusUnitAgentSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetErrorStatusWithoutInfo(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetAllocatingStatusAlreadyAssigned(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Allocating,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "allocating" as unit is already assigned`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetStatusUnassigned(c *tc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "foo"})
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	agent := u.Agent()
	for _, value := range []status.Status{status.Idle, status.Executing, status.Rebooting, status.Failed} {
		now := testing.ZeroTime()
		sInfo := status.StatusInfo{
			Status:  value,
			Message: "",
			Since:   &now,
		}
		err := agent.SetStatus(sInfo)
		c.Check(err, tc.ErrorMatches, fmt.Sprintf(`cannot set status %q until unit is assigned`, value))

		s.checkInitialStatus(c)
	}
}

func (s *StatusUnitAgentSuite) TestSetStatusRunningNonCAAS(c *tc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "foo"})
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	agent := u.Agent()
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "",
		Since:   &now,
	}
	err = agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set invalid status "running"`)
	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "something",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusAlive(c *tc.C) {
	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "something",
		Data: map[string]interface{}{
			"$foo":    "bar",
			"baz.qux": "ping",
			"pong": map[string]interface{}{
				"$unset": "txn-revno",
			},
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	agent := unit.Agent()

	statusInfo, err := agent.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Idle)
	c.Check(statusInfo.Message, tc.Equals, "something")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$foo":    "bar",
		"baz.qux": "ping",
		"pong": map[string]interface{}{
			"$unset": "txn-revno",
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusDying(c *tc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusDead(c *tc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusGone(c *tc.C) {
	err := s.unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "not really",
		Since:   &now,
	}
	err = s.agent.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: agent not found`)

	statusInfo, err := s.agent.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: agent not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *StatusUnitAgentSuite) TestGetSetErrorStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "test-hook failed",
		Data: map[string]interface{}{
			"foo": "bar",
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Agent error is reported as unit error.
	statusInfo, err := s.unit.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Error)
	c.Check(statusInfo.Message, tc.Equals, "test-hook failed")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})

	// For agents, error is reported as idle.
	statusInfo, err = s.agent.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Idle)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
}

func timeBeforeOrEqual(timeBefore, timeOther time.Time) bool {
	return timeBefore.Before(timeOther) || timeBefore.Equal(timeOther)
}

func (s *StatusUnitAgentSuite) TestSetAgentStatusSince(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err := s.agent.Status()
	c.Assert(err, tc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, tc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), tc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	now = now.Add(1 * time.Second)
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = s.agent.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err = s.agent.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), tc.IsTrue)
}

func (s *StatusUnitAgentSuite) TestStatusHistoryInitial(c *tc.C) {
	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 1})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 1)

	checkInitialUnitAgentStatus(c, history[0])
}

func (s *StatusUnitAgentSuite) TestStatusHistoryShort(c *tc.C) {
	primeUnitAgentStatusHistory(c, s.Clock, s.agent, 5, 0, "")

	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 6)

	checkInitialUnitAgentStatus(c, history[5])
	history = history[:5]
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 4-i, 0)
	}
}

func (s *StatusUnitAgentSuite) TestStatusHistoryLong(c *tc.C) {
	primeUnitAgentStatusHistory(c, s.Clock, s.agent, 25, 0, "")

	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 15})
	c.Check(err, tc.ErrorIsNil)
	c.Check(history, tc.HasLen, 15)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 24-i, 0)
	}
}
