// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/upgradeseries"
	"github.com/juju/juju/api/base/mocks"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type upgradeSeriesSuite struct {
	testhelpers.IsolationSuite

	tag                                  names.Tag
	args                                 params.Entities
	upgradeSeriesStartUnitCompletionArgs params.UpgradeSeriesStartUnitCompletionParam
}

func TestUpgradeSeriesSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesSuite{})
}

func (s *upgradeSeriesSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}
	s.upgradeSeriesStartUnitCompletionArgs = params.UpgradeSeriesStartUnitCompletionParam{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	s.IsolationSuite.SetUpTest(c)
}

func (s *upgradeSeriesSuite) TestMachineStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{
			Status: model.UpgradeSeriesPrepareStarted,
		}},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, tc.IsNil)
	c.Check(status, tc.Equals, model.UpgradeSeriesPrepareStarted)
}

func (s *upgradeSeriesSuite) TestMachineStatusNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "did not find",
			},
		}},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, tc.ErrorMatches, "did not find")
	c.Check(errors.IsNotFound(err), tc.IsTrue)
	c.Check(string(status), tc.Equals, "")
}

func (s *upgradeSeriesSuite) TestSetMachineStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{Status: model.UpgradeSeriesCompleteStarted, Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("SetMachineStatus", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.SetMachineStatus(model.UpgradeSeriesCompleteStarted, "")
	c.Assert(err, tc.IsNil)
}

func (s *upgradeSeriesSuite) TestUnitsPrepared(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	r0 := names.NewUnitTag("redis/0")
	r1 := names.NewUnitTag("redis/1")

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: r0.String()},
			{Tag: r1.String()},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsPrepared", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsPrepared()
	c.Assert(err, tc.IsNil)

	expected := []names.UnitTag{r0, r1}
	c.Check(units, tc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestUnitsCompleted(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	p0 := names.NewUnitTag("postgres/0")
	p1 := names.NewUnitTag("postgres/1")
	p2 := names.NewUnitTag("postgres/2")

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: p0.String()},
			{Tag: p1.String()},
			{Tag: p2.String()},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsCompleted", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsCompleted()
	c.Assert(err, tc.IsNil)

	expected := []names.UnitTag{p0, p1, p2}
	c.Check(units, tc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestStartUnitCompletion(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("StartUnitCompletion", s.upgradeSeriesStartUnitCompletionArgs, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.StartUnitCompletion("")
	c.Assert(err, tc.IsNil)
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{
			{Channel: "16.04", Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("FinishUpgradeSeries", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.FinishUpgradeSeries(corebase.MustParseBaseFromString("ubuntu@16.04"))
	c.Assert(err, tc.IsNil)
}

func (s *upgradeSeriesSuite) TestSetStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{
				Tag:    s.tag.String(),
				Status: status.Running.String(),
				Info:   "series upgrade complete started: waiting for something",
			},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("SetInstanceStatus", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.SetInstanceStatus(model.UpgradeSeriesCompleteStarted, "waiting for something")
	c.Assert(err, tc.IsNil)
}
