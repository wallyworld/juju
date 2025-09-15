// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type LeaderSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	accessor *StubLeadershipSettingsAccessor
	tracker  *StubTracker
	context  context.LeadershipContext
}

func TestLeaderSuite(t *tctesting.T) {
	tc.Run(t, &LeaderSuite{})
}

func (s *LeaderSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.accessor = &StubLeadershipSettingsAccessor{
		Stub: &s.Stub,
	}
	s.tracker = &StubTracker{
		Stub:            &s.Stub,
		applicationName: "led-application",
	}
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ApplicationName",
	}}, func() {
		s.context = context.NewLeadershipContext(s.accessor, s.tracker, "u/0")
	})
}

func (s *LeaderSuite) CheckCalls(c *tc.C, stubCalls []testhelpers.StubCall, f func()) {
	s.Stub = testhelpers.Stub{}
	f()
	s.Stub.CheckCalls(c, stubCalls)
}

func (s *LeaderSuite) TestIsLeaderSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// ...and so does the second.
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailure(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// ...and the second doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailureAfterSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The second fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The third doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestLeaderSettingsSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		// The first call grabs the settings...
		s.accessor.results = []map[string]string{{
			"some": "settings",
			"of":   "interest",
		}}
		settings, err := s.context.LeaderSettings()
		c.Check(settings, tc.DeepEquals, map[string]string{
			"some": "settings",
			"of":   "interest",
		})
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The second uses the cache.
		settings, err := s.context.LeaderSettings()
		c.Check(settings, tc.DeepEquals, map[string]string{
			"some": "settings",
			"of":   "interest",
		})
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestLeaderSettingsCopyMap(c *tc.C) {
	// Grab the settings to populate the cache...
	s.accessor.results = []map[string]string{{
		"some": "settings",
		"of":   "interest",
	}}
	settings, err := s.context.LeaderSettings()
	c.Check(err, tc.IsNil)

	// Put some nonsense into the returned settings...
	settings["bad"] = "news"

	// Get the settings again and check they're as expected.
	settings, err = s.context.LeaderSettings()
	c.Check(settings, tc.DeepEquals, map[string]string{
		"some": "settings",
		"of":   "interest",
	})
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestLeaderSettingsError(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		s.accessor.results = []map[string]string{nil}
		s.Stub.SetErrors(errors.New("blort"))
		settings, err := s.context.LeaderSettings()
		c.Check(settings, tc.IsNil)
		c.Check(err, tc.ErrorMatches, "cannot read settings: blort")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		s.tracker.results = []StubTicket{true}
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsMinion(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		err := s.context.WriteLeaderSettings(map[string]string{"blah": "blah"})
		c.Check(err, tc.ErrorMatches, "cannot write settings: not the leader")
	})

	s.CheckCalls(c, nil, func() {
		// The second doesn't even try.
		err := s.context.WriteLeaderSettings(map[string]string{"blah": "blah"})
		c.Check(err, tc.ErrorMatches, "cannot write settings: not the leader")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsError(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		s.tracker.results = []StubTicket{true}
		s.Stub.SetErrors(errors.New("glurk"))
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, tc.ErrorMatches, "cannot write settings: glurk")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsClearsCache(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		// Start off by populating the cache...
		s.accessor.results = []map[string]string{{
			"some": "settings",
			"of":   "interest",
		}}
		_, err := s.context.LeaderSettings()
		c.Check(err, tc.IsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		// Write new data to the controller...
		s.tracker.results = []StubTicket{true}
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		s.accessor.results = []map[string]string{{
			"totally": "different",
			"server":  "decides",
		}}
		settings, err := s.context.LeaderSettings()
		c.Check(err, tc.IsNil)
		c.Check(settings, tc.DeepEquals, map[string]string{
			"totally": "different",
			"server":  "decides",
		})
		c.Check(err, tc.ErrorIsNil)
	})
}

type StubLeadershipSettingsAccessor struct {
	*testhelpers.Stub
	results []map[string]string
}

func (stub *StubLeadershipSettingsAccessor) Read(applicationName string) (result map[string]string, _ error) {
	stub.MethodCall(stub, "Read", applicationName)
	result, stub.results = stub.results[0], stub.results[1:]
	return result, stub.NextErr()
}

func (stub *StubLeadershipSettingsAccessor) Merge(applicationName, unitName string, settings map[string]string) error {
	stub.MethodCall(stub, "Merge", applicationName, unitName, settings)
	return stub.NextErr()
}

type StubTracker struct {
	leadership.Tracker
	*testhelpers.Stub
	applicationName string
	results         []StubTicket
}

func (stub *StubTracker) ApplicationName() string {
	stub.MethodCall(stub, "ApplicationName")
	return stub.applicationName
}

func (stub *StubTracker) ClaimLeader() (result leadership.Ticket) {
	stub.MethodCall(stub, "ClaimLeader")
	result, stub.results = stub.results[0], stub.results[1:]
	return result
}

type StubTicket bool

func (ticket StubTicket) Wait() bool {
	return bool(ticket)
}

func (ticket StubTicket) Ready() <-chan struct{} {
	return alwaysReady
}

var alwaysReady = make(chan struct{})

func init() {
	close(alwaysReady)
}
