// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type statusSetterSuite struct {
	statusBaseSuite
	setter *common.StatusSetter
}

func TestStatusSetterSuite(t *tctesting.T) {
	tc.Run(t, &statusSetterSuite{})
}

func (s *statusSetterSuite) SetUpTest(c *tc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.setter = common.NewStatusSetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *statusSetterSuite) TestUnauthorized(c *tc.C) {
	tag := names.NewMachineTag("42")
	s.badTag = tag
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusSetterSuite) TestNotATag(c *tc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *statusSetterSuite) TestNotFound(c *tc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    names.NewMachineTag("42").String(),
		Status: status.Down.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *statusSetterSuite) TestSetMachineStatus(c *tc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    machine.Tag().String(),
		Status: status.Started.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	err = machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	machineStatus, err := machine.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineStatus.Status, tc.Equals, status.Started)
}

func (s *statusSetterSuite) TestSetUnitStatus(c *tc.C) {
	// The status has to be a valid workload status, because get status
	// on the unit returns the workload status not the agent status as it
	// does on a machine.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	err = unit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	unitStatus, err := unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitStatus.Status, tc.Equals, status.Active)
}

func (s *statusSetterSuite) TestSetServiceStatus(c *tc.C) {
	// Calls to set the status of a service should be going through the
	// ServiceStatusSetter that checks for leadership, so permission denied
	// here.
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    service.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)

	err = service.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	serviceStatus, err := service.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serviceStatus.Status, tc.Equals, status.Maintenance)
}

func (s *statusSetterSuite) TestBulk(c *tc.C) {
	s.badTag = names.NewMachineTag("42")
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: status.Active.String(),
	}, {
		Tag:    "bad-tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type serviceStatusSetterSuite struct {
	statusBaseSuite
	setter *common.ApplicationStatusSetter
}

func TestServiceStatusSetterSuite(t *tctesting.T) {
	tc.Run(t, &serviceStatusSetterSuite{})
}

func (s *serviceStatusSetterSuite) SetUpTest(c *tc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.setter = common.NewApplicationStatusSetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	}, s.leadershipChecker)
}

func (s *serviceStatusSetterSuite) TestUnauthorized(c *tc.C) {
	// Machines are unauthorized since they are not units
	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestNotATag(c *tc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *serviceStatusSetterSuite) TestNotFound(c *tc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    names.NewUnitTag("foo/0").String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *serviceStatusSetterSuite) TestSetMachineStatus(c *tc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    machine.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	// Can't call set service status on a machine.
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestSetServiceStatus(c *tc.C) {
	// TODO: the correct way to fix this is to have the authorizer on the
	// simple status setter to check to see if the unit (authTag) is a leader
	// and able to set the service status. However, that is for another day.
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    service.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	// Can't call set service status on a service. Weird I know, but the only
	// way is to go through the unit leader.
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusNotLeader(c *tc.C) {
	// If the unit isn't the leader, it can't set it.
	s.leadershipChecker.isLeader = false
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, tc.ErrorMatches, "not leader")
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusIsLeader(c *tc.C) {
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: service,
		Status: &status.StatusInfo{
			Status: status.Maintenance,
		}})
	// No need to claim leadership - the checker passed in in setup
	// always returns true.
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: status.Active.String(),
	}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	err = service.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	unitStatus, err := service.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitStatus.Status, tc.Equals, status.Active)
}

func (s *serviceStatusSetterSuite) TestBulk(c *tc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: status.Active.String(),
	}, {
		Tag:    machine.Tag().String(),
		Status: status.Active.String(),
	}, {
		Tag:    "bad-tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 3)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[2].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type unitAgentFinderSuite struct{}

func TestUnitAgentFinderSuite(t *tctesting.T) {
	tc.Run(t, &unitAgentFinderSuite{})
}

func (unitAgentFinderSuite) TestFindEntity(c *tc.C) {
	f := fakeEntityFinder{
		unit: fakeUnit{
			agent: &state.UnitAgent{},
		},
	}
	ua := &common.UnitAgentFinder{f}
	entity, err := ua.FindEntity(names.NewUnitTag("unit/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity, tc.DeepEquals, f.unit.agent)
}

func (unitAgentFinderSuite) TestFindEntityBadTag(c *tc.C) {
	ua := &common.UnitAgentFinder{fakeEntityFinder{}}
	_, err := ua.FindEntity(names.NewApplicationTag("foo"))
	c.Assert(err, tc.ErrorMatches, "unsupported tag.*")
}

func (unitAgentFinderSuite) TestFindEntityErr(c *tc.C) {
	f := fakeEntityFinder{err: errors.Errorf("boo")}
	ua := &common.UnitAgentFinder{f}
	_, err := ua.FindEntity(names.NewUnitTag("unit/0"))
	c.Assert(errors.Cause(err), tc.Equals, f.err)
}

type fakeEntityFinder struct {
	unit fakeUnit
	err  error
}

func (f fakeEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return f.unit, f.err
}

type fakeUnit struct {
	state.Entity
	agent *state.UnitAgent
}

func (f fakeUnit) Agent() *state.UnitAgent {
	return f.agent
}
