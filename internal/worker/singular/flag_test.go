// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/singular"
)

type FlagSuite struct {
	testhelpers.IsolationSuite
}

func TestFlagSuite(t *tctesting.T) {
	tc.Run(t, &FlagSuite{})
}

func (s *FlagSuite) TestClaimError(c *tc.C) {
	var stub testhelpers.Stub
	stub.SetErrors(errors.New("squish"))

	worker, err := singular.NewFlagWorker(singular.FlagConfig{
		Facade:   newStubFacade(&stub),
		Clock:    &fakeClock{},
		Duration: time.Hour,
	})
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "squish")
}

func (s *FlagSuite) TestClaimFailure(c *tc.C) {
	fix := newFixture(c, errClaimDenied, nil)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		c.Check(flag.Check(), tc.IsFalse)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimFailureWaitError(c *tc.C) {
	fix := newFixture(c, errClaimDenied, errors.New("glug"))
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, unblock func()) {
		c.Check(flag.Check(), tc.IsFalse)
		unblock()
		err := workertest.CheckKilled(c, flag)
		c.Check(err, tc.ErrorMatches, "glug")
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimFailureWaitSuccess(c *tc.C) {
	fix := newFixture(c, errClaimDenied, nil)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, unblock func()) {
		c.Check(flag.Check(), tc.IsFalse)
		unblock()
		err := workertest.CheckKilled(c, flag)
		c.Check(errors.Cause(err), tc.Equals, singular.ErrRefresh)
	})
	fix.CheckClaimWait(c)
}

func (s *FlagSuite) TestClaimSuccess(c *tc.C) {
	fix := newFixture(c, nil, errors.New("should not happen"))
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(29 * time.Second)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaims(c, 1)
}

func (s *FlagSuite) TestClaimSuccessThenFailure(c *tc.C) {
	fix := newFixture(c, nil, errClaimDenied)
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(30 * time.Second)
		err := workertest.CheckKilled(c, flag)
		c.Check(errors.Cause(err), tc.Equals, singular.ErrRefresh)
	})
	fix.CheckClaims(c, 2)
}

func (s *FlagSuite) TestClaimSuccessesThenError(c *tc.C) {
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, clock *testclock.Clock, unblock func()) {
		<-clock.Alarms()
		clock.Advance(time.Minute)
		<-clock.Alarms()
		clock.Advance(time.Minute)
		workertest.CheckAlive(c, flag)
	})
	fix.CheckClaims(c, 3)
}
