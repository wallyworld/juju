// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	controllermsg "github.com/juju/juju/pubsub/controller"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
	logger loggo.Logger
	agent  *mockAgent
	hub    *pubsub.StructuredHub
	config agentconfigupdater.WorkerConfig

	initialConfigMsg controllermsg.ConfigChangedMessage
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test")
	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Logger: s.logger,
	})
	s.agent = &mockAgent{
		conf: mockConfig{
			profile:               controller.DefaultMongoMemoryProfile,
			snapChannel:           controller.DefaultJujuDBSnapChannel,
			queryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			queryTracingThreshold: controller.DefaultQueryTracingThreshold,
		},
	}
	s.config = agentconfigupdater.WorkerConfig{
		Agent:                 s.agent,
		Hub:                   s.hub,
		MongoProfile:          controller.DefaultMongoMemoryProfile,
		JujuDBSnapChannel:     controller.DefaultJujuDBSnapChannel,
		QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
		QueryTracingThreshold: controller.DefaultQueryTracingThreshold,
		Logger:                s.logger,
	}
	s.initialConfigMsg = controllermsg.ConfigChangedMessage{
		Config: controller.Config{
			controller.MongoMemoryProfile:    controller.DefaultMongoMemoryProfile,
			controller.JujuDBSnapChannel:     controller.DefaultJujuDBSnapChannel,
			controller.QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			controller.QueryTracingThreshold: controller.DefaultQueryTracingThreshold,
		},
	}
}

func (s *WorkerSuite) TestWorkerConfig(c *tc.C) {
	for i, test := range []struct {
		name      string
		config    func() agentconfigupdater.WorkerConfig
		expectErr string
	}{
		{
			name:   "valid config",
			config: func() agentconfigupdater.WorkerConfig { return s.config },
		}, {
			name: "missing agent",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Agent = nil
				return result
			},
			expectErr: "missing agent not valid",
		}, {
			name: "missing hub",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Hub = nil
				return result
			},
			expectErr: "missing hub not valid",
		}, {
			name: "missing logger",
			config: func() agentconfigupdater.WorkerConfig {
				result := s.config
				result.Logger = nil
				return result
			},
			expectErr: "missing logger not valid",
		},
	} {
		s.logger.Infof("%d: %s", i, test.name)
		config := test.config()
		err := config.Validate()
		if test.expectErr == "" {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.Satisfies, errors.IsNotValid)
			c.Check(err, tc.ErrorMatches, test.expectErr)
		}
	}
}

func (s *WorkerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	config := s.config
	config.Agent = nil
	w, err := agentconfigupdater.NewWorker(config)
	c.Assert(w, tc.IsNil)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *WorkerSuite) TestNormalStart(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestUpdateMongoProfile(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	// Profile the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.MongoMemoryProfile] = "new-value"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateJujuDBSnapChannel(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	// Snap channel is the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.JujuDBSnapChannel] = "latest/candidate"
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingEnabled(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	// Query tracing enabled is the same, worker still alive.
	workertest.CheckAlive(c, w)

	newConfig.Config[controller.QueryTracingEnabled] = true
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *WorkerSuite) TestUpdateQueryTracingThreshold(c *tc.C) {
	w, err := agentconfigupdater.NewWorker(s.config)
	c.Assert(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)

	newConfig := s.initialConfigMsg
	handled, err := s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	// Query tracing threshold is the same, worker still alive.
	workertest.CheckAlive(c, w)

	d := time.Second * 2
	newConfig.Config[controller.QueryTracingThreshold] = d.String()
	handled, err = s.hub.Publish(controllermsg.ConfigChanged, newConfig)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-pubsub.Wait(handled):
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("event not handled")
	}

	err = workertest.CheckKilled(c, w)

	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)
}
