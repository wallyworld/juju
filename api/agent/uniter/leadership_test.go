// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type leadershipSuite struct {
	testhelpers.IsolationSuite
	stub       *testhelpers.Stub
	responders []responder
	lsa        *uniter.LeadershipSettingsAccessor
}

func TestLeadershipSuite(t *tctesting.T) {
	tc.Run(t, &leadershipSuite{})
}

type responder func(interface{})

var mockWatcher = struct{ watcher.NotifyWatcher }{}

func (s *leadershipSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.lsa = uniter.NewLeadershipSettingsAccessor(
		func(request string, params, response interface{}) error {
			s.stub.AddCall("FacadeCall", request, params)
			s.nextResponse(response)
			return s.stub.NextErr()
		},
		func(result params.NotifyWatchResult) watcher.NotifyWatcher {
			s.stub.AddCall("NewNotifyWatcher", result)
			return mockWatcher
		},
	)
}

func (s *leadershipSuite) nextResponse(response interface{}) {
	var responder responder
	responder, s.responders = s.responders[0], s.responders[1:]
	if responder != nil {
		responder(response)
	}
}

func (s *leadershipSuite) addResponder(responder responder) {
	s.responders = append(s.responders, responder)
}

func (s *leadershipSuite) CheckCalls(c *tc.C, calls []testhelpers.StubCall, f func()) {
	s.stub = &testhelpers.Stub{}
	s.responders = nil
	f()
	s.stub.CheckCalls(c, calls)
}

func (s *leadershipSuite) expectReadCalls() []testhelpers.StubCall {
	return []testhelpers.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"Read",
			params.Entities{Entities: []params.Entity{{
				Tag: "application-foobar",
			}}},
		},
	}}
}

func (s *leadershipSuite) TestReadSuccess(c *tc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.GetLeadershipSettingsBulkResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.GetLeadershipSettingsResult{{
				Settings: params.Settings{
					"foo": "bar",
					"baz": "qux",
				},
			}}
		})
		settings, err := s.lsa.Read("foobar")
		c.Check(err, tc.ErrorIsNil)
		c.Check(settings, tc.DeepEquals, map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
	})
}

func (s *leadershipSuite) TestReadFailure(c *tc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.GetLeadershipSettingsBulkResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.GetLeadershipSettingsResult{{
				Error: &params.Error{Message: "pow"},
			}}
		})
		settings, err := s.lsa.Read("foobar")
		c.Check(err, tc.ErrorMatches, "failed to read leadership settings: pow")
		c.Check(settings, tc.IsNil)
	})
}

func (s *leadershipSuite) TestReadError(c *tc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(errors.New("blart"))
		settings, err := s.lsa.Read("foobar")
		c.Check(err, tc.ErrorMatches, "failed to call leadership api: blart")
		c.Check(settings, tc.IsNil)
	})
}

func (s *leadershipSuite) TestReadNoResults(c *tc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(nil)
		settings, err := s.lsa.Read("foobar")
		c.Check(err, tc.ErrorMatches, "expected 1 result from leadership api, got 0")
		c.Check(settings, tc.IsNil)
	})
}

func (s *leadershipSuite) expectMergeCalls() []testhelpers.StubCall {
	return []testhelpers.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"Merge",
			params.MergeLeadershipSettingsBulkParams{
				Params: []params.MergeLeadershipSettingsParam{{
					ApplicationTag: "application-foobar",
					UnitTag:        "unit-foobar-0",
					Settings: map[string]string{
						"foo": "bar",
						"baz": "qux",
					},
				}},
			},
		},
	}}
}

func (s *leadershipSuite) TestMergeSuccess(c *tc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.ErrorResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.ErrorResult{{
				Error: nil,
			}}
		})
		err := s.lsa.Merge("foobar", "foobar/0", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *leadershipSuite) TestMergeFailure(c *tc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.ErrorResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.ErrorResult{{
				Error: &params.Error{Message: "zap"},
			}}
		})
		err := s.lsa.Merge("foobar", "foobar/0", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, tc.ErrorMatches, "failed to merge leadership settings: zap")
	})
}

func (s *leadershipSuite) TestMergeError(c *tc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(errors.New("dink"))
		err := s.lsa.Merge("foobar", "foobar/0", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, tc.ErrorMatches, "failed to call leadership api: dink")
	})
}

func (s *leadershipSuite) TestMergeNoResults(c *tc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(nil)
		err := s.lsa.Merge("foobar", "foobar/0", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, tc.ErrorMatches, "expected 1 result from leadership api, got 0")
	})
}

func (s *leadershipSuite) expectWatchCalls() []testhelpers.StubCall {
	return []testhelpers.StubCall{{
		FuncName: "FacadeCall",
		Args: []interface{}{
			"WatchLeadershipSettings",
			params.Entities{Entities: []params.Entity{{
				Tag: "application-foobar",
			}}},
		},
	}}
}

func (s *leadershipSuite) TestWatchSuccess(c *tc.C) {
	expectCalls := append(s.expectWatchCalls(), testhelpers.StubCall{
		FuncName: "NewNotifyWatcher",
		Args: []interface{}{
			params.NotifyWatchResult{
				NotifyWatcherId: "123",
			},
		},
	})
	s.CheckCalls(c, expectCalls, func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.NotifyWatchResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.NotifyWatchResult{{
				NotifyWatcherId: "123",
			}}
		})
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, tc.ErrorIsNil)
		c.Check(watcher, tc.Equals, mockWatcher)
	})
}

func (s *leadershipSuite) TestWatchFailure(c *tc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.NotifyWatchResults)
			c.Assert(ok, tc.IsTrue)
			typed.Results = []params.NotifyWatchResult{{
				Error: &params.Error{Message: "blah"},
			}}
		})
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, tc.ErrorMatches, "failed to watch leadership settings: blah")
		c.Check(watcher, tc.IsNil)
	})
}

func (s *leadershipSuite) TestWatchError(c *tc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(errors.New("snerk"))
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, tc.ErrorMatches, "failed to call leadership api: snerk")
		c.Check(watcher, tc.IsNil)
	})
}

func (s *leadershipSuite) TestWatchNoResults(c *tc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(nil)
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, tc.ErrorMatches, "expected 1 result from leadership api, got 0")
		c.Check(watcher, tc.IsNil)
	})
}
