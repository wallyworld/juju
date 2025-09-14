// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

const (
	newBranchName    = "new-branch"
	newBranchCreator = "new-branch-user"
	branchCommitter  = "commit-user"
)

type generationSuite struct {
	ConnSuite

	ch *state.Charm
}

func TestGenerationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &generationSuite{})
}

func (s *generationSuite) TestBranchNameNotFound(c *tc.C) {
	_, err := s.Model.Branch("non-extant-branch")
	c.Assert(errors.IsNotFound(err), tc.IsTrue)
}

func (s *generationSuite) TestAddBranchSuccess(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	c.Assert(gen, tc.NotNil)
	c.Check(gen.ModelUUID(), tc.Equals, s.Model.UUID())
	c.Check(gen.GenerationId(), tc.Equals, 0)
	c.Check(gen.Created(), tc.Not(tc.Equals), 0)
	c.Check(gen.CreatedBy(), tc.Equals, newBranchCreator)
	c.Check(gen.BranchName(), tc.Equals, newBranchName)
	c.Check(gen.IsCompleted(), tc.IsFalse)
	c.Check(gen.CompletedBy(), tc.Equals, "")
}

func (s *generationSuite) TestAssignApplicationCompletedError(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Assert(gen.AssignApplication("redis"), tc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestAssignApplicationSuccess(c *tc.C) {
	gen := s.addBranch(c)

	c.Assert(gen.AssignApplication("redis"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.DeepEquals, map[string][]string{"redis": {}})

	// Idempotent.
	c.Assert(gen.AssignApplication("redis"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.DeepEquals, map[string][]string{"redis": {}})
}

func (s *generationSuite) TestAssignUnitBranchAbortedError(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)

	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), tc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestAssignUnitNotExistsError(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)
	c.Assert(gen.AssignUnit("redis/0"), tc.ErrorMatches, `unit "redis/0" not found`)
}

func (s *generationSuite) TestAssignUnitBranchCommittedError(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	// Make a change so that commit is a real commit with a generation ID.
	c.Assert(gen.AssignApplication("riak"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	_, err := gen.Commit(branchCommitter)

	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Assert(gen.AssignUnit("redis/0"), tc.ErrorMatches, "branch was already committed")
}

func (s *generationSuite) TestAssignUnitSuccess(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignUnit("riak/0"), tc.ErrorIsNil)

	expected := map[string][]string{"riak": {"riak/0"}}

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.DeepEquals, expected)

	// Idempotent.
	c.Assert(gen.AssignUnit("riak/0"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.DeepEquals, expected)
}

func (s *generationSuite) TestAssignAllUnitsSuccessAll(c *tc.C) {
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)

	expected := []string{"riak/0", "riak/1", "riak/2", "riak/3"}

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)
}

func (s *generationSuite) TestAssignAllUnitsSuccessRemaining(c *tc.C) {
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignUnit("riak/2"), tc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)

	expected := []string{"riak/2", "riak/3", "riak/1", "riak/0"}

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)
}

func (s *generationSuite) TestAssignNumUnitsSuccessRemaining(c *tc.C) {
	gen := s.setupAssignAllUnits(c)

	expected := []string{"riak/0", "riak/1", "riak/2", "riak/3"}

	c.Assert(gen.AssignUnits("riak", 1), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected[:1])

	c.Assert(gen.AssignUnits("riak", 2), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected[:3])

	c.Assert(gen.AssignUnits("riak", 1), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)
}

func (s *generationSuite) TestAssignUnitsNoOperations(c *tc.C) {
	gen := s.setupAssignUnits(c)

	c.Assert(gen.AssignUnits("riak", 1), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 0)
}

func (s *generationSuite) TestAssignNumUnitsSelectAll(c *tc.C) {
	gen := s.setupAssignAllUnits(c)

	expected := []string{"riak/0", "riak/1", "riak/2", "riak/3"}

	c.Assert(gen.AssignUnits("riak", 100), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)

	// Idempotent.
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, expected)
}

func (s *generationSuite) TestAssignAllUnitsCompletedError(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	// Absence of changes will result in an aborted generation.
	_, err := gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Assert(gen.AssignAllUnits("riak"), tc.ErrorMatches, "branch was already aborted")
}

func (s *generationSuite) TestCommitAssignsRemainingUnits(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	c.Assert(gen.AssignUnit("riak/0"), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)

	genId, err := gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(genId, tc.Not(tc.Equals), 0)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.IsCompleted(), tc.IsTrue)
	c.Check(gen.CompletedBy(), tc.Equals, branchCommitter)
	c.Check(gen.AssignedUnits(), tc.HasLen, 1)
	c.Check(gen.AssignedUnits()["riak"], tc.SameContents, []string{"riak/0", "riak/1", "riak/2", "riak/3"})

	// Idempotent.
	_, err = gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *generationSuite) TestCommitNoChangesEffectivelyAborted(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.addBranch(c)

	genId, err := gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(genId, tc.Equals, 0)

	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.IsCompleted(), tc.IsTrue)
	c.Check(gen.CompletedBy(), tc.Equals, branchCommitter)
}

func (s *generationSuite) TestCommitAppliesConfigDeltas(c *tc.C) {
	s.setupTestingClock(c)
	gen := s.setupAssignAllUnits(c)

	app, err := s.State.Application("riak")
	c.Assert(err, tc.ErrorIsNil)

	newCfg := map[string]interface{}{"http_port": int64(9999)}
	c.Assert(app.UpdateCharmConfig(newBranchName, newCfg), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)

	_, err = gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)

	cfg, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(cfg, tc.DeepEquals, charm.Settings(newCfg))
}

func (s *generationSuite) TestAbortSuccess(c *tc.C) {
	s.setupTestingClock(c)

	gen := s.addBranch(c)

	err := gen.Abort(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)

	// Idempotent.
	err = gen.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = gen.Abort(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *generationSuite) TestAbortSuccessApplicationNoAssignedUnits(c *tc.C) {
	s.setupTestingClock(c)

	gen := s.addBranch(c)
	err := gen.AssignApplication("riak")
	c.Assert(err, tc.ErrorIsNil)
	err = gen.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	err = gen.Abort(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *generationSuite) TestAbortFailsAssignedUnits(c *tc.C) {
	s.setupTestingClock(c)

	gen := s.setupAssignAllUnits(c)
	err := gen.AssignUnit("riak/0")
	c.Assert(err, tc.ErrorIsNil)
	err = gen.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	err = gen.Abort(branchCommitter)
	c.Assert(err, tc.ErrorMatches, "branch is in progress. Either reset values on tracking units and commit the branch or remove them to abort.")
}

func (s *generationSuite) TestAbortCommittedBranch(c *tc.C) {
	s.setupTestingClock(c)

	gen := s.setupAssignAllUnits(c)
	err := gen.AssignUnit("riak/0")
	c.Assert(err, tc.ErrorIsNil)
	err = gen.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	_, err = gen.Commit(branchCommitter)
	c.Assert(err, tc.ErrorIsNil)
	err = gen.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	err = gen.Abort(branchCommitter)
	c.Assert(err, tc.ErrorMatches, "branch was already committed")
}

func (s *generationSuite) TestBranchCharmConfigDeltas(c *tc.C) {
	gen := s.setupAssignAllUnits(c)
	c.Assert(gen.Config(), tc.HasLen, 0)

	current := state.GetPopulatedSettings(map[string]interface{}{
		"http_port":    8098,
		"handoff_port": 8099,
		"riak_version": "1.1.4-1",
	})

	// Process a modification, deletion, and addition.
	changes := charm.Settings{
		"http_port":    8100,
		"handoff_port": nil,
		"node_name":    "nodey-node",
	}
	c.Assert(gen.UpdateCharmConfig("riak", current, changes), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)
	c.Check(gen.Config(), tc.DeepEquals, map[string]settings.ItemChanges{"riak": {
		settings.MakeDeletion("handoff_port", 8099),
		settings.MakeModification("http_port", 8098, 8100),
		settings.MakeAddition("node_name", "nodey-node"),
	}})

	// Now simulate node_name being set on master in the meantime,
	// along with changes to http_port and handoff_port.
	current = state.GetPopulatedSettings(map[string]interface{}{
		"http_port":    100,
		"handoff_port": 100,
		"riak_version": "1.1.4-1",
		"node_name":    "come-lately",
	})

	// Re-set previously deleted handoff_port, update node_name.
	changes = charm.Settings{
		"handoff_port": 9000,
		"node_name":    "latest-nodey-node",
	}
	c.Assert(gen.UpdateCharmConfig("riak", current, changes), tc.ErrorIsNil)
	c.Assert(gen.Refresh(), tc.ErrorIsNil)

	// handoff_port old value is the original.
	// http_port unchanged.
	// node_name remains as an addition.
	c.Check(gen.Config(), tc.DeepEquals, map[string]settings.ItemChanges{"riak": {
		settings.MakeModification("handoff_port", 8099, 9000),
		settings.MakeModification("http_port", 8098, 8100),
		settings.MakeAddition("node_name", "latest-nodey-node"),
	}})
}

func (s *generationSuite) TestBranches(c *tc.C) {
	s.setupTestingClock(c)

	branches, err := s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 0)

	_ = s.addBranch(c)
	branches, err = s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 1)
	c.Check(branches[0].BranchName(), tc.Equals, newBranchName)

	const otherBranchName = "other-branch"
	c.Assert(s.Model.AddBranch(otherBranchName, newBranchCreator), tc.ErrorIsNil)
	branches, err = s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 2)

	// Commit the newly added branch. Branches call should not return it.
	branch, err := s.Model.Branch(otherBranchName)
	c.Assert(err, tc.ErrorIsNil)
	_, err = branch.Commit(newBranchCreator)
	c.Assert(err, tc.ErrorIsNil)

	branches, err = s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 1)
	c.Check(branches[0].BranchName(), tc.Equals, newBranchName)
}

func (s *generationSuite) TestUnitBranch(c *tc.C) {
	s.setupTestingClock(c)

	branchA := s.setupAssignAllUnits(c)
	c.Assert(branchA.AssignUnit("riak/0"), tc.ErrorIsNil)

	c.Assert(branchA.AssignUnit("riak/2"), tc.ErrorIsNil)
	c.Assert(s.Model.AddBranch("banana", newBranchCreator), tc.ErrorIsNil)
	branchB, err := s.Model.Branch("banana")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(branchB.AssignUnit("riak/1"), tc.ErrorIsNil)

	unit2Branch, err := state.UnitBranch(s.Model, "riak/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit2Branch.BranchName(), tc.Equals, branchA.BranchName())

	unit1Branch, err := state.UnitBranch(s.Model, "riak/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit1Branch.BranchName(), tc.Equals, branchB.BranchName())

	// Idempotent.
	unit2BranchTake2, err := state.UnitBranch(s.Model, "riak/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit2BranchTake2.BranchName(), tc.Equals, unit2Branch.BranchName())
}

func (s *generationSuite) TestApplicationBranches(c *tc.C) {
	s.setupTestingClock(c)

	branchA := s.setupAssignAllUnits(c)
	c.Assert(branchA.AssignUnit("riak/0"), tc.ErrorIsNil)

	appBranchesA, err := state.ApplicationBranches(s.Model, "riak")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appBranchesA, tc.HasLen, 1)
	c.Assert(appBranchesA[0].BranchName(), tc.Equals, branchA.BranchName())

	c.Assert(s.Model.AddBranch("banana", newBranchCreator), tc.ErrorIsNil)
	branchB, err := s.Model.Branch("banana")
	c.Assert(err, tc.ErrorIsNil)

	appBranchesATake2, err := state.ApplicationBranches(s.Model, "riak")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appBranchesATake2, tc.HasLen, 1)
	c.Assert(appBranchesA[0].BranchName(), tc.Equals, appBranchesATake2[0].BranchName())

	c.Assert(branchB.AssignUnit("riak/1"), tc.ErrorIsNil)

	appBranchesA, err = state.ApplicationBranches(s.Model, "riak")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appBranchesA, tc.HasLen, 2)

	// Idempotent.
	appBranchesATake2, err = state.ApplicationBranches(s.Model, "riak")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appBranchesATake2, tc.DeepEquals, appBranchesA)
}

func (s *generationSuite) TestDestroyCleansupBranches(c *tc.C) {
	s.setupTestingClock(c)

	branches, err := s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 0)

	_ = s.addBranch(c)

	branches, err = s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 1)
	c.Check(branches[0].BranchName(), tc.Equals, newBranchName)

	c.Assert(s.Model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(s.Model.Refresh(), tc.ErrorIsNil)
	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	branches, err = s.State.Branches()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(branches, tc.HasLen, 0)
}

func (s *generationSuite) setupAssignAllUnits(c *tc.C) *state.Generation {
	var cfgYAML = `
options:
  http_port: {default: 8089, description: HTTP Port, type: int}
`
	s.ch = s.AddConfigCharm(c, "riak", cfgYAML, 666)

	riak := s.AddTestingApplication(c, "riak", s.ch)
	for i := 0; i < 4; i++ {
		_, err := riak.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
	}

	return s.addBranch(c)
}

func (s *generationSuite) setupAssignUnits(c *tc.C) *state.Generation {
	var cfgYAML = `
options:
  http_port: {default: 8089, description: HTTP Port, type: int}
`
	s.ch = s.AddConfigCharm(c, "riak", cfgYAML, 666)

	s.AddTestingApplication(c, "riak", s.ch)

	return s.addBranch(c)
}

func (s *generationSuite) addBranch(c *tc.C) *state.Generation {
	c.Assert(s.Model.AddBranch(newBranchName, newBranchCreator), tc.ErrorIsNil)
	branch, err := s.Model.Branch(newBranchName)
	c.Assert(err, tc.ErrorIsNil)
	return branch
}

func (s *generationSuite) setupTestingClock(c *tc.C) {
	clock := testclock.NewClock(testing.NonZeroTime())
	clock.Advance(400000 * time.Hour)
	c.Assert(s.State.SetClockForTesting(clock), tc.ErrorIsNil)
}
