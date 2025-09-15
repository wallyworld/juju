// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"
	"time" // Only used for time types.

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type UnitStatusSuite struct {
	ConnSuite
	unit *state.Unit
}

func TestUnitStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &UnitStatusSuite{})
}

func (s *UnitStatusSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)
}

func (s *UnitStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *UnitStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.unit.Status()
	c.Check(err, tc.ErrorIsNil)
	checkInitialWorkloadStatus(c, statusInfo)
}

func (s *UnitStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *UnitStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) TestGetSetStatusAlive(c *tc.C) {
	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"$ping": map[string]interface{}{
				"foo.bar": 123,
			}},
		Since: &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := unit.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Active)
	c.Check(statusInfo.Message, tc.Equals, "healthy")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$ping": map[string]interface{}{
			"foo.bar": 123,
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *UnitStatusSuite) TestGetSetStatusDying(c *tc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) TestGetSetStatusDead(c *tc.C) {
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

func (s *UnitStatusSuite) TestGetSetStatusGone(c *tc.C) {
	err := s.unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.unit.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: unit not found`)

	statusInfo, err := s.unit.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: unit not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *UnitStatusSuite) TestSetUnitStatusSince(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err := s.unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, tc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), tc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	now = now.Add(1 * time.Second)
	sInfo = status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err = s.unit.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err = s.unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), tc.IsTrue)
}

func (s *UnitStatusSuite) TestStatusSinceDoesNotChangeWhenReceivedStatusIsTheSameAsCurrent(c *tc.C) {
	lastStatus, err := s.unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, tc.ErrorIsNil)
	originalHistoryLength := len(history)

	// Ensure new status change has a distinctly different update time.
	now := lastStatus.Since.Add(1 * time.Hour)
	changeTime := now
	msg := "within the loop"
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: msg,
		Since:   &now,
	}

	// Setting the same status consecutively with different timestamps,
	// should not update a status. It should, however, update
	// the history record with the timestamp of the last call made.
	for i := 0; i < 10; i++ {
		err = s.unit.SetStatus(sInfo)
		c.Assert(err, tc.ErrorIsNil)
		// Next status sent will be an hour from now.
		now = now.Add(1 * time.Hour)
		sInfo.Since = &now
	}

	statusInfo, err := s.unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	//Check that 'since' field reflects when change first happened.
	c.Assert(changeTime.Equal((*statusInfo.Since)), tc.IsTrue)

	historyAfter, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, tc.ErrorIsNil)
	// Only expecting one more status history addition.
	c.Assert(len(historyAfter), tc.Equals, originalHistoryLength+1)

	// The time of history record for this change should be updated
	// to the last time setstatus was called.
	expectedTimeInHistory := changeTime.Add(9 * time.Hour)
	for _, record := range historyAfter {
		if record.Message == msg {
			c.Assert(expectedTimeInHistory.Equal((*record.Since)), tc.IsTrue)
		}
	}
}

func (s *UnitStatusSuite) TestStatusHistoryInitial(c *tc.C) {
	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 1})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 1)

	checkInitialWorkloadStatus(c, history[0])
}

func (s *UnitStatusSuite) TestStatusHistoryShort(c *tc.C) {
	primeUnitStatusHistory(c, s.Clock, s.unit, 5, 0)

	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 6)

	checkInitialWorkloadStatus(c, history[5])
	history = history[:5]
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 4-i, 0)
	}
}

func (s *UnitStatusSuite) TestStatusHistoryLong(c *tc.C) {
	primeUnitStatusHistory(c, s.Clock, s.unit, 25, 0)

	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 15})
	c.Check(err, tc.ErrorIsNil)
	c.Check(history, tc.HasLen, 15)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 24-i, 0)
	}
}
