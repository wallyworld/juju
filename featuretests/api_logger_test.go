// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type apiLoggerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *apiLoggerSuite) TestLoggingConfig(c *tc.C) {
	root, machine := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	logging := logger.NewState(root)

	obtained, err := logging.LoggingConfig(machine.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.Equals, "<root>=INFO")
}

func (s *apiLoggerSuite) TestWatchLoggingConfig(c *tc.C) {
	root, machine := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	logging := logger.NewState(root)

	watcher, err := logging.WatchLoggingConfig(machine.Tag())
	c.Assert(err, tc.ErrorIsNil)
	_ = watcher

	wc := watchertest.NewNotifyWatcherC(c, watcher)
	// Initial event.
	wc.AssertOneChange()

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.UpdateModelConfig(
		map[string]interface{}{
			"logging-config": "juju=INFO;test=TRACE",
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertOneChange()
	wc.AssertStops()
}
