// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgradeseries"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type upgradeSeriesSuite struct {
	testing.BaseSuite

	backend *mocks.MockUpgradeSeriesBackend
	machine *mocks.MockUpgradeSeriesMachine

	entityArgs                           params.Entities
	upgradeSeriesStatusArgs              params.UpgradeSeriesStatusParams
	upgradeSeriesStartUnitCompletionArgs params.UpgradeSeriesStartUnitCompletionParam

	api *upgradeseries.API

	machineTag names.MachineTag
	unitTag    names.UnitTag
}

func TestUpgradeSeriesSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesSuite{})
}

func (s *upgradeSeriesSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.unitTag = names.NewUnitTag("redis/0")

	s.entityArgs = params.Entities{Entities: []params.Entity{{Tag: s.machineTag.String()}}}
	s.upgradeSeriesStatusArgs = params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.machineTag.String()},
			},
		},
	}
	s.upgradeSeriesStartUnitCompletionArgs = params.UpgradeSeriesStartUnitCompletionParam{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	}
}

func (s *upgradeSeriesSuite) TestMachineStatus(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareCompleted, nil)

	results, err := s.api.MachineStatus(s.entityArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{Status: model.UpgradeSeriesPrepareCompleted}},
	})
}

func (s *upgradeSeriesSuite) TestSetMachineStatus(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{{Entity: entity, Status: model.UpgradeSeriesPrepareCompleted}},
	}

	results, err := s.api.SetMachineStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestCurrentSeries(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().Base().Return(state.UbuntuBase("16.04")).AnyTimes()

	api := &upgradeseries.APIv3{s.api}

	results, err := api.CurrentSeries(s.entityArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{Result: "xenial"}},
	})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesTarget(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesTarget().Return("bionic", nil)

	api := &upgradeseries.APIv3{s.api}

	results, err := api.TargetSeries(s.entityArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{Result: "bionic"}},
	})
}

func (s *upgradeSeriesSuite) TestStartUnitCompletion(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().StartUpgradeSeriesUnitCompletion(gomock.Any()).Return(nil)

	results, err := s.api.StartUnitCompletion(s.upgradeSeriesStartUnitCompletionArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestUnitsPrepared(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesUnitStatuses().Return(map[string]state.UpgradeSeriesUnitStatus{
		"redis/0": {Status: model.UpgradeSeriesPrepareCompleted},
		"redis/1": {Status: model.UpgradeSeriesPrepareStarted},
	}, nil)

	results, err := s.api.UnitsPrepared(s.entityArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{{Tag: s.unitTag.String()}}}},
	})
}

func (s *upgradeSeriesSuite) TestUnitsCompleted(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesUnitStatuses().Return(map[string]state.UpgradeSeriesUnitStatus{
		"redis/0": {Status: model.UpgradeSeriesCompleted},
		"redis/1": {Status: model.UpgradeSeriesCompleteStarted},
	}, nil)

	results, err := s.api.UnitsCompleted(s.entityArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{{Tag: s.unitTag.String()}}}},
	})
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeriesUpgraded(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	exp := s.machine.EXPECT()
	exp.Base().Return(state.UbuntuBase("22.04"))
	exp.UpdateMachineSeries(state.UbuntuBase("20.04")).Return(nil)
	exp.RemoveUpgradeSeriesLock().Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{Entity: entity, Channel: "20.04"}},
	}

	results, err := s.api.FinishUpgradeSeries(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeriesNotUpgraded(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	exp := s.machine.EXPECT()
	exp.Base().Return(state.UbuntuBase("22.04"))
	exp.RemoveUpgradeSeriesLock().Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{Entity: entity, Channel: "22.04"}},
	}

	results, err := s.api.FinishUpgradeSeries(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestSetStatus(c *tc.C) {
	defer s.arrangeTest(c).Finish()

	msg := "series upgrade: " + string(model.UpgradeSeriesPrepareStarted)

	exp := s.machine.EXPECT()
	exp.SetInstanceStatus(status.StatusInfo{
		Status:  status.Running,
		Message: msg,
	}).Return(nil)

	results, err := s.api.SetInstanceStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{
				Tag:    s.machineTag.String(),
				Status: status.Running.String(),
				Info:   msg,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) arrangeTest(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{Tag: s.machineTag}

	s.backend = mocks.NewMockUpgradeSeriesBackend(ctrl)
	s.machine = mocks.NewMockUpgradeSeriesMachine(ctrl)

	s.backend.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)

	var err error
	s.api, err = upgradeseries.NewUpgradeSeriesAPI(s.backend, resources, authorizer, nil)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}
