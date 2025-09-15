// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestUnitSuite(t *tctesting.T) {
	tc.Run(t, &unitSuite{})
}

type unitSuite struct {
	testing.JujuConnSuite
	unit *state.Unit
	st   api.Connection
}

func (s *unitSuite) SetUpTest(c *tc.C) {
	var err error
	s.JujuConnSuite.SetUpTest(c)
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)

	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
}

func (s *unitSuite) TestUnitEntity(c *tc.C) {
	tag := names.NewUnitTag("wordpress/1")
	apiSt, err := apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err := apiSt.Entity(tag)
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, tc.IsNil)

	apiSt, err = apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err = apiSt.Entity(s.unit.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Tag(), tc.Equals, s.unit.Tag().String())
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(m.Jobs(), tc.HasLen, 0)

	err = s.unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	apiSt, err = apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err = apiSt.Entity(s.unit.Tag())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("unit %q not found", s.unit.Name()))
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, tc.IsNil)
}
