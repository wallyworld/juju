// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	tctesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/multiwatcher"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type WorkerSuite struct {
	statetesting.StateSuite
	logger loggo.Logger
	config multiwatcher.Config
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test")
	s.logger.SetLogLevel(loggo.TRACE)

	allWatcherBacking, err := state.NewAllWatcherBacking(s.StatePool)
	c.Assert(err, tc.ErrorIsNil)
	s.config = multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               s.logger,
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	}
}

func (s *WorkerSuite) TestConfigMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Clock not valid")
}

func (s *WorkerSuite) TestConfigMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Logger not valid")
}

func (s *WorkerSuite) TestConfigMissingBacking(c *tc.C) {
	s.config.Backing = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Backing not valid")
}

func (s *WorkerSuite) TestConfigMissingRegisterer(c *tc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing PrometheusRegisterer not valid")
}
