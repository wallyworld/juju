// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/modelcache"
	"github.com/juju/juju/internal/worker/multiwatcher"
	"github.com/juju/juju/pubsub/controller"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type sharedServerContextSuite struct {
	statetesting.StateSuite

	hub    *pubsub.StructuredHub
	config sharedServerConfig
}

func TestSharedServerContextSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &sharedServerContextSuite{})
}

func (s *sharedServerContextSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)

	allWatcherBacking, err := state.NewAllWatcherBacking(s.StatePool)
	c.Assert(err, tc.ErrorIsNil)
	multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               loggo.GetLogger("test"),
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	})
	c.Assert(err, tc.ErrorIsNil)
	// The worker itself is a coremultiwatcher.Factory.
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, multiWatcherWorker) })

	initialized := gate.NewLock()
	s.hub = pubsub.NewStructuredHub(nil)
	modelCache, err := modelcache.NewWorker(modelcache.Config{
		StatePool:            s.StatePool,
		Hub:                  s.hub,
		InitializedGate:      initialized,
		Logger:               loggo.GetLogger("test"),
		WatcherFactory:       multiWatcherWorker.WatchController,
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	}.WithDefaultRestartStrategy())
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, modelCache) })
	c.Assert(err, tc.ErrorIsNil)
	var controller *cache.Controller
	err = modelcache.ExtractCacheController(modelCache, &controller)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	s.config = sharedServerConfig{
		statePool:           s.StatePool,
		controller:          controller,
		multiwatcherFactory: multiWatcherWorker,
		centralHub:          s.hub,
		presence:            presence.New(clock.WallClock),
		leaseManager:        &lease.Manager{},
		controllerConfig:    controllerConfig,
		logger:              loggo.GetLogger("test"),
		dbGetter:            StubDBGetter{},
	}
}

func (s *sharedServerContextSuite) TestConfigNoStatePool(c *tc.C) {
	s.config.statePool = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil statePool not valid")
}

func (s *sharedServerContextSuite) TestConfigNoController(c *tc.C) {
	s.config.controller = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil controller not valid")
}

func (s *sharedServerContextSuite) TestConfigNoMultiwatcherFactory(c *tc.C) {
	s.config.multiwatcherFactory = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil multiwatcherFactory not valid")
}

func (s *sharedServerContextSuite) TestConfigNoHub(c *tc.C) {
	s.config.centralHub = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil centralHub not valid")
}

func (s *sharedServerContextSuite) TestConfigNoPresence(c *tc.C) {
	s.config.presence = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil presence not valid")
}

func (s *sharedServerContextSuite) TestConfigNoLeaseManager(c *tc.C) {
	s.config.leaseManager = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil leaseManager not valid")
}

func (s *sharedServerContextSuite) TestConfigNoControllerconfig(c *tc.C) {
	s.config.controllerConfig = nil
	err := s.config.validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil controllerConfig not valid")
}

func (s *sharedServerContextSuite) TestNewCallsConfigValidate(c *tc.C) {
	s.config.statePool = nil
	ctx, err := newSharedServerContext(s.config)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "nil statePool not valid")
	c.Check(ctx, tc.IsNil)
}

func (s *sharedServerContextSuite) TestValidConfig(c *tc.C) {
	ctx, err := newSharedServerContext(s.config)
	c.Assert(err, tc.ErrorIsNil)
	// Normally you wouldn't directly access features.
	c.Assert(ctx.features, tc.HasLen, 0)
	ctx.Close()
}

func (s *sharedServerContextSuite) newContext(c *tc.C) *sharedServerContext {
	ctx, err := newSharedServerContext(s.config)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { ctx.Close() })
	return ctx
}

type stubHub struct {
	*pubsub.StructuredHub

	published []string
}

func (s *stubHub) Publish(topic string, data interface{}) (func(), error) {
	s.published = append(s.published, topic)
	return func() {}, nil
}

func (s *sharedServerContextSuite) TestControllerConfigChanged(c *tc.C) {
	stub := &stubHub{StructuredHub: s.hub}
	s.config.centralHub = stub
	ctx := s.newContext(c)

	msg := controller.ConfigChangedMessage{
		Config: corecontroller.Config{
			corecontroller.Features: []string{"foo", "bar"},
		},
	}

	done, err := s.hub.Publish(controller.ConfigChanged, msg)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-pubsub.Wait(done):
	case <-time.After(testing.LongWait):
		c.Fatalf("handler didn't")
	}

	c.Check(ctx.featureEnabled("foo"), tc.IsTrue)
	c.Check(ctx.featureEnabled("bar"), tc.IsTrue)
	c.Check(ctx.featureEnabled("baz"), tc.IsFalse)
	c.Check(stub.published, tc.HasLen, 0)
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
