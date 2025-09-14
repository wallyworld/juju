// Copyright 2015 Canonical Ltd.
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

type ApplicationStatusSuite struct {
	ConnSuite
	application *state.Application
}

func TestApplicationStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ApplicationStatusSuite{})
}

func (s *ApplicationStatusSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.application = s.Factory.MakeApplication(c, nil)
}

func (s *ApplicationStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *ApplicationStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.application.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Unset)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *ApplicationStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ApplicationStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) TestGetSetStatusAlive(c *tc.C) {
	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"$ping": map[string]interface{}{
				"foo.bar": 123,
			},
		},
		Since: &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	application, err := s.State.Application(s.application.Name())
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := application.Status()
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

func (s *ApplicationStatusSuite) TestGetSetStatusDying(c *tc.C) {
	_, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) TestGetSetStatusGone(c *tc.C) {
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: application not found`)

	statusInfo, err := s.application.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: application not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *ApplicationStatusSuite) TestSetStatusSince(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err := s.application.Status()
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
	err = s.application.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	statusInfo, err = s.application.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), tc.IsTrue)
}
