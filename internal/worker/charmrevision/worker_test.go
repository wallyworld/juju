// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/charmrevision"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) TestNoMoreUpdatesUntilPeriod(c *tc.C) {
	fix := newFixture(time.Minute)
	fix.cleanTest(c, func(_ worker.Worker) {
		fix.clock.Advance(time.Minute - time.Nanosecond)
		fix.waitNoCall(c)
	})
}

func (s *WorkerSuite) TestUpdatesAfterPeriod(c *tc.C) {
	fix := newFixture(time.Minute)
	fix.cleanTest(c, func(_ worker.Worker) {
		if err := fix.clock.WaitAdvance(time.Minute*2, testhelpers.LongWait, 1); err != nil {
			c.Fatal(err)
		}
		fix.waitCall(c)
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions")
}

func (s *WorkerSuite) TestDelayedUpdateError(c *tc.C) {
	fix := newFixture(time.Minute)
	fix.revisionUpdater.stub.SetErrors(
		errors.New("no more updates for you"),
	)
	fix.dirtyTest(c, func(w worker.Worker) {
		if err := fix.clock.WaitAdvance(time.Minute*2, testhelpers.LongWait, 1); err != nil {
			c.Fatal(err)
		}
		fix.waitCall(c)
		c.Check(w.Wait(), tc.ErrorMatches, "no more updates for you")
		fix.waitNoCall(c)
	})
	fix.revisionUpdater.stub.CheckCallNames(c, "UpdateLatestRevisions")
}

// workerFixture isolates a charmrevision worker for testing.
type workerFixture struct {
	revisionUpdater mockRevisionUpdater
	clock           *testclock.Clock
	period          time.Duration
}

func newFixture(period time.Duration) workerFixture {
	return workerFixture{
		revisionUpdater: newMockRevisionUpdater(),
		clock:           testclock.NewClock(coretesting.ZeroTime()),
		period:          period,
	}
}

type testFunc func(worker.Worker)

func (fix workerFixture) cleanTest(c *tc.C, test testFunc) {
	fix.runTest(c, test, true)
}

func (fix workerFixture) dirtyTest(c *tc.C, test testFunc) {
	fix.runTest(c, test, false)
}

func (fix workerFixture) runTest(c *tc.C, test testFunc, checkWaitErr bool) {
	w, err := charmrevision.NewWorker(charmrevision.Config{
		RevisionUpdater: fix.revisionUpdater,
		Clock:           fix.clock,
		Period:          fix.period,
		Logger:          loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := worker.Stop(w)
		if checkWaitErr {
			c.Check(err, tc.ErrorIsNil)
		}
	}()
	test(w)
}

func (fix workerFixture) waitCall(c *tc.C) {
	select {
	case <-fix.revisionUpdater.calls:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (fix workerFixture) waitNoCall(c *tc.C) {
	select {
	case <-fix.revisionUpdater.calls:
		c.Fatalf("unexpected revisionUpdater call")
	case <-time.After(coretesting.ShortWait):
	}
}

// mockRevisionUpdater records (and notifies of) calls made to UpdateLatestRevisions.
type mockRevisionUpdater struct {
	stub  *testhelpers.Stub
	calls chan struct{}
}

func newMockRevisionUpdater() mockRevisionUpdater {
	return mockRevisionUpdater{
		stub:  &testhelpers.Stub{},
		calls: make(chan struct{}, 1000),
	}
}

func (mock mockRevisionUpdater) UpdateLatestRevisions() error {
	mock.stub.AddCall("UpdateLatestRevisions")
	mock.calls <- struct{}{}
	return mock.stub.NextErr()
}
