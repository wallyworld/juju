// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/applicationscaler"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "washington the terrible",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"washington the terrible"})
}

func (s *ManifoldSuite) TestOutput(c *tc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{})
	c.Check(manifold.Output, tc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *tc.C) {
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartFacadeError(c *tc.C) {
	expectCaller := &fakeCaller{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(apiCaller base.APICaller) (applicationscaler.Facade, error) {
			c.Check(apiCaller, tc.Equals, expectCaller)
			return nil, errors.New("blort")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectCaller,
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorMatches, "blort")
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartWorkerError(c *tc.C) {
	expectFacade := &fakeFacade{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (applicationscaler.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config applicationscaler.Config) (worker.Worker, error) {
			c.Check(config.Validate(), tc.ErrorIsNil)
			c.Check(config.Facade, tc.Equals, expectFacade)
			return nil, errors.New("splot")
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeCaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorMatches, "splot")
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestSuccess(c *tc.C) {
	expectWorker := &fakeWorker{}
	manifold := applicationscaler.Manifold(applicationscaler.ManifoldConfig{
		APICallerName: "api-caller",
		NewFacade: func(_ base.APICaller) (applicationscaler.Facade, error) {
			return &fakeFacade{}, nil
		},
		NewWorker: func(_ applicationscaler.Config) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeCaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expectWorker)
}

type fakeCaller struct {
	base.APICaller
}

type fakeFacade struct {
	applicationscaler.Facade
}

type fakeWorker struct {
	worker.Worker
}
