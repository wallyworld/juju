// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/logger"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/cache/cachetest"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type loggerSuite struct {
	statetesting.StateSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	logger     *logger.LoggerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	ctrl *cachetest.TestController
}

func TestLoggerSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &loggerSuite{})
}

func (s *loggerSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}

	s.ctrl = cachetest.NewTestController(cachetest.ModelEvents)
	s.ctrl.Init(c)
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, s.ctrl.Controller) })

	// Add the current model to the controller.
	m := cachetest.ModelChangeFromState(c, s.State)
	s.ctrl.SendChange(m)

	// Ensure it is processed before we create the logger API.
	_ = s.ctrl.NextChange(c)

	s.logger, err = s.makeLoggerAPI(s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loggerSuite) makeLoggerAPI(auth facade.Authorizer) (*logger.LoggerAPI, error) {
	ctx := facadetest.Context{
		Auth_:       auth,
		Controller_: s.ctrl.Controller,
		Resources_:  s.resources,
		State_:      s.State,
	}
	return logger.NewLoggerAPI(ctx)
}

func (s *loggerSuite) TestNewLoggerAPIRefusesNonAgent(c *tc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUserTag("some-user")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(endPoint, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsUnitAgent(c *tc.C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("germany/7")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endPoint, tc.NotNil)
}

func (s *loggerSuite) TestNewLoggerAPIAcceptsApplicationAgent(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewApplicationTag("germany")
	endPoint, err := s.makeLoggerAPI(anAuthorizer)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endPoint, tc.NotNil)
}

func (s *loggerSuite) TestWatchLoggingConfigNothing(c *tc.C) {
	// Not an error to watch nothing
	results := s.logger.WatchLoggingConfig(params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *loggerSuite) setLoggingConfig(c *tc.C, loggingConfig string) {
	m := cachetest.ModelChangeFromState(c, s.State)
	m.Config["logging-config"] = loggingConfig
	s.ctrl.SendChange(m)
	_ = s.ctrl.NextChange(c)
}

func (s *loggerSuite) TestWatchLoggingConfig(c *tc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.WatchLoggingConfig(args)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Assert(results.Results[0].Error, tc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, tc.NotNil)

	_, ok := resource.(cache.NotifyWatcher)
	c.Assert(ok, tc.IsTrue)
	// The watcher implementation is tested in the cache package.
}

func (s *loggerSuite) TestWatchLoggingConfigRefusesWrongAgent(c *tc.C) {
	// We are a machine agent, but not the one we are trying to track
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.WatchLoggingConfig(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "")
	c.Assert(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForNoone(c *tc.C) {
	// Not an error to request nothing, dumb, but not an error.
	results := s.logger.LoggingConfig(params.Entities{})
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *loggerSuite) TestLoggingConfigRefusesWrongAgent(c *tc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-12354"}},
	}
	results := s.logger.LoggingConfig(args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *loggerSuite) TestLoggingConfigForAgent(c *tc.C) {
	newLoggingConfig := "<root>=WARN;juju.log.test=DEBUG;unit=INFO"
	s.setLoggingConfig(c, newLoggingConfig)

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results := s.logger.LoggingConfig(args)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.Result, tc.Equals, newLoggingConfig)
}
