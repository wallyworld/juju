// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/agent"
	corepresence "github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/presence"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config presence.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = presence.ManifoldConfig{
		AgentName:      "agent",
		CentralHubName: "central-hub",
		Recorder:       corepresence.New(testclock.NewClock(time.Now())),
		Logger:         loggo.GetLogger("test"),
		NewWorker: func(presence.WorkerConfig) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return presence.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent", "central-hub"})
}

func (s *ManifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingAgentName(c *tc.C) {
	s.config.AgentName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing AgentName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingCentralHubName(c *tc.C) {
	s.config.CentralHubName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing CentralHubName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingRecorder(c *tc.C) {
	s.config.Recorder = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Recorder not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing NewWorker not valid")
}

func (s *ManifoldSuite) TestConfigNewWorker(c *tc.C) {
	// This test will fail at compile time if the presence.NewWorker function
	// has a different signature to the NewWorker config attribute for ManifoldConfig.
	s.config.NewWorker = presence.NewWorker
}

func (s *ManifoldSuite) TestManifoldCallsValidate(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{})
	s.config.Recorder = nil
	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `missing Recorder not valid`)
}

func (s *ManifoldSuite) TestAgentMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	hub := pubsub.NewStructuredHub(nil)
	var config presence.WorkerConfig
	s.config.NewWorker = func(c presence.WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": hub,
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.NotNil)

	c.Check(config.Origin, tc.Equals, "machine-42")
	c.Check(config.Hub, tc.Equals, hub)
	c.Check(config.Recorder, tc.Equals, s.config.Recorder)
}

type fakeWorker struct {
	worker.Worker
}

type fakeAgent struct {
	agent.Agent
	agent.Config

	tag names.Tag
}

// The fake is its own config.
func (f *fakeAgent) CurrentConfig() agent.Config {
	return f
}

func (f *fakeAgent) Tag() names.Tag {
	return f.tag
}
