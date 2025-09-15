// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/charmrevision"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "billy",
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"billy"})
	c.Check(manifold.Start, tc.NotNil)
	c.Check(manifold.Output, tc.IsNil)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "api-caller",
		Clock:         fakeClock{},
	})

	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	}))
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingClock(c *tc.C) {
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "api-caller",
	})

	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": fakeAPICaller{},
	}))
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, "nil Clock not valid")
}

func (s *ManifoldSuite) TestNewFacadeError(c *tc.C) {
	fakeAPICaller := &fakeAPICaller{}

	stub := testhelpers.Stub{}
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "api-caller",
		Clock:         fakeClock{},
		NewFacade: func(apiCaller base.APICaller) (charmrevision.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return nil, errors.New("blefgh")
		},
	})

	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": fakeAPICaller,
	}))
	c.Check(err, tc.ErrorMatches, "cannot create facade: blefgh")
	stub.CheckCalls(c, []testhelpers.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}})
}

func (s *ManifoldSuite) TestNewWorkerError(c *tc.C) {
	fakeClock := &fakeClock{}
	fakeFacade := &fakeFacade{}
	fakeAPICaller := &fakeAPICaller{}

	stub := testhelpers.Stub{}
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "api-caller",
		Clock:         fakeClock,
		NewFacade: func(apiCaller base.APICaller) (charmrevision.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return fakeFacade, nil
		},
		NewWorker: func(config charmrevision.Config) (worker.Worker, error) {
			stub.AddCall("NewWorker", config)
			return nil, errors.New("snrght")
		},
	})

	_, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": fakeAPICaller,
	}))
	c.Check(err, tc.ErrorMatches, "cannot create worker: snrght")
	stub.CheckCalls(c, []testhelpers.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}, {
		"NewWorker", []interface{}{charmrevision.Config{
			RevisionUpdater: fakeFacade,
			Clock:           fakeClock,
		}},
	}})
}

func (s *ManifoldSuite) TestSuccess(c *tc.C) {
	fakeClock := &fakeClock{}
	fakeFacade := &fakeFacade{}
	fakeWorker := &fakeWorker{}
	fakeAPICaller := &fakeAPICaller{}

	stub := testhelpers.Stub{}
	manifold := charmrevision.Manifold(charmrevision.ManifoldConfig{
		APICallerName: "api-caller",
		Clock:         fakeClock,
		Period:        10 * time.Minute,
		NewFacade: func(apiCaller base.APICaller) (charmrevision.Facade, error) {
			stub.AddCall("NewFacade", apiCaller)
			return fakeFacade, nil
		},
		NewWorker: func(config charmrevision.Config) (worker.Worker, error) {
			stub.AddCall("NewWorker", config)
			return fakeWorker, nil
		},
	})

	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": fakeAPICaller,
	}))
	c.Check(w, tc.Equals, fakeWorker)
	c.Check(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{{
		"NewFacade", []interface{}{fakeAPICaller},
	}, {
		"NewWorker", []interface{}{charmrevision.Config{
			Period:          10 * time.Minute,
			RevisionUpdater: fakeFacade,
			Clock:           fakeClock,
		}},
	}})
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeClock struct {
	clock.Clock
}

type fakeWorker struct {
	worker.Worker
}

type fakeFacade struct {
	charmrevision.Facade
}
