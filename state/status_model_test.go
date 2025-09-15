// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type ModelStatusSuite struct {
	ConnSuite
	st      *state.State
	model   *state.Model
	factory *factory.Factory
}

func TestModelStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelStatusSuite{})
}

func (s *ModelStatusSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.st = s.Factory.MakeModel(c, nil)
	m, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.model = m
	s.factory = factory.NewFactory(s.st, s.StatePool)
}

func (s *ModelStatusSuite) TearDownTest(c *tc.C) {
	if s.st != nil {
		err := s.st.Close()
		c.Assert(err, tc.ErrorIsNil)
		s.st = nil
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *ModelStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.model.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Available)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *ModelStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Assert(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDying(c *tc.C) {
	// Add a machine to the model to ensure it is non-empty
	// when we destroy; this prevents the model from advancing
	// directly to Dead.
	s.factory.MakeMachine(c, nil)

	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDead(c *tc.C) {
	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusGone(c *tc.C) {
	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "not really",
		Since:   &now,
	}
	err = s.model.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: model not found`)

	_, err = s.model.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: model not found`)
}

func (s *ModelStatusSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			}},
		Since: &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	// Get another instance of the Model to compare against
	model, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := model.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Available)
	c.Check(statusInfo.Message, tc.Equals, "blah")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *ModelStatusSuite) TestModelStatusForModel(c *tc.C) {
	ms, err := s.model.LoadModelStatus()
	c.Assert(err, tc.ErrorIsNil)

	info, err := ms.Model()
	c.Assert(err, tc.ErrorIsNil)

	mInfo, err := s.model.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, mInfo)
}

func (s *ModelStatusSuite) TestMachineStatus(c *tc.C) {
	machine := s.factory.MakeMachine(c, nil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, tc.ErrorIsNil)

	msAgent, err := ms.MachineAgent(machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	msInstance, err := ms.MachineInstance(machine.Id())
	c.Assert(err, tc.ErrorIsNil)

	mAgent, err := machine.Status()
	c.Assert(err, tc.ErrorIsNil)
	mInstance, err := machine.InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(msAgent, tc.DeepEquals, mAgent)
	c.Assert(msInstance, tc.DeepEquals, mInstance)
}

func (s *ModelStatusSuite) TestUnitStatus(c *tc.C) {
	unit := s.factory.MakeUnit(c, nil)

	c.Assert(unit.SetWorkloadVersion("42.1"), tc.ErrorIsNil)
	c.Assert(unit.SetStatus(status.StatusInfo{Status: status.Active}), tc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), tc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, tc.ErrorIsNil)

	msAgent, err := ms.UnitAgent(unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unit.Name(), true)
	c.Assert(err, tc.ErrorIsNil)
	msWorkloadVersion, err := ms.UnitWorkloadVersion(unit.Name())
	c.Assert(err, tc.ErrorIsNil)

	uAgent, err := unit.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)
	uWorkload, err := unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	uWorkloadVersion, err := unit.WorkloadVersion()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(msAgent, tc.DeepEquals, uAgent)
	c.Check(msWorkload, tc.DeepEquals, uWorkload)
	c.Check(msWorkloadVersion, tc.DeepEquals, uWorkloadVersion)
}

func (s *ModelStatusSuite) TestUnitStatusWeirdness(c *tc.C) {
	unit := s.factory.MakeUnit(c, nil)

	// When the agent status is in error, we show the workload status
	// as an error, and the agent as idle
	c.Assert(unit.SetStatus(status.StatusInfo{Status: status.Active}), tc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "OMG"}), tc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, tc.ErrorIsNil)

	msAgent, err := ms.UnitAgent(unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unit.Name(), true)
	c.Assert(err, tc.ErrorIsNil)

	uAgent, err := unit.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)
	uWorkload, err := unit.Status()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(msAgent, tc.DeepEquals, uAgent)
	c.Check(msWorkload, tc.DeepEquals, uWorkload)

	c.Check(msAgent.Status, tc.Equals, status.Idle)
	c.Check(msWorkload.Status, tc.Equals, status.Error)
}
