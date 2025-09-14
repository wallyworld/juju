// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/worker/caasunitsmanager"
	"github.com/juju/juju/internal/worker/caasunitsmanager/mocks"
)

type manifoldSuite struct {
	config caasunitsmanager.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.config = caasunitsmanager.ManifoldConfig{
		Clock:  testclock.NewClock(time.Now()),
		Logger: loggo.GetLogger("test"),
		Hub:    mocks.NewMockHub(gomock.NewController(c)),
	}
}

func (s *manifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *manifoldSuite) TestConfigValidationMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Clock not valid")
}

func (s *manifoldSuite) TestConfigValidationMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Logger not valid")
}

func (s *manifoldSuite) TestConfigValidationMissingHub(c *tc.C) {
	s.config.Hub = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Hub not valid")
}
