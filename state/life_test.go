// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

type LifeSuite struct {
	ConnSuite
	charm *state.Charm
	app   *state.Application
}

func (s *LifeSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.app = s.AddTestingApplication(c, "dummyapp", s.charm)
}

func TestLifeSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &LifeSuite{})
}

var stateChanges = []struct {
	cached, desired    state.Life
	dbinitial, dbfinal state.Life
}{
	{
		state.Alive, state.Dying,
		state.Alive, state.Dying,
	},
	{
		state.Alive, state.Dying,
		state.Dying, state.Dying,
	},
	{
		state.Alive, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Alive, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Dying, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Dead, state.Dead,
	},
	{
		state.Dying, state.Dying,
		state.Dying, state.Dying,
	},
	{
		state.Dying, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Dying, state.Dead,
		state.Dying, state.Dead,
	},
	{
		state.Dying, state.Dead,
		state.Dead, state.Dead,
	},
	{
		state.Dead, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Dead, state.Dead,
		state.Dead, state.Dead,
	},
}

type lifeFixture interface {
	id() (coll string, id interface{})
	setup(s *LifeSuite, c *tc.C) state.AgentLiving
	isDying(s *LifeSuite, c *tc.C) bool
}

type unitLife struct {
	unit *state.Unit
	st   *state.State
}

func (l *unitLife) id() (coll string, id interface{}) {
	return state.UnitsC, state.DocID(l.st, l.unit.Name())
}

func (l *unitLife) setup(s *LifeSuite, c *tc.C) state.AgentLiving {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	l.unit = unit
	return l.unit
}

func (l *unitLife) isDying(s *LifeSuite, c *tc.C) bool {
	col, id := l.id()
	dying, err := state.IsDying(l.st, col, id)
	c.Assert(err, tc.ErrorIsNil)
	return dying
}

type machineLife struct {
	machine *state.Machine
	st      *state.State
}

func (l *machineLife) id() (coll string, id interface{}) {
	return state.MachinesC, state.DocID(l.st, l.machine.Id())
}

func (l *machineLife) setup(s *LifeSuite, c *tc.C) state.AgentLiving {
	var err error
	l.machine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	return l.machine
}

func (l *machineLife) isDying(s *LifeSuite, c *tc.C) bool {
	col, id := l.id()
	dying, err := state.IsDying(l.st, col, id)
	c.Assert(err, tc.ErrorIsNil)
	return dying
}

func (s *LifeSuite) prepareFixture(living state.Living, lfix lifeFixture, cached, dbinitial state.Life, c *tc.C) {
	collName, id := lfix.id()
	coll := s.MgoSuite.Session.DB("juju").C(collName)

	err := coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", cached},
	}}})
	c.Assert(err, tc.ErrorIsNil)
	err = living.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	err = coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", dbinitial},
	}}})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *LifeSuite) TestLifecycleStateChanges(c *tc.C) {
	for i, lfix := range []lifeFixture{&unitLife{st: s.State}, &machineLife{st: s.State}} {
		c.Logf("fixture %d", i)
		for j, v := range stateChanges {
			c.Logf("sequence %d", j)
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Destroy()
				c.Assert(err, tc.ErrorIsNil)

				// If we're already in the dead state, we can't transition, so
				// don't test that permutation.
				if v.dbinitial != state.Dead {
					ok := lfix.isDying(s, c)
					c.Assert(ok, tc.IsTrue)
				}
			case state.Dead:
				err := living.EnsureDead()
				c.Assert(err, tc.ErrorIsNil)
			default:
				panic("desired lifecycle can only be dying or dead")
			}
			err := living.Refresh()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(living.Life(), tc.Equals, v.dbfinal)
			err = living.EnsureDead()
			c.Assert(err, tc.ErrorIsNil)
			err = living.Remove()
			c.Assert(err, tc.ErrorIsNil)
		}
	}
}

func (s *LifeSuite) TestLifeString(c *tc.C) {
	var tests = []struct {
		life state.Life
		want string
	}{
		{state.Alive, "alive"},
		{state.Dying, "dying"},
		{state.Dead, "dead"},
		{42, "unknown"},
	}
	for _, test := range tests {
		got := test.life.String()
		c.Assert(got, tc.Equals, test.want)
	}
}

const (
	notAliveErr = ".*: .* is not found or not alive"
	deadErr     = ".*: not found or dead"
	noErr       = ""
)

type lifer interface {
	EnsureDead() error
	Destroy() error
	Life() state.Life
}

func runLifeChecks(c *tc.C, obj lifer, expectErr string, checks []func() error) {
	for i, check := range checks {
		c.Logf("check %d when %v", i, obj.Life())
		err := check()
		if expectErr == noErr {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorMatches, expectErr)
		}
	}
}

// testWhenDying sets obj to Dying and Dead in turn, and asserts
// that the errors from the given checks match aliveErr, dyingErr and deadErr
// in each respective life state.
func testWhenDying(c *tc.C, obj lifer, dyingErr, deadErr string, checks ...func() error) {
	c.Logf("checking life of %v (%T)", obj, obj)
	err := obj.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	runLifeChecks(c, obj, dyingErr, checks)
	err = obj.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	runLifeChecks(c, obj, deadErr, checks)
}
