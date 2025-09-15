// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	coreagent "github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/internal/testing"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ManifoldSuite struct {
	statetesting.StateSuite
	manifold          dependency.Manifold
	openStateCalled   bool
	openStateErr      error
	config            workerstate.ManifoldConfig
	resources         dt.StubResources
	setStatePoolCalls []*state.StatePool
}

func TestManifoldSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)

	s.openStateCalled = false
	s.openStateErr = nil
	s.setStatePoolCalls = nil

	s.config = workerstate.ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		OpenStatePool:          s.fakeOpenState,
		PingInterval:           10 * time.Millisecond,
		SetStatePool: func(pool *state.StatePool) {
			s.setStatePoolCalls = append(s.setStatePoolCalls, pool)
		},
	}
	s.manifold = workerstate.Manifold(s.config)
	s.resources = dt.StubResources{
		"agent":                dt.NewStubResource(new(mockAgent)),
		"state-config-watcher": dt.NewStubResource(true),
	}
}

func (s *ManifoldSuite) fakeOpenState(context.Context, coreagent.Config) (*state.StatePool, error) {
	s.openStateCalled = true
	if s.openStateErr != nil {
		return nil, s.openStateErr
	}
	// Here's one we prepared earlier...
	return s.StatePool, nil
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{
		"agent",
		"state-config-watcher",
	})
}

func (s *ManifoldSuite) TestStartAgentMissing(c *tc.C) {
	s.resources["agent"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.startManifold(c)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStateConfigWatcherMissing(c *tc.C) {
	s.resources["state-config-watcher"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.startManifold(c)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenStateNil(c *tc.C) {
	s.config.OpenStatePool = nil
	s.startManifoldInvalidConfig(c, s.config, "nil OpenStatePool not valid")
}

func (s *ManifoldSuite) TestStartSetStatePoolNil(c *tc.C) {
	s.config.SetStatePool = nil
	s.startManifoldInvalidConfig(c, s.config, "nil SetStatePool not valid")
}

func (s *ManifoldSuite) startManifoldInvalidConfig(c *tc.C, config workerstate.ManifoldConfig, expect string) {
	manifold := workerstate.Manifold(config)
	w, err := manifold.Start(s.resources.Context())
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, expect)
}

func (s *ManifoldSuite) TestStartNotStateServer(c *tc.C) {
	s.resources["state-config-watcher"] = dt.NewStubResource(false)
	w, err := s.startManifold(c)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	c.Check(err, tc.ErrorMatches, "no StateServingInfo in config: dependency not available")
}

func (s *ManifoldSuite) TestStartOpenStateFails(c *tc.C) {
	s.openStateErr = errors.New("boom")
	w, err := s.startManifold(c)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	w := s.mustStartManifold(c)
	c.Check(s.openStateCalled, tc.IsTrue)
	checkNotExiting(c, w)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestStatePinging(c *tc.C) {
	w := s.mustStartManifold(c)
	checkNotExiting(c, w)

	// Kill the mongod to cause pings to fail.
	mgotesting.MgoServer.Destroy()

	// FIXME: Ideally we'd want the "state ping failed" error here, but in reality the txn watcher will fail
	// first because it is long polling.
	checkExitsWithError(c, w, "(state ping failed|hub txn watcher sync error): .+")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	var st *state.State
	err := s.manifold.Output(dummyWorker{}, &st)
	c.Check(st, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `in should be a \*state.stateWorker; .+`)
}

func (s *ManifoldSuite) TestOutputWrongType(c *tc.C) {
	w := s.mustStartManifold(c)

	var wrong int
	err := s.manifold.Output(w, &wrong)
	c.Check(wrong, tc.Equals, 0)
	c.Check(err, tc.ErrorMatches, `out should be \*StateTracker; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccess(c *tc.C) {
	w := s.mustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, tc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, tc.ErrorIsNil)
	systemState, err := pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(systemState, tc.Equals, s.State)
	c.Assert(stTracker.Done(), tc.ErrorIsNil)

	// Ensure State is closed when the worker is done.
	workertest.CleanKill(c, w)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *ManifoldSuite) TestStateStillInUse(c *tc.C) {
	w := s.mustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, tc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, tc.ErrorIsNil)

	// Close the worker while the State is still in use.
	workertest.CleanKill(c, w)
	assertStatePoolNotClosed(c, pool)

	// Now signal that the State is no longer needed.
	c.Assert(stTracker.Done(), tc.ErrorIsNil)
	assertStatePoolClosed(c, pool)
}

func (s *ManifoldSuite) TestDeadStateRemoved(c *tc.C) {
	// Create an additional state *before* we start
	// the worker, so the worker's lifecycle watcher
	// is guaranteed to observe it in both the Alive
	// state and the Dead state.
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	model, err := newSt.Model()
	c.Assert(err, tc.ErrorIsNil)

	w := s.mustStartManifold(c)
	defer workertest.CleanKill(c, w)

	var stTracker workerstate.StateTracker
	err = s.manifold.Output(w, &stTracker)
	c.Assert(err, tc.ErrorIsNil)
	pool, err := stTracker.Use()
	c.Assert(err, tc.ErrorIsNil)
	defer stTracker.Done()

	// Get a reference to the state pool entry, so we can
	// prevent it from being fully removed from the pool.
	st, err := pool.Get(newSt.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	defer st.Release()

	// Progress the model to Dead.
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(newSt.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		st, err := pool.Get(newSt.ModelUUID())
		if errors.IsNotFound(err) {
			c.Assert(err, tc.ErrorMatches, "model .* has been removed")
			return
		}
		c.Assert(err, tc.ErrorIsNil)
		st.Release()
	}
	c.Fatal("timed out waiting for model state to be removed from pool")
}

func (s *ManifoldSuite) mustStartManifold(c *tc.C) worker.Worker {
	w, err := s.startManifold(c)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *ManifoldSuite) startManifold(c *tc.C) (worker.Worker, error) {
	w, err := s.manifold.Start(s.resources.Context())
	if w != nil {
		s.AddCleanup(func(*tc.C) { worker.Stop(w) })
	}
	return w, err
}

func checkNotExiting(c *tc.C, w worker.Worker) {
	exited := make(chan bool)
	go func() {
		w.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		c.Fatal("worker exited unexpectedly")
	case <-time.After(coretesting.ShortWait):
		// Worker didn't exit (good)
	}
}

func checkExitsWithError(c *tc.C, w worker.Worker, expectedErr string) {
	errCh := make(chan error)
	go func() {
		errCh <- w.Wait()
	}()
	select {
	case err := <-errCh:
		c.Check(err, tc.ErrorMatches, expectedErr)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to exit")
	}
}

type mockAgent struct {
	coreagent.Agent
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return new(mockConfig)
}

type mockConfig struct {
	coreagent.Config
}

type dummyWorker struct {
	worker.Worker
}
