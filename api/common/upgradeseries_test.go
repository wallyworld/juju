// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type upgradeSeriesSuite struct {
	testhelpers.IsolationSuite
	tag names.Tag
}

func TestUpgradeSeriesSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesSuite{})
}

func (s *upgradeSeriesSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("0")
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotifications(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "WatchUpgradeSeriesNotifications")
		c.Assert(args, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
				Error:           nil,
			}},
		}
		return nil
	}
	apiCaller := apitesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, tc.Equals, "NotifyWatcher")
			c.Check(id, tc.Equals, "1")
			c.Check(request, tc.Equals, "Next")
			c.Check(a, tc.IsNil)
			return nil
		},
	)
	facadeCaller.ReturnRawAPICaller = apitesting.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}

	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	_, err := api.WatchUpgradeSeriesNotifications()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusWithComplete(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{
				Status: model.UpgradeSeriesCompleted,
				Target: "focal",
			}},
		}
		return nil
	}

	sts, target, err := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag).UpgradeSeriesUnitStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.Equals, model.UpgradeSeriesCompleted)
	c.Check(target, tc.Equals, "focal")
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusNotFound(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{
				Error: &params.Error{
					Code:    params.CodeNotFound,
					Message: `testing`,
				},
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	_, _, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, tc.ErrorMatches, "testing")
	c.Check(errors.IsNotFound(err), tc.IsTrue)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusMultiple(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{
				{Status: "prepare started"},
				{Status: "prepare completed"},
			},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	_, _, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatus(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusNotOne(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusResultError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, tc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "error in call"},
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, tc.ErrorMatches, "error in call")
}
