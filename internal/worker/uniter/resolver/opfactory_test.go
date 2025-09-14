// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type ResolverOpFactorySuite struct {
	testing.BaseSuite
	opFactory *mockOpFactory
}

func TestResolverOpFactorySuite(t *tctesting.T) {
	tc.Run(t, &ResolverOpFactorySuite{})
}

func (s *ResolverOpFactorySuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.opFactory = &mockOpFactory{}
}

func (s *ResolverOpFactorySuite) TestInitialState(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	c.Assert(f.LocalState, tc.DeepEquals, &resolver.LocalState{})
	c.Assert(f.RemoteState, tc.DeepEquals, remotestate.Snapshot{})
}

func (s *ResolverOpFactorySuite) TestUpdateStatusChanged(c *tc.C) {
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testUpdateStatusChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) testUpdateStatusChanged(
	c *tc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.UpdateStatusVersion = 1

	op, err := f.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	c.Assert(err, tc.ErrorIsNil)
	f.RemoteState.UpdateStatusVersion = 2

	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, tc.Equals, 1)
}

func (s *ResolverOpFactorySuite) TestConfigChanged(c *tc.C) {
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testConfigChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) TestUpgradeSeriesStatusChanged(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)

	// The initial state.
	f.LocalState.UpgradeMachineStatus = model.UpgradeSeriesNotStarted
	f.RemoteState.UpgradeMachineStatus = model.UpgradeSeriesPrepareStarted

	op, err := f.NewRunHook(hook.Info{Kind: hooks.PreSeriesUpgrade})
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.LocalState.UpgradeMachineStatus, tc.Equals, model.UpgradeSeriesPrepareStarted)
	f.RemoteState.UpgradeMachineStatus = model.UpgradeSeriesPrepareCompleted

	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.LocalState.UpgradeMachineStatus, tc.Equals, model.UpgradeSeriesPrepareCompleted)
}

func (s *ResolverOpFactorySuite) TestNewHookError(c *tc.C) {
	s.opFactory.SetErrors(
		errors.New("NewRunHook fails"),
		errors.New("NewSkipHook fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorMatches, "NewRunHook fails")
	_, err = f.NewSkipHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorMatches, "NewSkipHook fails")
}

func (s *ResolverOpFactorySuite) testConfigChanged(
	c *tc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ConfigHash = "confighash"
	f.RemoteState.TrustHash = "trusthash"
	f.RemoteState.AddressesHash = "addresseshash"
	f.RemoteState.UpdateStatusVersion = 3

	op, err := f.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)
	f.RemoteState.ConfigHash = "newhash"
	f.RemoteState.TrustHash = "badhash"
	f.RemoteState.AddressesHash = "differenthash"
	f.RemoteState.UpdateStatusVersion = 4

	resultState, err := op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultState, tc.NotNil)

	// Local state's UpdateStatusVersion should be set to what
	// RemoteState's UpdateStatusVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.UpdateStatusVersion, tc.Equals, 3)
	// The hashes need to be set on the result state, because that is
	// written to disk by the executor before the next step is picked.
	c.Assert(resultState.ConfigHash, tc.Equals, "confighash")
	c.Assert(resultState.TrustHash, tc.Equals, "trusthash")
	c.Assert(resultState.AddressesHash, tc.Equals, "addresseshash")
}

func (s *ResolverOpFactorySuite) TestLeaderSettingsChanged(c *tc.C) {
	s.testLeaderSettingsChanged(c, resolver.ResolverOpFactory.NewRunHook)
	s.testLeaderSettingsChanged(c, resolver.ResolverOpFactory.NewSkipHook)
}

func (s *ResolverOpFactorySuite) testLeaderSettingsChanged(
	c *tc.C, meth func(resolver.ResolverOpFactory, hook.Info) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.LeaderSettingsVersion = 1
	f.RemoteState.UpdateStatusVersion = 3

	op, err := meth(f, hook.Info{Kind: hooks.LeaderSettingsChanged})
	c.Assert(err, tc.ErrorIsNil)
	f.RemoteState.LeaderSettingsVersion = 2
	f.RemoteState.UpdateStatusVersion = 4

	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	// Local state's LeaderSettingsVersion should be set to what
	// RemoteState's LeaderSettingsVersion was when the operation
	// was constructed.
	c.Assert(f.LocalState.LeaderSettingsVersion, tc.Equals, 1)
	c.Assert(f.LocalState.UpdateStatusVersion, tc.Equals, 3)
}

func (s *ResolverOpFactorySuite) TestUpgrade(c *tc.C) {
	s.testUpgrade(c, resolver.ResolverOpFactory.NewUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewRevertUpgrade)
	s.testUpgrade(c, resolver.ResolverOpFactory.NewResolvedUpgrade)
}

func (s *ResolverOpFactorySuite) testUpgrade(
	c *tc.C, meth func(resolver.ResolverOpFactory, string) (operation.Operation, error),
) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.Conflicted = true
	curl := "ch:trusty/mysql"
	op, err := meth(f, curl)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CharmURL, tc.DeepEquals, curl)
	c.Assert(f.LocalState.Conflicted, tc.IsFalse)
}

func (s *ResolverOpFactorySuite) TestRemoteInit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.OutdatedRemoteCharm = true
	op, err := f.NewRemoteInit(remotestate.ContainerRunningStatus{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.OutdatedRemoteCharm, tc.IsFalse)
}

func (s *ResolverOpFactorySuite) TestSkipRemoteInit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.LocalState.OutdatedRemoteCharm = true
	op, err := f.NewSkipRemoteInit(false)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.OutdatedRemoteCharm, tc.IsTrue)
}

func (s *ResolverOpFactorySuite) TestNewUpgradeError(c *tc.C) {
	curl := "ch:trusty/mysql"
	s.opFactory.SetErrors(
		errors.New("NewUpgrade fails"),
		errors.New("NewRevertUpgrade fails"),
		errors.New("NewResolvedUpgrade fails"),
	)
	f := resolver.NewResolverOpFactory(s.opFactory)
	_, err := f.NewUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewUpgrade fails")
	_, err = f.NewRevertUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewRevertUpgrade fails")
	_, err = f.NewResolvedUpgrade(curl)
	c.Assert(err, tc.ErrorMatches, "NewResolvedUpgrade fails")
}

func (s *ResolverOpFactorySuite) TestCommitError(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)

	s.opFactory.op.commit = func(operation.State) (*operation.State, error) {
		return nil, errors.New("commit fails")
	}
	op, err := f.NewUpgrade("ch:trusty/mysql")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorMatches, "commit fails")
	// Local state should not have been updated. We use the same code
	// internally for all operations, so it suffices to test just the
	// upgrade case.
	c.Assert(f.LocalState.CharmURL, tc.Equals, "")
}

func (s *ResolverOpFactorySuite) TestActionsCommit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"action 1", "action 2", "action 3"}
	f.LocalState.CompletedActions = map[string]struct{}{}
	op, err := f.NewAction("action 1")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"action 1": {},
	})
}

func (s *ResolverOpFactorySuite) TestActionsTrimming(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"c", "d"}
	f.LocalState.CompletedActions = map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
	}
	op, err := f.NewAction("d")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"c": {},
		"d": {},
	})
}

func (s *ResolverOpFactorySuite) TestFailActionsCommit(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"action 1", "action 2", "action 3"}
	f.LocalState.CompletedActions = map[string]struct{}{}
	op, err := f.NewFailAction("action 1")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"action 1": {},
	})
}

func (s *ResolverOpFactorySuite) TestFailActionsTrimming(c *tc.C) {
	f := resolver.NewResolverOpFactory(s.opFactory)
	f.RemoteState.ActionsPending = []string{"c", "d"}
	f.LocalState.CompletedActions = map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
	}
	op, err := f.NewFailAction("d")
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.LocalState.CompletedActions, tc.DeepEquals, map[string]struct{}{
		"c": {},
		"d": {},
	})
}
