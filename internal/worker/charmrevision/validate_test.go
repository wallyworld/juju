// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/charmrevision"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
	config charmrevision.Config
}

func TestValidateSuite(t *tctesting.T) {
	tc.Run(t, &ValidateSuite{})
}

func (s *ValidateSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = charmrevision.Config{
		RevisionUpdater: struct{ charmrevision.RevisionUpdater }{},
		Clock:           struct{ clock.Clock }{},
		Period:          time.Hour,
		Logger:          loggertesting.WrapCheckLog(c),
	}
}

func (s *ValidateSuite) TestValid(c *tc.C) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (s *ValidateSuite) TestNilRevisionUpdater(c *tc.C) {
	s.config.RevisionUpdater = nil
	s.checkNotValid(c, "nil RevisionUpdater not valid")
}

func (s *ValidateSuite) TestNilClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ValidateSuite) TestNilLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ValidateSuite) TestBadPeriods(c *tc.C) {
	for i, period := range []time.Duration{
		0, -time.Nanosecond, -time.Hour,
	} {
		c.Logf("test %d", i)
		s.config.Period = period
		s.checkNotValid(c, "non-positive Period not valid")
	}
}

func (s *ValidateSuite) checkNotValid(c *tc.C, match string) {
	check := func(err error) {
		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, match)
	}
	err := s.config.Validate()
	check(err)

	worker, err := charmrevision.NewWorker(s.config)
	c.Check(worker, tc.IsNil)
	check(err)
}
