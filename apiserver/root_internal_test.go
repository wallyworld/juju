// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	statetesting "github.com/juju/juju/state/testing"
)

type facadeContextSuite struct {
	statetesting.StateSuite

	changes    chan interface{}
	handled    chan interface{}
	controller *cache.Controller
	clock      *testclock.Clock
}

func TestFacadeContextSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &facadeContextSuite{})
}

func (s *facadeContextSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)

	s.changes = make(chan interface{})
	s.handled = make(chan interface{})

	controller, err := cache.NewController(cache.ControllerConfig{
		Changes: s.changes,
		Notify: func(e interface{}) {
			s.handled <- e
		}})
	c.Assert(err, tc.ErrorIsNil)
	s.controller = controller
	s.clock = testclock.NewClock(time.Now())
}

func (s *facadeContextSuite) newContext() *facadeContext {
	// This is a bare minimum facade context for these tests.
	return &facadeContext{
		r: &apiRoot{
			clock: s.clock,
			shared: &sharedServerContext{
				controller: s.controller,
				logger:     loggo.GetLogger("test"),
			},
			state: s.State,
		},
	}
}

func (s *facadeContextSuite) processChange(c *tc.C, change interface{}) {
	select {
	case s.changes <- change:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("controller did not read change")
	}
	select {
	case obtained := <-s.handled:
		c.Check(obtained, tc.DeepEquals, change)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("controller did not handle change")
	}
}

func (s *facadeContextSuite) TestCachedModelValid(c *tc.C) {
	// Populate the cache with the model we are looking for.
	s.processChange(c, cache.ModelChange{
		ModelUUID: "some-uuid",
	})
	// We don't need to advance the clock to get the model
	// as it is already in the cache.
	ctx := s.newContext()
	model, err := ctx.CachedModel("some-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.UUID(), tc.Equals, "some-uuid")
}

func (s *facadeContextSuite) TestCachedModelMissing(c *tc.C) {
	ctx := s.newContext()
	done := make(chan interface{})
	go func() {
		defer close(done)
		model, err := ctx.CachedModel("some-uuid")
		c.Check(err, tc.Satisfies, errors.IsNotFound)
		c.Check(model, tc.IsNil)
	}()

	s.clock.WaitAdvance(10*time.Second, testhelpers.LongWait, 1)
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Error("CachedModel didn't return")
	}
}

func (s *facadeContextSuite) TestCachedModelTimeout(c *tc.C) {
	// Make a model in the DB, but don't tell the cache about it.
	state := s.Factory.MakeModel(c, nil)
	defer state.Close()

	ctx := s.newContext()
	done := make(chan interface{})
	go func() {
		defer close(done)
		model, err := ctx.CachedModel(state.ModelUUID())
		c.Check(err, tc.Satisfies, errors.IsTimeout)
		c.Check(model, tc.IsNil)
	}()

	s.clock.WaitAdvance(10*time.Second, testhelpers.LongWait, 1)
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Error("CachedModel didn't return")
	}
}
