// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/upgradeseries"
)

type ResolverSuite struct {
	testhelpers.IsolationSuite
}

func TestResolverSuite(t *tctesting.T) {
	tc.Run(t, &ResolverSuite{})
}

func (ResolverSuite) NewResolver() resolver.Resolver {
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	return upgradeseries.NewResolver(logger)
}

func (s ResolverSuite) TestNextOpWithValidationStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)
	res := s.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		UpgradeMachineStatus: model.UpgradeSeriesValidate,
	}, mockFactory)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
}

func (s ResolverSuite) TestNextOpWithRemoveStateCompleted(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)
	res := s.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		UpgradeMachineStatus: model.UpgradeSeriesPrepareCompleted,
	}, mockFactory)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
}

func (s ResolverSuite) TestNextOpWithPreSeriesUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewRunHook(hook.Info{Kind: hooks.PreSeriesUpgrade}).Return(mockOp, nil)

	res := s.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
	}, remotestate.Snapshot{
		UpgradeMachineStatus: model.UpgradeSeriesPrepareStarted,
	}, mockFactory)
	c.Assert(err, tc.IsNil)
	c.Assert(op, tc.NotNil)
}

func (s ResolverSuite) TestNextOpWithPostSeriesUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewRunHook(hook.Info{Kind: hooks.PostSeriesUpgrade}).Return(mockOp, nil)

	res := s.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
	}, remotestate.Snapshot{
		UpgradeMachineStatus: model.UpgradeSeriesCompleteStarted,
	}, mockFactory)
	c.Assert(err, tc.IsNil)
	c.Assert(op, tc.NotNil)
}

func (s ResolverSuite) TestNextOpWithFinishUpgradeSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewNoOpFinishUpgradeSeries().Return(mockOp, nil)

	res := s.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeMachineStatus: model.UpgradeSeriesCompleted,
	}, remotestate.Snapshot{
		UpgradeMachineStatus: model.UpgradeSeriesNotStarted,
	}, mockFactory)
	c.Assert(err, tc.IsNil)
	c.Assert(op, tc.NotNil)
}

func (s ResolverSuite) TestNextOpWithNoState(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := s.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, mockFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}
