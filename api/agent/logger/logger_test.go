// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type loggerSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	logger *logger.State
}

func TestLoggerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &loggerSuite{})
}

func (s *loggerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var stateAPI api.Connection
	stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	// Create the logger facade.
	s.logger = logger.NewState(stateAPI)
	c.Assert(s.logger, tc.NotNil)
}

func (s *loggerSuite) TestLoggingConfigWrongMachine(c *tc.C) {
	config, err := s.logger.LoggingConfig(names.NewMachineTag("42"))
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(config, tc.Equals, "")
}

func (s *loggerSuite) TestLoggingConfig(c *tc.C) {
	config, err := s.logger.LoggingConfig(s.rawMachine.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config, tc.Not(tc.Equals), "")
}

func (s *loggerSuite) setLoggingConfig(c *tc.C, loggingConfig string) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"logging-config": loggingConfig}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *tc.C) {
	watcher, err := s.logger.WatchLoggingConfig(s.rawMachine.Tag())
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, watcher)
	defer wc.AssertStops()

	// Initial event
	wc.AssertOneChange()

	loggingConfig := "<root>=WARN;juju.log.test=DEBUG"
	s.setLoggingConfig(c, loggingConfig)
	// One change noticing the new version
	wc.AssertOneChange()
	// Setting the version to the same value doesn't trigger a change
	s.setLoggingConfig(c, loggingConfig)
	wc.AssertNoChange()

	loggingConfig = loggingConfig + ";wibble=DEBUG"
	s.setLoggingConfig(c, loggingConfig)
	wc.AssertOneChange()
}
