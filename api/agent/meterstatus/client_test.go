// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/meterstatus"
	"github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type meterStatusSuite struct {
	coretesting.BaseSuite
}

func TestMeterStatusSuite(t *tctesting.T) {
	tc.Run(t, &meterStatusSuite{})
}

func (s *meterStatusSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *meterStatusSuite) TestGetMeterStatus(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = []params.MeterStatusResult{{
			Code: "GREEN",
			Info: "All ok.",
		}}
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusCode, tc.Equals, "GREEN")
	c.Assert(statusInfo, tc.Equals, "All ok.")
}

func (s *meterStatusSuite) TestGetMeterStatusNotImplemented(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})

	tag := names.NewUnitTag("wp/1")
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)

	_, _, err := status.MeterStatus()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *meterStatusSuite) TestGetMeterStatusResultError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = []params.MeterStatusResult{{
			Error: &params.Error{
				Message: "An error in the meter status.",
				Code:    params.CodeNotAssigned,
			},
		}}
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "An error in the meter status.")
	c.Assert(statusCode, tc.Equals, "")
	c.Assert(statusInfo, tc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusReturnsError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.MeterStatusResults{})
		called = true
		return fmt.Errorf("could not retrieve meter status")
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)

	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "could not retrieve meter status")
	c.Assert(statusCode, tc.Equals, "")
	c.Assert(statusInfo, tc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusMoreResults(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.MeterStatusResults{})
		result := response.(*params.MeterStatusResults)
		result.Results = make([]params.MeterStatusResult, 2)
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)
	statusCode, statusInfo, err := status.MeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(statusCode, tc.Equals, "")
	c.Assert(statusInfo, tc.Equals, "")
}

func (s *meterStatusSuite) TestWatchMeterStatusError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 1)
		called = true
		return fmt.Errorf("could not retrieve meter status watcher")
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "could not retrieve meter status watcher")
	c.Assert(w, tc.IsNil)
}

func (s *meterStatusSuite) TestWatchMeterStatusNotImplemented(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})

	tag := names.NewUnitTag("wp/1")
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)

	_, err := status.WatchMeterStatus()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *meterStatusSuite) TestWatchMeterStatusMoreResults(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = make([]params.NotifyWatchResult, 2)
		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, tc.IsNil)
}

func (s *meterStatusSuite) TestWatchMeterStatusResultError(c *tc.C) {
	tag := names.NewUnitTag("wp/1")
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, response interface{}) error {
		c.Check(objType, tc.Equals, "MeterStatus")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchMeterStatus")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: tag.String()}},
		})
		c.Assert(response, tc.FitsTypeOf, &params.NotifyWatchResults{})
		result := response.(*params.NotifyWatchResults)
		result.Results = []params.NotifyWatchResult{{
			Error: &params.Error{
				Message: "error",
				Code:    params.CodeNotAssigned,
			},
		}}

		called = true
		return nil
	})
	status := meterstatus.NewClient(apiCaller, tag)
	c.Assert(status, tc.NotNil)
	w, err := status.WatchMeterStatus()
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "error")
	c.Assert(w, tc.IsNil)
}
