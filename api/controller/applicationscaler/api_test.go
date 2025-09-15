// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/applicationscaler"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type APISuite struct {
	testhelpers.IsolationSuite
}

func TestAPISuite(t *tctesting.T) {
	tc.Run(t, &APISuite{})
}

func (s *APISuite) TestRescaleMethodName(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, tc.Equals, "Rescale")
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Rescale(nil)
	c.Check(called, tc.IsTrue)
}

func (s *APISuite) TestRescaleBadArgs(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		panic("should not be called")
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale([]string{"good-name", "bad/name"})
	c.Check(err, tc.ErrorMatches, `application name "bad/name" not valid`)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *APISuite) TestRescaleConvertArgs(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(_ string, arg, _ interface{}) error {
		called = true
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				"application-foo",
			}, {
				"application-bar-baz",
			}},
		})
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Rescale([]string{"foo", "bar-baz"})
	c.Check(called, tc.IsTrue)
}

func (s *APISuite) TestRescaleCallError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("snorble flip")
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, tc.ErrorMatches, "snorble flip")
}

func (s *APISuite) TestRescaleFirstError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, result interface{}) error {
		resultPtr, ok := result.(*params.ErrorResults)
		c.Assert(ok, tc.IsTrue)
		*resultPtr = params.ErrorResults{Results: []params.ErrorResult{{
			nil,
		}, {
			&params.Error{Message: "expect this error"},
		}, {
			&params.Error{Message: "not this one"},
		}, {
			nil,
		}}}
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, tc.ErrorMatches, "expect this error")
}

func (s *APISuite) TestRescaleNoError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *APISuite) TestWatchMethodName(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, tc.Equals, "Watch")
		return errors.New("irrelevant")
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Watch()
	c.Check(called, tc.IsTrue)
}

func (s *APISuite) TestWatchError(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, tc.Equals, "Watch")
		return errors.New("blam pow")
	})
	api := applicationscaler.NewAPI(caller, nil)

	watcher, err := api.Watch()
	c.Check(watcher, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "blam pow")
	c.Check(called, tc.IsTrue)
}

func (s *APISuite) TestWatchSuccess(c *tc.C) {
	expectResult := params.StringsWatchResult{
		StringsWatcherId: "123",
		Changes:          []string{"ping", "pong", "pung"},
	}
	caller := apiCaller(c, func(_ string, _, result interface{}) error {
		resultPtr, ok := result.(*params.StringsWatchResult)
		c.Assert(ok, tc.IsTrue)
		*resultPtr = expectResult
		return nil
	})
	expectWatcher := &stubWatcher{}
	newWatcher := func(gotCaller base.APICaller, gotResult params.StringsWatchResult) watcher.StringsWatcher {
		c.Check(gotCaller, tc.NotNil) // uncomparable
		c.Check(gotResult, tc.DeepEquals, expectResult)
		return expectWatcher
	}
	api := applicationscaler.NewAPI(caller, newWatcher)

	watcher, err := api.Watch()
	c.Check(watcher, tc.Equals, expectWatcher)
	c.Check(err, tc.ErrorIsNil)
}

func apiCaller(c *tc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "ApplicationScaler")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		return check(request, arg, result)
	})
}

type stubWatcher struct {
	watcher.StringsWatcher
}
