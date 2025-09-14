// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type EnableHASuite struct {
	ConnSuite
}

func TestEnableHASuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &EnableHASuite{})
}

func (s *EnableHASuite) TestHasVote(c *tc.C) {
	controller, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)

	node, err := s.State.ControllerNode(controller.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	c.Assert(node.HasVote(), tc.IsFalse)

	// Make another node value so that
	// it won't have the cached HasVote value.
	nodeCopy, err := s.State.ControllerNode(controller.Id())
	c.Assert(err, tc.ErrorIsNil)

	err = node.SetHasVote(true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Assert(node.HasVote(), tc.IsTrue)
	c.Assert(nodeCopy.HasVote(), tc.IsFalse)

	err = nodeCopy.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nodeCopy.HasVote(), tc.IsTrue)

	err = nodeCopy.SetHasVote(false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nodeCopy.Refresh(), tc.ErrorIsNil)
	c.Assert(nodeCopy.HasVote(), tc.IsFalse)

	c.Assert(node.HasVote(), tc.IsTrue)
	err = node.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.HasVote(), tc.IsFalse)
}

func (s *EnableHASuite) TestEnableHAFailsWithBadCount(c *tc.C) {
	for _, n := range []int{-1, 2, 6} {
		changes, err := s.State.EnableHA(n, constraints.Value{}, state.Base{}, nil)
		c.Assert(err, tc.ErrorMatches, "number of controllers must be odd and non-negative")
		c.Assert(changes.Added, tc.HasLen, 0)
	}
	_, err := s.State.EnableHA(controller.MaxPeers+2, constraints.Value{}, state.Base{}, nil)
	c.Assert(err, tc.ErrorMatches, `controller count is too large \(allowed \d+\)`)
}

func (s *EnableHASuite) TestEnableHAAddsNewMachines(c *tc.C) {
	ids := make([]string, 3)
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add a non-controller machine just to make sure.
	_, err = s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	cons := constraints.Value{
		Mem: newUint64(100),
	}
	changes, err := s.State.EnableHA(3, cons, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i + 1))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Jobs(), tc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(gotCons, tc.DeepEquals, cons)
		node, err := s.State.ControllerNode(m0.Id())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(node.HasVote(), tc.IsFalse)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func (s *EnableHASuite) TestEnableHAAddsControllerCharm(c *tc.C) {
	state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("20.04"), "controller",
		state.AddTestingCharmMultiSeries(c, s.State, "juju-controller"))
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	for i := 0; i < 3; i++ {
		unitName := fmt.Sprintf("controller/%d", i)
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Principals(), tc.DeepEquals, []string{unitName})
		u, err := s.State.Unit(unitName)
		c.Assert(err, tc.ErrorIsNil)
		mID, err := u.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mID, tc.Equals, fmt.Sprint(i))
	}
}

func (s *EnableHASuite) TestEnableHAAddsControllerCharmToPromoted(c *tc.C) {
	state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("20.04"), "controller",
		state.AddTestingCharmMultiSeries(c, s.State, "juju-controller"))
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), []string{"0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	c.Assert(changes.Converted, tc.HasLen, 1)
	for i := 0; i < 3; i++ {
		unitName := fmt.Sprintf("controller/%d", i)
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Principals(), tc.DeepEquals, []string{unitName})
		u, err := s.State.Unit(unitName)
		c.Assert(err, tc.ErrorIsNil)
		mID, err := u.AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mID, tc.Equals, fmt.Sprint(i))
	}
	err = m0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m0.Principals(), tc.DeepEquals, []string{"controller/0"})
}

func (s *EnableHASuite) TestEnableHATo(c *tc.C) {
	ids := make([]string, 3)
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add two non-controller machines.
	_, err = s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), []string{"1", "2"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 0)
	c.Assert(changes.Converted, tc.HasLen, 2)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Jobs(), tc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(gotCons, tc.DeepEquals, constraints.Value{})
		node, err := s.State.ControllerNode(m0.Id())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(node.HasVote(), tc.IsFalse)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func (s *EnableHASuite) TestEnableHAToPartial(c *tc.C) {
	ids := make([]string, 3)
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	ids[0] = m0.Id()

	// Add one non-controller machine.
	_, err = s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.assertControllerInfo(c, []string{"0"}, []string{"0"}, nil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), []string{"1"})
	c.Assert(err, tc.ErrorIsNil)

	// One machine is converted (existing machine with placement),
	// and another is added to make up the 3.
	c.Assert(changes.Converted, tc.HasLen, 1)
	c.Assert(changes.Added, tc.HasLen, 1)

	for i := 1; i < 3; i++ {
		m, err := s.State.Machine(fmt.Sprint(i))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Jobs(), tc.DeepEquals, []state.MachineJob{
			state.JobHostUnits,
			state.JobManageModel,
		})
		gotCons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(gotCons, tc.DeepEquals, constraints.Value{})
		node, err := s.State.ControllerNode(m0.Id())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(node.HasVote(), tc.IsFalse)
		ids[i] = m.Id()
	}
	s.assertControllerInfo(c, ids, ids, nil)
}

func newUint64(i uint64) *uint64 {
	return &i
}

func (s *EnableHASuite) assertControllerInfo(c *tc.C, expectedIds []string, wantVoteMachineIds []string, placement []string) {
	controllerIds, err := s.State.ControllerIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerIds, tc.SameContents, expectedIds)

	foundVoting := make([]string, 0)
	for i, id := range expectedIds {
		m, err := s.State.Machine(id)
		c.Assert(err, tc.ErrorIsNil)
		if len(placement) == 0 || i >= len(placement) {
			c.Check(m.Placement(), tc.Equals, "")
		} else {
			c.Check(m.Placement(), tc.Equals, placement[i])
		}
		node, err := s.State.ControllerNode(id)
		c.Assert(err, tc.ErrorIsNil)
		if node.WantsVote() {
			foundVoting = append(foundVoting, m.Id())
		}
	}
	c.Check(foundVoting, tc.DeepEquals, wantVoteMachineIds)
}

func (s *EnableHASuite) TestEnableHASamePlacementAsNewCount(c *tc.C) {
	placement := []string{"p1", "p2", "p3"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), placement)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *EnableHASuite) TestEnableHAMorePlacementThanNewCount(c *tc.C) {
	placement := []string{"p1", "p2", "p3", "p4"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), placement)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2", "p3"})
}

func (s *EnableHASuite) TestEnableHALessPlacementThanNewCount(c *tc.C) {
	placement := []string{"p1", "p2"}
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), placement)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, []string{"p1", "p2"})
}

func (s *EnableHASuite) TestEnableHAMockBootstrap(c *tc.C) {
	// Testing based on lp:1748275 - Juju HA fails due to demotion of Machine 0
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, tc.ErrorIsNil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	c.Assert(changes.Maintained, tc.DeepEquals, []string{"0"})
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
}

func (s *EnableHASuite) TestEnableHADefaultsTo3(c *tc.C) {
	changes, err := s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	// Mark machine 0 as being removed, and then run it again
	s.progressControllerToDead(c, "0")
	changes, err = s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.DeepEquals, []string{"3"})

	// New controller machine "3" is created
	s.assertControllerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.WantsVote(), tc.IsFalse)
	c.Assert(m0.IsManager(), tc.IsFalse) // job still intact for now
	m3, err := s.State.Machine("3")
	c.Assert(err, tc.ErrorIsNil)
	node, err = s.State.ControllerNode(m3.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(node.HasVote(), tc.IsFalse) // No vote yet.
	c.Assert(m3.IsManager(), tc.IsTrue)
}

// progressControllerToDead starts the machine as dying, and then does what the normal workers would do
// (like peergrouper), and runs all the cleanups to progress the machine all the way to dead.
func (s *EnableHASuite) progressControllerToDead(c *tc.C, id string) {
	m, err := s.State.Machine(id)
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(id)
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("destroying machine 0")
	c.Assert(m.Destroy(), tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Check(node.WantsVote(), tc.IsFalse)
	// Pretend to be the peergrouper, notice the machine doesn't want to vote, so get rid of its vote, and remove it
	// as a controller machine.
	c.Check(node.SetHasVote(false), tc.ErrorIsNil)
	// TODO(HA) - no longer need to refresh once HasVote is moved off machine
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Assert(s.State.RemoveControllerReference(node), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	c.Assert(m.EnsureDead(), tc.ErrorIsNil)
}

func (s *EnableHASuite) TestEnableHAGoesToNextOdd(c *tc.C) {
	changes, err := s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	// "run the peergrouper" and give all the controllers the vote
	for _, id := range []string{"0", "1", "2"} {
		node, err := s.State.ControllerNode(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(node.SetHasVote(true), tc.ErrorIsNil)
	}
	// Remove machine 0, so that we are down to 2 machines that want to vote. Requesting a count of '0' should
	// still bring us back to 3
	s.progressControllerToDead(c, "0")
	s.assertControllerInfo(c, []string{"1", "2"}, []string{"1", "2"}, nil)
	changes, err = s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	// We should try to get back to 3 again, since we only have 2 voting machines
	c.Check(changes.Added, tc.DeepEquals, []string{"3"})
	s.assertControllerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	// Doing it again with 0, should be a no-op, still going to '3'
	changes, err = s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changes.Added, tc.HasLen, 0)
	s.assertControllerInfo(c, []string{"1", "2", "3"}, []string{"1", "2", "3"}, nil)
	// Now if we go up to 5, and drop down to 4, we should again go to 5
	changes, err = s.State.EnableHA(5, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	sort.Strings(changes.Added)
	c.Check(changes.Added, tc.DeepEquals, []string{"4", "5"})
	s.assertControllerInfo(c, []string{"1", "2", "3", "4", "5"}, []string{"1", "2", "3", "4", "5"}, nil)
	s.progressControllerToDead(c, "1")
	s.assertControllerInfo(c, []string{"2", "3", "4", "5"}, []string{"2", "3", "4", "5"}, nil)
	changes, err = s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changes.Added, tc.DeepEquals, []string{"6"})
	s.assertControllerInfo(c, []string{"2", "3", "4", "5", "6"}, []string{"2", "3", "4", "5", "6"}, nil)
	// And again 0 should be treated as 5, and thus a no-op
	changes, err = s.State.EnableHA(0, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changes.Added, tc.HasLen, 0)
	s.assertControllerInfo(c, []string{"2", "3", "4", "5", "6"}, []string{"2", "3", "4", "5", "6"}, nil)
}

func (s *EnableHASuite) TestEnableHAConcurrentSame(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
		c.Assert(err, tc.ErrorIsNil)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..5.
		c.Assert(changes.Added, tc.HasLen, 3)
		expected := []string{"3", "4", "5"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.DeepEquals, []string{"0", "1", "2"})
	s.assertControllerInfo(c, []string{"3", "4", "5"}, []string{"3", "4", "5"}, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestEnableHAConcurrentLess(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(changes.Added, tc.HasLen, 3)
		// The outer EnableHA call will initially allocate IDs 0..4,
		// and the inner one 5..7.
		expected := []string{"5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..4, and fail due to the concurrent change. It will then
	// allocate machines 8..9 to make up the difference from the concurrent
	// EnableHA call.
	changes, err := s.State.EnableHA(5, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	expected := []string{"5", "6", "7", "8", "9"}
	s.assertControllerInfo(c, expected, expected, nil)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestEnableHAConcurrentMore(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		changes, err := s.State.EnableHA(5, constraints.Value{}, state.UbuntuBase("18.04"), nil)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(changes.Added, tc.HasLen, 5)
		// The outer EnableHA call will allocate IDs 0..2,
		// and the inner one 3..7.
		expected := []string{"3", "4", "5", "6", "7"}
		s.assertControllerInfo(c, expected, expected, nil)
	}).Check()

	// This call to EnableHA will initially attempt to allocate
	// machines 0..2, and fail due to the concurrent change. It will then
	// find that the number of voting machines in state is greater than
	// what we're attempting to ensure, and fail.
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorMatches, "failed to enable HA with 3 controllers: cannot remove controllers with enable-ha, use remove-machine and chose the controller\\(s\\) to remove")
	c.Assert(changes.Added, tc.HasLen, 0)

	// Machine 0 should never have been created.
	_, err = s.State.Machine("0")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *EnableHASuite) TestWatchControllerInfo(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.State.WatchControllerInfo()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange("0")

	info, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &state.ControllerInfo{
		CloudName:     "dummy",
		ModelTag:      s.modelTag,
		ControllerIds: []string{"0"},
	})

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)

	wc.AssertChange("1", "2")

	info, err = s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &state.ControllerInfo{
		CloudName:     "dummy",
		ModelTag:      s.modelTag,
		ControllerIds: []string{"0", "1", "2"},
	})
}

func (s *EnableHASuite) TestDestroyFromHA(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	err = m0.Destroy()
	c.Assert(err, tc.ErrorMatches, "controller 0 is the only controller")
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	err = m0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	c.Check(m0.Life(), tc.Equals, state.Dying)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.WantsVote(), tc.IsFalse)
}

func (s *EnableHASuite) TestForceDestroyFromHA(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, tc.ErrorIsNil)
	// ForceDestroy must be blocked if there is only 1 machine.
	err = m0.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorMatches, "controller 0 is the only controller")
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	err = m0.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	// Force remove of controller machines will first clean up units and
	// after that it will pass the machine life to Dying.
	c.Check(m0.Life(), tc.Equals, state.Alive)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Assert(node.WantsVote(), tc.IsFalse)
}

func (s *EnableHASuite) TestDestroyRaceLastController(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	for _, id := range []string{"0", "1", "2"} {
		node, err := s.State.ControllerNode(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(node.SetHasVote(true), tc.ErrorIsNil)
	}

	defer state.SetBeforeHooks(c, s.State, func() {
		// We remove the other 2 controllers just before controller "0" would be destroyed
		for _, id := range []string{"1", "2"} {
			c.Check(state.SetWantsVote(s.State, id, false), tc.ErrorIsNil)
			node, err := s.State.ControllerNode(id)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(node.SetHasVote(false), tc.ErrorIsNil)
			c.Check(node.Refresh(), tc.ErrorIsNil)
			c.Check(s.State.RemoveControllerReference(node), tc.ErrorIsNil)
			c.Logf("removed machine %s", id)
			c.Assert(m0.Refresh(), tc.ErrorIsNil)
			c.Assert(node.Refresh(), tc.ErrorIsNil)
			c.Logf("machine 0: %s wants %t has %t", m0.Life(), node.WantsVote(), node.HasVote())
		}
	}).Check()
	c.Logf("destroying machine 0")
	err = m0.Destroy()
	c.Check(err, tc.ErrorMatches, "controller 0 is the only controller")
	c.Logf("attempted to destroy machine 0 finished")
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	c.Check(m0.Life(), tc.Equals, state.Alive)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(node.HasVote(), tc.IsTrue)
	c.Check(node.WantsVote(), tc.IsTrue)
}

func (s *EnableHASuite) TestRemoveControllerMachineOneMachine(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(true), tc.ErrorIsNil)
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, tc.ErrorMatches, "controller 0 cannot be removed as it still wants to vote")
	c.Assert(state.SetWantsVote(s.State, m0.Id(), false), tc.ErrorIsNil)
	// TODO(HA) - no longer need to refresh once HasVote is moved off machine
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, tc.ErrorMatches, "controller 0 cannot be removed as it still has a vote")
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	// it seems odd that we would end up the last controller but not have a vote, but we care about the DB integrity
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, tc.ErrorMatches, "controller 0 cannot be removed as it is the last controller")
}

func (s *EnableHASuite) TestRemoveControllerMachine(c *tc.C) {
	m0, err := s.State.AddMachine(state.UbuntuBase("18.04"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(true), tc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changes.Added, tc.HasLen, 2)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	c.Assert(m0.Destroy(), tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, tc.ErrorIsNil)
	s.assertControllerInfo(c, []string{"1", "2"}, []string{"1", "2"}, nil)
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	c.Check(m0.Jobs(), tc.DeepEquals, []state.MachineJob{})
}

func (s *EnableHASuite) TestRemoveControllerMachineVoteRace(c *tc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state.SetWantsVote(s.State, m0.Id(), false), tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	// It no longer wants the vote, but does have the JobManageModel
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"1", "2"}, nil)
	defer state.SetBeforeHooks(c, s.State, func() {
		// we sneakily add the vote back to machine 1 just before it would be removed
		m0, err := s.State.Machine("0")
		c.Check(err, tc.ErrorIsNil)
		c.Check(state.SetWantsVote(s.State, m0.Id(), true), tc.ErrorIsNil)
	}).Check()
	err = s.State.RemoveControllerReference(node)
	c.Check(err, tc.ErrorMatches, "controller 0 cannot be removed as it still wants to vote")
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	c.Check(m0.Jobs(), tc.DeepEquals, []state.MachineJob{state.JobHostUnits, state.JobManageModel})
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Check(node.HasVote(), tc.IsFalse)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
}

func (s *EnableHASuite) TestRemoveControllerMachineRace(c *tc.C) {
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)
	s.assertControllerInfo(c, []string{"0", "1", "2"}, []string{"0", "1", "2"}, nil)
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state.SetWantsVote(s.State, m0.Id(), false), tc.ErrorIsNil)
	node, err := s.State.ControllerNode(m0.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	removeOne := func(id string) {
		c.Check(state.SetWantsVote(s.State, id, false), tc.ErrorIsNil)
		node, err := s.State.ControllerNode(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(s.State.RemoveControllerReference(node), tc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		// we sneakily remove machine 1 just before 0 can be removed, this causes the removal of m0 to be retried
		removeOne("1")
	}, func() {
		// then we remove machine 2, leaving 0 as the last machine, and that aborts the removal
		removeOne("2")
	}).Check()
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, tc.ErrorMatches, "controller 0 cannot be removed as it is the last controller")
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Check(node.WantsVote(), tc.IsFalse)
	c.Check(node.HasVote(), tc.IsFalse)
	c.Assert(m0.Refresh(), tc.ErrorIsNil)
	c.Check(m0.Jobs(), tc.DeepEquals, []state.MachineJob{state.JobHostUnits, state.JobManageModel})
}

func (s *EnableHASuite) TestEnableHAOpensSSHProxyPort(c *tc.C) {
	state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("20.04"), "controller",
		state.AddTestingCharmMultiSeries(c, s.State, "juju-controller"))
	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("18.04"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.HasLen, 3)

	controllerApp, err := s.State.Application(bootstrap.ControllerApplicationName)
	c.Assert(err, tc.ErrorIsNil)

	units, err := controllerApp.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 3)

	config, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	expectedPort := config.SSHServerPort()

	for _, unit := range units {
		ports, err := unit.OpenedPortRanges()
		c.Assert(err, tc.ErrorIsNil)
		openPorts := ports.UniquePortRanges()

		foundSSHProxyPort := false
		for _, portRange := range openPorts {
			if portRange.FromPort <= expectedPort && portRange.ToPort >= expectedPort {
				foundSSHProxyPort = true
				break
			}
		}

		c.Assert(foundSSHProxyPort, tc.IsTrue)
	}
}
