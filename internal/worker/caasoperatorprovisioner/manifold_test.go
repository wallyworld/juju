// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasoperatorprovisioner"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config caasoperatorprovisioner.ManifoldConfig
}

func TestManifoldConfigSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() caasoperatorprovisioner.ManifoldConfig {
	return caasoperatorprovisioner.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		BrokerName:    "broker",
		ClockName:     "clock",
		NewWorker: func(config caasoperatorprovisioner.Config) (worker.Worker, error) {
			return nil, nil
		},
		Logger: loggo.GetLogger("test"),
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAgentName(c *tc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingBrokerName(c *tc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingClockName(c *tc.C) {
	s.config.ClockName = ""
	s.checkNotValid(c, "empty ClockName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}
