// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/modelcache"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config modelcache.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = modelcache.ManifoldConfig{
		StateName:            "state",
		CentralHubName:       "central-hub",
		MultiwatcherName:     "multiwatcher",
		InitializedGateName:  "initialized-gate",
		Logger:               loggo.GetLogger("test"),
		PrometheusRegisterer: noopRegisterer{},
		NewWorker: func(modelcache.Config) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return modelcache.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{
		"state", "central-hub", "multiwatcher", "initialized-gate"})
}

func (s *ManifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingStateName(c *tc.C) {
	s.config.StateName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing StateName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingCentralHubName(c *tc.C) {
	s.config.CentralHubName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing CentralHubName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingInitializedGateName(c *tc.C) {
	s.config.InitializedGateName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing InitializedGateName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingMultiwatcherName(c *tc.C) {
	s.config.MultiwatcherName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing MultiwatcherName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingPrometheusRegisterer(c *tc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing PrometheusRegisterer not valid")
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
	c.Check(err, tc.ErrorMatches, "missing NewWorker func not valid")
}

func (s *ManifoldSuite) TestManifoldCallsValidate(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{})
	s.config.Logger = nil
	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `missing Logger not valid`)
}

func (s *ManifoldSuite) TestStateMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            dependency.ErrMissing,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      dependency.ErrMissing,
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMultiwatcherMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     dependency.ErrMissing,
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestInitializedGateMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": dependency.ErrMissing,
	})

	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	var config modelcache.Config
	s.config.NewWorker = func(c modelcache.Config) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            tracker,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	c.Check(config.Validate(), tc.ErrorIsNil)
	c.Check(config.Hub, tc.NotNil)
	c.Check(config.WatcherFactory, tc.NotNil)
	c.Check(config.Logger, tc.Equals, s.config.Logger)
	c.Check(config.PrometheusRegisterer, tc.Equals, s.config.PrometheusRegisterer)

	c.Check(tracker.released, tc.IsFalse)
	config.Cleanup()
	c.Check(tracker.released, tc.IsTrue)
}

func (s *ManifoldSuite) TestNewWorkerErrorReleasesState(c *tc.C) {
	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            tracker,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, tc.ErrorMatches, "boom")
	c.Check(worker, tc.IsNil)
	c.Check(tracker.released, tc.IsTrue)
}

type fakeWorker struct {
	worker.Worker
}

type fakeMultwatcherFactory struct {
	multiwatcher.Factory
}

type fakeStateTracker struct {
	workerstate.StateTracker
	released bool
}

// Return an invalid but non-zero state pool pointer.
// Is only ever used for comparison.
func (f *fakeStateTracker) Use() (*state.StatePool, error) {
	return f.pool(), nil
}

func (f *fakeStateTracker) pool() *state.StatePool {
	return &state.StatePool{}
}

// Done tracks that the used pool is released.
func (f *fakeStateTracker) Done() error {
	f.released = true
	return nil
}
