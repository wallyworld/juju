// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasenvironupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"api-caller", "gate"})
}

func (*ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestMissingGateName(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"gate":       dependency.ErrMissing,
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewFacadeError(c *tc.C) {
	expectAPICaller := struct{ base.APICaller }{}
	expectGate := struct{ gate.Unlocker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
		"gate":       expectGate,
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
		NewFacade: func(actual base.APICaller) (caasenvironupgrader.Facade, error) {
			c.Check(actual, tc.Equals, expectAPICaller)
			return nil, errors.New("error")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "error")
}

func (*ManifoldSuite) TestNewWorkerError(c *tc.C) {
	expectFacade := struct{ caasenvironupgrader.Facade }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (caasenvironupgrader.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config caasenvironupgrader.Config) (worker.Worker, error) {
			c.Check(config.Facade, tc.Equals, expectFacade)
			return nil, errors.New("error")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "error")
}
