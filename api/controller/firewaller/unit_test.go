// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type unitSuite struct {
	firewallerSuite

	apiUnit *firewaller.Unit
}

func TestUnitSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &unitSuite{})
}

func (s *unitSuite) SetUpTest(c *tc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiUnit, err = s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TearDownTest(c *tc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *unitSuite) TestUnit(c *tc.C) {
	apiUnitFoo, err := s.firewaller.Unit(names.NewUnitTag("foo/42"))
	c.Assert(err, tc.ErrorMatches, `unit "foo/42" not found`)
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiUnitFoo, tc.IsNil)

	apiUnit0, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiUnit0, tc.NotNil)
	c.Assert(apiUnit0.Name(), tc.Equals, s.units[0].Name())
	c.Assert(apiUnit0.Tag(), tc.Equals, names.NewUnitTag(s.units[0].Name()))
}

func (s *unitSuite) TestRefresh(c *tc.C) {
	c.Assert(s.apiUnit.Life(), tc.Equals, life.Alive)

	err := s.units[0].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), tc.Equals, life.Alive)

	err = s.apiUnit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), tc.Equals, life.Dead)
}

func (s *unitSuite) TestAssignedMachine(c *tc.C) {
	machineTag, err := s.apiUnit.AssignedMachine()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineTag, tc.Equals, names.NewMachineTag(s.machines[0].Id()))

	// Unassign now and check CodeNotAssigned is reported.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.apiUnit.AssignedMachine()
	c.Assert(err, tc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
	c.Assert(err, tc.Satisfies, params.IsCodeNotAssigned)
}

func (s *unitSuite) TestApplication(c *tc.C) {
	application, err := s.apiUnit.Application()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application.Name(), tc.Equals, s.application.Name())
}

func (s *unitSuite) TestName(c *tc.C) {
	c.Assert(s.apiUnit.Name(), tc.Equals, s.units[0].Name())
}
