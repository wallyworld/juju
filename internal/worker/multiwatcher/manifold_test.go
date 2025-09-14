// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	tctesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/multiwatcher"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config multiwatcher.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = multiwatcher.ManifoldConfig{
		StateName:            "state",
		Clock:                clock.WallClock,
		Logger:               loggo.GetLogger("test"),
		PrometheusRegisterer: noopRegisterer{},
		NewWorker: func(multiwatcher.Config) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
		NewAllWatcher: func(*state.StatePool) (state.AllWatcherBacking, error) {
			return &fakeAllWatcher{}, nil
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return multiwatcher.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"state"})
}

func (s *ManifoldSuite) TestConfigValidation(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingStateName(c *tc.C) {
	s.config.StateName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "empty StateName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingPrometheusRegisterer(c *tc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing PrometheusRegisterer not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing Clock not valid")
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
		"state": dependency.ErrMissing,
	})

	w, err := s.manifold().Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
	var config multiwatcher.Config
	s.config.NewWorker = func(c multiwatcher.Config) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state": tracker,
	})

	w, err := s.manifold().Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	c.Check(config.Validate(), tc.ErrorIsNil)
	c.Check(config.Logger, tc.Equals, s.config.Logger)
	c.Check(config.PrometheusRegisterer, tc.Equals, s.config.PrometheusRegisterer)

	c.Check(tracker.released, tc.IsFalse)
	config.Cleanup()
	c.Check(tracker.released, tc.IsTrue)
}

func (s *ManifoldSuite) TestNewWorkerErrorReleasesState(c *tc.C) {
	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state": tracker,
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, tc.ErrorMatches, "boom")
	c.Check(worker, tc.IsNil)
	c.Check(tracker.released, tc.IsTrue)
}

type fakeWorker struct {
	worker.Worker
}

type fakeStateTracker struct {
	workerstate.StateTracker
	released bool
}

// Return an invalid but non-zero state pool pointer.
// Is only ever used for comparison.
func (f *fakeStateTracker) Use() (*state.StatePool, error) {
	return &state.StatePool{}, nil
}

// Done tracks that the used pool is released.
func (f *fakeStateTracker) Done() error {
	f.released = true
	return nil
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

type fakeAllWatcher struct {
	state.AllWatcherBacking
}
