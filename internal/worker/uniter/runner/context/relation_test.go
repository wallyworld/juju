// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type ContextRelationSuite struct {
	testing.JujuConnSuite
	app *state.Application
	rel *state.Relation
	ru  *state.RelationUnit

	st      api.Connection
	uniter  *apiuniter.State
	relUnit context.RelationUnit
}

func TestContextRelationSuite(t *tctesting.T) {
	jujutesting.MgoTestPackage(t, &ContextRelationSuite{})
}

func (s *ContextRelationSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "riak")
	s.app = s.AddTestingApplication(c, "u", ch)
	rels, err := s.app.Relations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rels, tc.HasLen, 1)
	s.rel = rels[0]
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = s.ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	password, err = utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = uniter.NewFromConnection(s.st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.uniter, tc.NotNil)

	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, tc.ErrorIsNil)
	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	relUnit, err := apiRel.Unit(apiUnit.Tag())
	c.Assert(err, tc.ErrorIsNil)
	s.relUnit = &relUnitShim{relUnit}
}

func (s *ContextRelationSuite) TestMemberCaching(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, tc.ErrorIsNil)
	settings, err := ru.Settings()
	c.Assert(err, tc.ErrorIsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, tc.ErrorIsNil)

	cache := context.NewRelationCache(s.relUnit.ReadSettings, []string{"u/1"})
	ctx := context.NewContextRelation(s.relUnit, cache, false)

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, tc.ErrorIsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, tc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, tc.ErrorIsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, expectSettings)
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, tc.ErrorIsNil)
	settings, err := ru.Settings()
	c.Assert(err, tc.ErrorIsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, tc.ErrorIsNil)

	cache := context.NewRelationCache(s.relUnit.ReadSettings, nil)
	ctx := context.NewContextRelation(s.relUnit, cache, false)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, tc.ErrorIsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, tc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the obtained settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, tc.ErrorIsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, expectSettings)
}

func convertMap(settingsMap map[string]interface{}) params.Settings {
	result := make(params.Settings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}

func (s *ContextRelationSuite) TestSuspended(c *tc.C) {
	_, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.rel.SetSuspended(true, "")
	c.Assert(err, tc.ErrorIsNil)

	ctx := context.NewContextRelation(s.relUnit, nil, false)
	err = s.relUnit.Relation().Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx.Suspended(), tc.IsTrue)
}

func (s *ContextRelationSuite) TestSetStatus(c *tc.C) {
	_, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.State.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	err = claimer.Claim("u", "u/0", time.Minute)
	c.Assert(err, tc.ErrorIsNil)

	ctx := context.NewContextRelation(s.relUnit, nil, false)
	err = ctx.SetStatus(relation.Suspended)
	c.Assert(err, tc.ErrorIsNil)
	relStatus, err := s.rel.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relStatus.Status, tc.Equals, status.Suspended)
}

func (s *ContextRelationSuite) TestRemoteApplicationName(c *tc.C) {
	ctx := context.NewContextRelation(s.relUnit, nil, false)
	c.Assert(ctx.RemoteApplicationName(), tc.Equals, "u")
}

func (s *ContextRelationSuite) TestRemoteModelUUID(c *tc.C) {
	ctx := context.NewContextRelation(s.relUnit, nil, false)
	c.Assert(ctx.RemoteModelUUID(), tc.Equals, jujutesting.ModelTag.Id())
}

type relUnitShim struct {
	*apiuniter.RelationUnit
}

func (r *relUnitShim) Relation() context.Relation {
	return r.RelationUnit.Relation()
}
