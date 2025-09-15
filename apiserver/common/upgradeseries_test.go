// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type upgradeSeriesSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag
	unitTag2    names.UnitTag
}

func TestUpgradeSeriesSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesSuite{})
}

func (s *upgradeSeriesSuite) SetUpTest(c *tc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
	s.unitTag2 = names.NewUnitTag("redis/1")
}

func (s *upgradeSeriesSuite) assertBackendApi(
	c *tc.C, tag names.Tag,
) (*common.UpgradeSeriesAPI, *gomock.Controller, *mocks.MockUpgradeSeriesBackend) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	ctrl := gomock.NewController(c)
	mockBackend := mocks.NewMockUpgradeSeriesBackend(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.unitTag1.Id()
		}, nil
	}

	machineAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.machineTag1.Id()
		}, nil
	}

	api := common.NewUpgradeSeriesAPI(
		mockBackend, resources, authorizer, machineAuthFunc, unitAuthFunc, loggo.GetLogger("juju.apiserver.common"))
	return api, ctrl, mockBackend
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotificationsUnitTag(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	upgradeSeriesWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	upgradeSeriesWatcher.changes <- struct{}{}

	mockMachine1 := mocks.NewMockUpgradeSeriesMachine(ctrl)
	mockUnit1 := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine1, nil)
	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit1, nil)
	mockMachine1.EXPECT().WatchUpgradeSeriesNotifications().Return(upgradeSeriesWatcher, nil)
	mockUnit1.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: names.NewUnitTag("mysql/2").String()},
		{Tag: s.unitTag1.String()},
	}}
	watches, err := api.WatchUpgradeSeriesNotifications(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "1", Error: nil},
		},
	})
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotificationsMachineTag(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.machineTag1)
	defer ctrl.Finish()

	mockMachine := mocks.NewMockUpgradeSeriesMachine(ctrl)

	upgradeSeriesWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	upgradeSeriesWatcher.changes <- struct{}{}

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine, nil)
	mockMachine.EXPECT().WatchUpgradeSeriesNotifications().Return(upgradeSeriesWatcher, nil)

	watches, err := api.WatchUpgradeSeriesNotifications(
		params.Entities{
			Entities: []params.Entity{
				{Tag: s.machineTag1.String()},
				{Tag: names.NewMachineTag("7").String()},
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusUnitTag(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareRunning, "focal", nil)
	mockUnit.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.unitTag1.String()},
				Status: model.UpgradeSeriesPrepareCompleted,
			},
			{
				Entity: params.Entity{Tag: names.NewUnitTag("mysql/2").String()},
				Status: model.UpgradeSeriesPrepareCompleted,
			},
		},
	}
	watches, err := api.SetUpgradeSeriesUnitStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusUnitTagWithInvalidStatus(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesNotStarted, "", nil)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.unitTag1.String()},
				Status: model.UpgradeSeriesPrepareCompleted,
			},
		},
	}
	watches, err := api.SetUpgradeSeriesUnitStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "upgrade series status \"prepare completed\"", Code: "bad request"}},
		},
	})
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusUnitTagWithSameStatus(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesCompleteRunning, "", nil)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.unitTag1.String()},
				Status: model.UpgradeSeriesCompleteRunning,
			},
		},
	}
	watches, err := api.SetUpgradeSeriesUnitStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusUnitTag(c *tc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareCompleted, "focal", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.unitTag1.String()},
			{Tag: names.NewUnitTag("mysql/2").String()},
		},
	}

	results, err := api.UpgradeSeriesUnitStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{
			{
				Status: model.UpgradeSeriesPrepareCompleted,
				Target: "focal",
			},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func (m *mockNotifyWatcher) Stop() error {
	m.Kill()
	return m.Wait()
}

func (m *mockNotifyWatcher) Kill() {
	m.tomb.Kill(nil)
}

func (m *mockNotifyWatcher) Wait() error {
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Err() error {
	return m.tomb.Err()
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	return m.changes
}
