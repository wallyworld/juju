// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiconfigwatcher_test

import (
	"sync"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	manifold           dependency.Manifold
	context            dependency.Context
	agent              *mockAgent
	agentConfigChanged *voyeur.Value
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = new(mockAgent)
	s.context = dt.StubContext(nil, map[string]interface{}{
		"agent": s.agent,
	})
	s.agentConfigChanged = voyeur.NewValue(0)
	s.manifold = apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
		AgentName:          "agent",
		AgentConfigChanged: s.agentConfigChanged,
		Logger:             loggo.GetLogger("test"),
	})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNilAgentConfigChanged(c *tc.C) {
	manifold := apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
		AgentName: "agent",
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, tc.ErrorMatches, "nil AgentConfigChanged .+")
}

func (s *ManifoldSuite) TestNoAgent(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	_, err := s.manifold.Start(context)
	c.Assert(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestBounceOnChange(c *tc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1")
	w := s.startWorkerClean(c)

	// Change API addresses - worker should bounce.
	s.agent.conf.setAddresses("2.2.2.2:2")
	s.agentConfigChanged.Set(0)
	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.Equals, dependency.ErrBounce)

	// Restart the worker - worker should stay up.
	w = s.startWorkerClean(c)

	// Change API addresses again - worker should bounce again.
	s.agent.conf.setAddresses("2.2.2.2:2", "3.3.3.3:3")
	s.agentConfigChanged.Set(0)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.Equals, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestConfigChangeWithNoAddrChange(c *tc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1")
	w := s.startWorkerClean(c)

	// Signal config change without changing API addresses - worker
	// should continue running.
	s.agentConfigChanged.Set(0)
	workertest.CheckAlive(c, w)
}

func (s *ManifoldSuite) TestConfigChangeWithAddrReordering(c *tc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1", "2.2.2.2:2")
	w := s.startWorkerClean(c)

	// Change API address ordering - worker should stay up.
	s.agent.conf.setAddresses("2.2.2.2:2", "1.1.1.1:1")
	s.agentConfigChanged.Set(0)
	workertest.CheckAlive(c, w)
}

func (s *ManifoldSuite) TestClosedVoyeur(c *tc.C) {
	w := s.startWorkerClean(c)
	s.agentConfigChanged.Close()
	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "config changed value closed")
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockConfig struct {
	agent.Config

	mu    sync.Mutex
	addrs []string
}

func (mc *mockConfig) setAddresses(addrs ...string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.addrs = append([]string(nil), addrs...)
}

func (mc *mockConfig) APIAddresses() ([]string, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.addrs, nil
}
