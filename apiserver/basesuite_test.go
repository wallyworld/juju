// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/testserver"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/modelcache"
	"github.com/juju/juju/internal/worker/multiwatcher"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type baseSuite struct {
	statetesting.StateSuite

	controller *cache.Controller

	cfg apiserver.ServerConfig
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

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
	modelCache, err := modelcache.NewWorker(modelcache.Config{
		StatePool:            s.StatePool,
		Hub:                  pubsub.NewStructuredHub(nil),
		InitializedGate:      initialized,
		Logger:               loggo.GetLogger("modelcache"),
		WatcherFactory:       multiWatcherWorker.WatchController,
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	}.WithDefaultRestartStrategy())
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, modelCache) })

	select {
	case <-initialized.Unlocked():
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("model cache not initialized after %s", testhelpers.LongWait)
	}
	err = modelcache.ExtractCacheController(modelCache, &s.controller)
	c.Assert(err, tc.ErrorIsNil)

	s.cfg = testserver.DefaultServerConfig(c, s.Clock)
	s.cfg.Controller = s.controller
}

func (s *baseSuite) newServer(c *tc.C) *testserver.Server {
	server := testserver.NewServerWithConfig(c, s.StatePool, s.cfg)
	s.AddCleanup(func(c *tc.C) {
		workertest.CleanKill(c, server.APIServer)
		server.HTTPServer.Close()
	})
	server.Info.ModelTag = s.Model.ModelTag()
	return server
}

func (s *baseSuite) openAPIWithoutLogin(c *tc.C, info0 *api.Info) api.Connection {
	info := *info0
	info.Tag = nil
	info.Password = ""
	info.SkipLogin = true
	info.Macaroons = nil
	st, err := api.Open(&info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { _ = st.Close() })
	return st
}

// derivedSuite is just here to test newServer is clean.
type derivedSuite struct {
	baseSuite
}

func TestDerivedSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &derivedSuite{})
}

func (s *derivedSuite) TestNewServer(c *tc.C) {
	_ = s.newServer(c)
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
