// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type deployerSuite struct {
	testing.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	machine     *state.Machine
	app0        *state.Application
	app1        *state.Application
	principal   *state.Unit
	subordinate *state.Unit

	st *deployer.State
}

func TestDeployerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &deployerSuite{})
}

func (s *deployerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageModel, state.JobHostUnits)
	err := s.machine.SetProviderAddresses(network.NewSpaceAddress("0.1.2.3"))
	c.Assert(err, tc.ErrorIsNil)

	// Create the needed applications and relate them.
	s.app0 = s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.app1 = s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Create principal and subordinate units and assign them.
	s.principal, err = s.app0.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.principal.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	relUnit, err := rel.Unit(s.principal)
	c.Assert(err, tc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.subordinate, err = s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	// Create the deployer facade.
	s.st = deployer.NewState(s.stateAPI)
	c.Assert(s.st, tc.NotNil)
}

// Note: This is really meant as a unit-test, this isn't a test that
// should need all of the setup we have for this test suite
func (s *deployerSuite) TestNew(c *tc.C) {
	deployer := deployer.NewState(s.stateAPI)
	c.Assert(deployer, tc.NotNil)
}

func (s *deployerSuite) assertUnauthorized(c *tc.C, err error) {
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *deployerSuite) TestWatchUnitsWrongMachine(c *tc.C) {
	// Try with a non-existent machine tag.
	machine, err := s.st.Machine(names.NewMachineTag("42"))
	c.Assert(err, tc.ErrorIsNil)
	w, err := machine.WatchUnits()
	s.assertUnauthorized(c, err)
	c.Assert(w, tc.IsNil)
}

func (s *deployerSuite) TestWatchUnits(c *tc.C) {
	// TODO(dfc) fix state.Machine to return a MachineTag
	machine, err := s.st.Machine(s.machine.Tag().(names.MachineTag))
	c.Assert(err, tc.ErrorIsNil)
	w, err := machine.WatchUnits()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.subordinate.SetPassword("foo")
	c.Assert(err, tc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	wc.AssertNoChange()

	err = s.subordinate.SetPassword("foo-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the subordinate dead and check it's detected.
	err = s.subordinate.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()
}

func (s *deployerSuite) TestUnit(c *tc.C) {
	// Try getting a missing unit and an invalid tag.
	unit, err := s.st.Unit(names.NewUnitTag("foo/42"))
	s.assertUnauthorized(c, err)
	c.Assert(unit, tc.IsNil)

	// Try getting a unit we're not responsible for.
	// First create a new machine and deploy another unit there.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	principal1, err := s.app0.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = principal1.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	unit, err = s.st.Unit(principal1.Tag().(names.UnitTag))
	s.assertUnauthorized(c, err)
	c.Assert(unit, tc.IsNil)

	// Get the principal and subordinate we're responsible for.
	unit, err = s.st.Unit(s.principal.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/0")
	unit, err = s.st.Unit(s.subordinate.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "logging/0")
}

func (s *deployerSuite) TestUnitLifeRefresh(c *tc.C) {
	unit, err := s.st.Unit(s.subordinate.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(unit.Life(), tc.Equals, life.Alive)

	// Now make it dead and check again, then refresh and check.
	err = s.subordinate.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.subordinate.Life(), tc.Equals, state.Dead)
	c.Assert(unit.Life(), tc.Equals, life.Alive)
	err = unit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Life(), tc.Equals, life.Dead)
}

func (s *deployerSuite) TestUnitRemove(c *tc.C) {
	unit, err := s.st.Unit(s.principal.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	// It fails because the entity is still alive.
	// And EnsureDead will fail because there is a subordinate.
	err = unit.Remove()
	c.Assert(err, tc.ErrorMatches, `cannot remove entity "unit-mysql-0": still alive`)
	c.Assert(params.ErrCode(err), tc.Equals, "")

	// With the subordinate it also fails due to it being alive.
	unit, err = s.st.Unit(s.subordinate.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorMatches, `cannot remove entity "unit-logging-0": still alive`)
	c.Assert(params.ErrCode(err), tc.Equals, "")

	// Make it dead first and try again.
	err = s.subordinate.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	// Verify it's gone.
	err = unit.Refresh()
	s.assertUnauthorized(c, err)
	unit, err = s.st.Unit(s.subordinate.Tag().(names.UnitTag))
	s.assertUnauthorized(c, err)
	c.Assert(unit, tc.IsNil)
}

func (s *deployerSuite) TestUnitSetPassword(c *tc.C) {
	unit, err := s.st.Unit(s.principal.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	// Change the principal's password and verify.
	err = unit.SetPassword("foobar-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	err = s.principal.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.principal.PasswordValid("foobar-12345678901234567890"), tc.IsTrue)

	// Then the subordinate.
	unit, err = s.st.Unit(s.subordinate.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword("phony-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.subordinate.PasswordValid("phony-12345678901234567890"), tc.IsTrue)
}

func (s *deployerSuite) TestUnitSetStatus(c *tc.C) {
	unit, err := s.st.Unit(s.principal.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetStatus(status.Blocked, "waiting", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)

	stateUnit, err := s.BackingState.Unit(unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	sInfo, err := stateUnit.Status()
	c.Assert(err, tc.ErrorIsNil)
	sInfo.Since = nil
	c.Assert(sInfo, tc.DeepEquals, status.StatusInfo{
		Status:  status.Blocked,
		Message: "waiting",
		Data:    map[string]interface{}{"foo": "bar"},
	})
}
