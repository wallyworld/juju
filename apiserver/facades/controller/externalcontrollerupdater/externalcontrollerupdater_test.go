// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCrossControllerSuite(t *tctesting.T) {
	tc.Run(t, &CrossControllerSuite{})
}

type CrossControllerSuite struct {
	coretesting.BaseSuite

	watcher             *mockStringsWatcher
	externalControllers *mockExternalControllers
	resources           *common.Resources
	auth                testing.FakeAuthorizer
	api                 *externalcontrollerupdater.ExternalControllerUpdaterAPI
}

func (s *CrossControllerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = testing.FakeAuthorizer{Controller: true}
	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })
	s.watcher = newMockStringsWatcher()
	s.AddCleanup(func(*tc.C) { s.watcher.Stop() })
	s.externalControllers = &mockExternalControllers{
		watcher: s.watcher,
	}
	api, err := externalcontrollerupdater.NewAPI(s.auth, s.resources, s.externalControllers)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
}

func (s *CrossControllerSuite) TestNewAPINonController(c *tc.C) {
	s.auth.Controller = false
	_, err := externalcontrollerupdater.NewAPI(s.auth, s.resources, s.externalControllers)
	c.Assert(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *CrossControllerSuite) TestExternalControllerInfo(c *tc.C) {
	s.externalControllers.controllers = append(s.externalControllers.controllers, &mockExternalController{
		id: coretesting.ControllerTag.Id(),
		info: crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
			Alias:         "foo",
			Addrs:         []string{"bar"},
			CACert:        "baz",
		},
	})

	results, err := s.api.ExternalControllerInfo(params.Entities{
		Entities: []params.Entity{
			{coretesting.ControllerTag.String()},
			{"controller-" + coretesting.ModelTag.Id()},
			{"machine-42"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ExternalControllerInfoResults{
		[]params.ExternalControllerInfoResult{{
			Result: &params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			Error: &params.Error{
				Code:    "not found",
				Message: `external controller "deadbeef-0bad-400d-8000-4b1d0d06f00d" not found`,
			},
		}, {
			Error: &params.Error{Message: `"machine-42" is not a valid controller tag`},
		}},
	})
}

func (s *CrossControllerSuite) TestSetExternalControllerInfo(c *tc.C) {
	s.externalControllers.controllers = append(s.externalControllers.controllers, &mockExternalController{
		id: coretesting.ControllerTag.Id(),
		info: crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
		},
	})

	results, err := s.api.SetExternalControllerInfo(params.SetExternalControllersInfoParams{
		[]params.SetExternalControllerInfoParams{{
			params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: "controller-" + coretesting.ModelTag.Id(),
				Alias:         "qux",
				Addrs:         []string{"quux"},
				CACert:        "quuz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: "machine-42",
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		[]params.ErrorResult{
			{nil},
			{nil},
			{Error: &params.Error{Message: `"machine-42" is not a valid controller tag`}},
		},
	})

	c.Assert(
		s.externalControllers.controllers,
		tc.DeepEquals,
		[]*mockExternalController{{
			id: coretesting.ControllerTag.Id(),
			info: crossmodel.ControllerInfo{
				ControllerTag: coretesting.ControllerTag,
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			id: coretesting.ModelTag.Id(),
			info: crossmodel.ControllerInfo{
				ControllerTag: names.NewControllerTag(coretesting.ModelTag.Id()),
				Alias:         "qux",
				Addrs:         []string{"quux"},
				CACert:        "quuz",
			},
		}},
	)
}

func (s *CrossControllerSuite) TestWatchExternalControllers(c *tc.C) {
	s.watcher.changes <- []string{"a", "b"} // initial value
	results, err := s.api.WatchExternalControllers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"a", "b"},
		}},
	})
	c.Assert(s.resources.Get("1"), tc.Equals, s.watcher)
}

func (s *CrossControllerSuite) TestWatchControllerInfoError(c *tc.C) {
	s.watcher.tomb.Kill(errors.New("nope"))
	close(s.watcher.changes)

	results, err := s.api.WatchExternalControllers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
	c.Assert(s.resources.Get("1"), tc.IsNil)
}
