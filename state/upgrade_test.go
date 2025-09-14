// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type UpgradeSuite struct {
	ConnSuite
	serverIdA string
}

func TestUpgradeSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &UpgradeSuite{})
}

func vers(s string) version.Number {
	return version.MustParse(s)
}

func (s *UpgradeSuite) provision(c *tc.C, machineIds ...string) {
	for _, machineId := range machineIds {
		machine, err := s.State.Machine(machineId)
		c.Assert(err, tc.ErrorIsNil)
		err = machine.SetProvisioned(
			instance.Id(fmt.Sprintf("instance-%s", machineId)),
			"",
			fmt.Sprintf("nonce-%s", machineId),
			nil,
		)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *UpgradeSuite) addControllers(c *tc.C) (machineId1, machineId2 string) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	return changes.Added[0], changes.Added[1]
}

func (s *UpgradeSuite) assertUpgrading(c *tc.C, expect bool) {
	upgrading, err := s.State.IsUpgrading()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(upgrading, tc.Equals, expect)
}

func (s *UpgradeSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	controller, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	s.serverIdA = controller.Id()
	s.provision(c, s.serverIdA)
}

func (s *UpgradeSuite) assertEnsureUpgradeInfo(c *tc.C, st *state.State, controllerId string) {
	vPrevious := vers("1.2.3")
	vTarget := vers("2.3.4")
	vMismatch := vers("1.9.1")

	// create
	info, err := st.EnsureUpgradeInfo(controllerId, vPrevious, vTarget)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.PreviousVersion(), tc.DeepEquals, vPrevious)
	c.Assert(info.TargetVersion(), tc.DeepEquals, vTarget)
	c.Assert(info.Status(), tc.Equals, state.UpgradePending)
	c.Assert(info.Started().IsZero(), tc.IsFalse)
	c.Assert(info.ControllersReady(), tc.DeepEquals, []string{controllerId})
	c.Assert(info.ControllersDone(), tc.HasLen, 0)

	// retrieve existing
	info, err = st.EnsureUpgradeInfo(controllerId, vPrevious, vTarget)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.PreviousVersion(), tc.DeepEquals, vPrevious)
	c.Assert(info.TargetVersion(), tc.DeepEquals, vTarget)

	// mismatching previous
	info, err = st.EnsureUpgradeInfo(controllerId, vMismatch, vTarget)
	c.Assert(err, tc.ErrorMatches, "current upgrade info mismatch: expected previous version 1.9.1, got 1.2.3")
	c.Assert(info, tc.IsNil)

	// mismatching target
	info, err = st.EnsureUpgradeInfo(controllerId, vPrevious, vMismatch)
	c.Assert(err, tc.ErrorMatches, "current upgrade info mismatch: expected target version 1.9.1, got 2.3.4")
	c.Assert(info, tc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfo(c *tc.C) {
	s.assertEnsureUpgradeInfo(c, s.State, s.serverIdA)
}

func (s *UpgradeSuite) TestCAASEnsureUpgradeInfo(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	node, err := st.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)

	s.assertEnsureUpgradeInfo(c, st, node.Id())
}

func (s *UpgradeSuite) TestControllersReadyCopies(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.4.5"))
	c.Assert(err, tc.ErrorIsNil)
	controllersReady := info.ControllersReady()
	c.Assert(controllersReady, tc.DeepEquals, []string{"0"})
	controllersReady[0] = "lol"
	controllersReady = info.ControllersReady()
	c.Assert(controllersReady, tc.DeepEquals, []string{"0"})
}

func (s *UpgradeSuite) TestControllersDoneCopies(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.4.5"))
	c.Assert(err, tc.ErrorIsNil)
	s.setToRunning(c, info)
	err = info.SetControllerDone("0")
	c.Assert(err, tc.ErrorIsNil)

	info = s.getOneUpgradeInfo(c)
	controllersDone := info.ControllersDone()
	c.Assert(controllersDone, tc.DeepEquals, []string{"0"})
	controllersDone[0] = "lol"
	controllersDone = info.ControllersReady()
	c.Assert(controllersDone, tc.DeepEquals, []string{"0"})
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoDowngrade(c *tc.C) {
	v123 := vers("1.2.3")
	v111 := vers("1.1.1")

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v111)
	c.Assert(err, tc.ErrorMatches, "cannot upgrade from 1.2.3 to 1.1.1")
	c.Assert(info, tc.IsNil)

	info, err = s.State.EnsureUpgradeInfo(s.serverIdA, v123, v123)
	c.Assert(err, tc.ErrorMatches, "cannot upgrade from 1.2.3 to 1.2.3")
	c.Assert(info, tc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoNonController(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo("2345678", vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, tc.ErrorMatches, `machine "2345678" is not a controller`)
	c.Assert(info, tc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoNotProvisioned(c *tc.C) {
	serverIdB, _ := s.addControllers(c)
	_, err := s.State.EnsureUpgradeInfo(serverIdB, vers("1.1.1"), vers("1.2.3"))
	expectErr := fmt.Sprintf("machine %s is not provisioned and should not be participating in upgrades", serverIdB)
	c.Assert(err, tc.ErrorMatches, expectErr)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServers(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	// add first new controller with bad version
	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, vers("1.2.4"))
	c.Assert(err, tc.ErrorMatches, "current upgrade info mismatch: expected target version 1.2.4, got 1.2.3")
	c.Assert(info, tc.IsNil)

	// add first new controller properly
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	expectReady := []string{s.serverIdA, serverIdB}
	c.Assert(info.ControllersReady(), tc.SameContents, expectReady)

	// add second new controller
	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	expectReady = append(expectReady, serverIdC)
	c.Assert(info.ControllersReady(), tc.SameContents, expectReady)

	// add second new controller again
	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.ControllersReady(), tc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoRace(c *tc.C) {
	v100 := vers("1.0.0")
	v200 := vers("2.0.0")

	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, tc.ErrorMatches, "current upgrade info not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(info, tc.IsNil)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace1(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	expectReady := []string{serverIdB, serverIdC}
	c.Assert(info.ControllersReady(), tc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace2(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, func() {
		_, err := s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	info, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	expectReady := []string{s.serverIdA, serverIdB, serverIdC}
	c.Assert(info.ControllersReady(), tc.SameContents, expectReady)
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace3(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, nil, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v124)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorMatches, "upgrade info changed during update")
}

func (s *UpgradeSuite) TestEnsureUpgradeInfoMultipleServersRace4(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetAfterHooks(c, s.State, nil, func() {
		err := s.State.ClearUpgradeInfo()
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v124)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorMatches, "current upgrade info mismatch: expected target version 1.2.3, got 1.2.4")
}

func (s *UpgradeSuite) TestRefresh(c *tc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, _ := s.addControllers(c)
	s.provision(c, serverIdB)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	info2, err := s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	err = info2.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.ControllersReady(), tc.SameContents, []string{s.serverIdA})
	c.Assert(info.Status(), tc.Equals, state.UpgradePending)

	err = info.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.ControllersReady(), tc.SameContents, []string{s.serverIdA, serverIdB})
	c.Assert(info.Status(), tc.Equals, state.UpgradeDBComplete)
}

func (s *UpgradeSuite) TestWatch(c *tc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	w := s.State.WatchUpgradeInfo()
	defer statetesting.AssertStop(c, w)

	// initial event
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// single change is reported
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// non-change is not reported
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// changes are coalesced
	_, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// closed on stop
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *UpgradeSuite) TestWatchMethod(c *tc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := info.Watch()
	defer statetesting.AssertStop(c, w)

	// initial event
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// single change is reported
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// non-change is not reported
	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// changes are coalesced
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// closed on stop
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *UpgradeSuite) TestAllProvisionedControllersReady(c *tc.C) {
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB)

	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)

	assertReady := func(expect bool) {
		ok, err := info.AllProvisionedControllersReady()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ok, tc.Equals, expect)
	}
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdB, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	assertReady(true)

	s.provision(c, serverIdC)
	assertReady(false)

	info, err = s.State.EnsureUpgradeInfo(serverIdC, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	assertReady(true)
}

func (s *UpgradeSuite) TestSetStatusSetsModelStatus(c *tc.C) {
	v123 := vers("1.2.3")
	v234 := vers("2.3.4")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
	c.Assert(err, tc.ErrorIsNil)

	assertStatus := func(expect state.UpgradeStatus) {
		info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(info.Status(), tc.Equals, expect)
	}

	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	st, err := m.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st.Status, tc.Equals, status.Available)
	c.Assert(st.Message, tc.Equals, "")

	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)
	assertStatus(state.UpgradeRunning)

	st, err = m.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st.Status, tc.Equals, status.Busy)
	c.Assert(st.Message, tc.HasPrefix, "upgrade in progress since")

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorIsNil)
	st, err = m.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st.Status, tc.Equals, status.Available)
	c.Assert(st.Message, tc.HasPrefix, "upgraded on")
}

func (s *UpgradeSuite) TestSetStatusSuccess(c *tc.C) {
	v123 := vers("1.2.3")
	v234 := vers("2.3.4")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
	c.Assert(err, tc.ErrorIsNil)

	assertStatus := func(expect state.UpgradeStatus) {
		info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(info.Status(), tc.Equals, expect)
	}

	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)
	assertStatus(state.UpgradeDBComplete)

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)
	assertStatus(state.UpgradeRunning)

	// Test idempotency for when multiple controllers commence upgrade steps.
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)
	assertStatus(state.UpgradeRunning)
}

func (s *UpgradeSuite) TestSetStatusErrors(c *tc.C) {
	v123 := vers("1.2.3")
	v234 := vers("2.3.4")
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
	c.Assert(err, tc.ErrorIsNil)

	assertStatus := func(expect state.UpgradeStatus) {
		info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v234)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(info.Status(), tc.Equals, expect)
	}

	err = info.SetStatus(state.UpgradePending)
	c.Assert(err, tc.ErrorMatches, `cannot explicitly set upgrade status to "pending"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeComplete)
	c.Assert(err, tc.ErrorMatches, `cannot explicitly set upgrade status to "complete"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeAborted)
	c.Assert(err, tc.ErrorMatches, `cannot explicitly set upgrade status to "aborted"`)
	assertStatus(state.UpgradePending)

	err = info.SetStatus("lol")
	c.Assert(err, tc.ErrorMatches, "unknown upgrade status: lol")
	assertStatus(state.UpgradePending)

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorMatches, `setting upgrade status to "running": `+
		`upgrade status transition from "pending" to "running" not valid`)
}

func (s *UpgradeSuite) TestSetControllerDone(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, tc.ErrorIsNil)

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorMatches, "cannot complete upgrade: upgrade has not yet run")

	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorMatches, "cannot complete upgrade: upgrade has not yet run")

	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, false)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeComplete, 1)
}

func (s *UpgradeSuite) TestSetControllerDoneMultipleServers(c *tc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)
	for _, id := range []string{serverIdB, serverIdC} {
		_, err := s.State.EnsureUpgradeInfo(id, v111, v123)
		c.Assert(err, tc.ErrorIsNil)
	}

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	s.setToRunning(c, info)

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetControllerDone(serverIdB)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = info.SetControllerDone(serverIdC)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, false)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeComplete, 3)
}

func (s *UpgradeSuite) TestSetControllerDoneMultipleServersRace(c *tc.C) {
	v100 := vers("1.0.0")
	v200 := vers("2.0.0")
	serverIdB, serverIdC := s.addControllers(c)
	s.provision(c, serverIdB, serverIdC)

	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, v100, v200)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(serverIdB, v100, v200)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(serverIdC, v100, v200)
	c.Assert(err, tc.ErrorIsNil)
	s.setToRunning(c, info)

	// Interrupt the transaction for controller A twice with calls
	// from the other machines.
	defer state.SetBeforeHooks(c, s.State, func() {
		err = info.SetControllerDone(serverIdB)
		c.Assert(err, tc.ErrorIsNil)
	}, func() {
		err = info.SetControllerDone(serverIdC)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	err = info.SetControllerDone(s.serverIdA)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, false)

	info = s.getOneUpgradeInfo(c)
	c.Assert(info.Status(), tc.Equals, state.UpgradeComplete)
	c.Assert(info.ControllersDone(), tc.SameContents, []string{"0", "1", "2"})
}

func (s *UpgradeSuite) TestAbort(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, tc.ErrorIsNil)

	err = info.Abort()
	c.Assert(err, tc.ErrorIsNil)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeAborted, 0)
}

func (s *UpgradeSuite) TestAbortRace(c *tc.C) {
	info, err := s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("2.3.4"))
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err = info.Abort()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	err = info.Abort()
	c.Assert(err, tc.ErrorIsNil)

	s.checkUpgradeInfoArchived(c, info, state.UpgradeAborted, 0)
}

func (s *UpgradeSuite) checkUpgradeInfoArchived(
	c *tc.C,
	initialInfo *state.UpgradeInfo,
	expectedStatus state.UpgradeStatus,
	expectedControllers int,
) {
	info := s.getOneUpgradeInfo(c)
	c.Assert(info.Status(), tc.Equals, expectedStatus)
	c.Assert(info.PreviousVersion(), tc.Equals, initialInfo.PreviousVersion())
	c.Assert(info.TargetVersion(), tc.Equals, initialInfo.TargetVersion())
	c.Assert(info.Started().Equal(truncateDBTime(initialInfo.Started())), tc.IsTrue)
	c.Assert(len(info.ControllersDone()), tc.Equals, expectedControllers)
	if expectedControllers > 0 {
		c.Assert(info.ControllersDone(), tc.SameContents, info.ControllersReady())
	}
}

func (s *UpgradeSuite) getOneUpgradeInfo(c *tc.C) *state.UpgradeInfo {
	upgradeInfos, err := state.GetAllUpgradeInfos(s.State)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(upgradeInfos), tc.Equals, 1)
	return upgradeInfos[0]
}

func (s *UpgradeSuite) TestAbortCurrentUpgrade(c *tc.C) {
	// First try with nothing to abort.
	err := s.State.AbortCurrentUpgrade()
	c.Assert(err, tc.ErrorIsNil)

	upgradeInfos, err := state.GetAllUpgradeInfos(s.State)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(upgradeInfos), tc.Equals, 0)

	// Now create a UpgradeInfo to abort.
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.1.1"), vers("1.2.3"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.AbortCurrentUpgrade()
	c.Assert(err, tc.ErrorIsNil)

	info := s.getOneUpgradeInfo(c)
	c.Check(info.Status(), tc.Equals, state.UpgradeAborted)

	// It should now be possible to start another upgrade.
	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, vers("1.2.3"), vers("1.3.0"))
	c.Check(err, tc.ErrorIsNil)
}

func (s *UpgradeSuite) TestClearUpgradeInfo(c *tc.C) {
	v111 := vers("1.1.1")
	v123 := vers("1.2.3")
	v153 := vers("1.5.3")

	s.assertUpgrading(c, false)
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v111, v123)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)

	err = s.State.ClearUpgradeInfo()
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, false)

	_, err = s.State.EnsureUpgradeInfo(s.serverIdA, v111, v153)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)
}

func (s *UpgradeSuite) TestApplicationUnitSeqToSequence(c *tc.C) {
	v123 := vers("1.2.3")
	v124 := vers("1.2.4")

	s.assertUpgrading(c, false)
	_, err := s.State.EnsureUpgradeInfo(s.serverIdA, v123, v124)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpgrading(c, true)
}

func (s *UpgradeSuite) setToRunning(c *tc.C, info *state.UpgradeInfo) {
	err := info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeRunning)
	c.Assert(err, tc.ErrorIsNil)
}
