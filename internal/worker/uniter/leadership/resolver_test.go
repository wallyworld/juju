// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/leadership"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

func TestResolverSuite(t *tctesting.T) {
	tc.Run(t, &resolverSuite{})
}

type resolverSuite struct {
	coretesting.BaseSuite
}

func (s *resolverSuite) TestNextOpNotInstalled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	logger := loggo.GetLogger("test")

	r := leadership.NewResolver(logger)
	_, err := r.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, f)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestNextOpAcceptLeader(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggo.GetLogger("test")

	f.EXPECT().NewAcceptLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(resolver.LocalState{
		State: operation.State{Installed: true, Kind: operation.Continue},
	}, remotestate.Snapshot{
		Leader: true,
	}, f)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, op)
}

func (s *resolverSuite) TestNextOpResignLeader(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggo.GetLogger("test")

	f.EXPECT().NewResignLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(resolver.LocalState{
		State: operation.State{Installed: true, Leader: true, Kind: operation.Continue},
	}, remotestate.Snapshot{}, f)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, op)
}

func (s *resolverSuite) TestNextOpResignLeaderDying(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggo.GetLogger("test")

	f.EXPECT().NewResignLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(resolver.LocalState{
		State: operation.State{Installed: true, Leader: true, Kind: operation.Continue},
	}, remotestate.Snapshot{
		Leader: true, Life: life.Dying,
	}, f)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, op)
}

func (s *resolverSuite) TestNextOpLeaderSettings(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggo.GetLogger("test")

	f.EXPECT().NewRunHook(hook.Info{Kind: hooks.LeaderSettingsChanged}).Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(resolver.LocalState{
		State:                 operation.State{Installed: true, Kind: operation.Continue},
		LeaderSettingsVersion: 1,
	}, remotestate.Snapshot{LeaderSettingsVersion: 2}, f)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, op)
}

func (s *resolverSuite) TestNextOpNoLeaderSettingsWhenDying(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	logger := loggo.GetLogger("test")

	r := leadership.NewResolver(logger)
	_, err := r.NextOp(resolver.LocalState{
		State:                 operation.State{Installed: true, Kind: operation.Continue},
		LeaderSettingsVersion: 1,
	}, remotestate.Snapshot{Life: life.Dying, LeaderSettingsVersion: 2}, f)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}
