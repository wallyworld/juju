// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/environupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "boris",
		EnvironName:   "nikolayevich",
		GateName:      "yeltsin",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"boris", "nikolayevich", "yeltsin"})
}

func (*ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
		"environ":    struct{ environs.Environ }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestMissingGateName(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
		"gate":       dependency.ErrMissing,
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestNewFacadeError(c *tc.C) {
	expectAPICaller := struct{ base.APICaller }{}
	expectEnviron := struct{ environs.Environ }{}
	expectGate := struct{ gate.Unlocker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
		"environ":    expectEnviron,
		"gate":       expectGate,
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(actual base.APICaller) (environupgrader.Facade, error) {
			c.Check(actual, tc.Equals, expectAPICaller)
			return nil, errors.New("splort")
		},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "splort")
}

func (*ManifoldSuite) TestNewWorkerError(c *tc.C) {
	expectFacade := struct{ environupgrader.Facade }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
		"gate":       struct{ gate.Unlocker }{},
	})
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return expectFacade, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			c.Check(config.Facade, tc.Equals, expectFacade)
			return nil, errors.New("boof")
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boof")
}

func (*ManifoldSuite) TestNewWorkerSuccessWithEnviron(c *tc.C) {
	expectWorker := &struct{ worker.Worker }{}
	expectEnviron := struct{ environs.Environ }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    expectEnviron,
		"gate":       struct{ gate.Unlocker }{},
	})
	var newWorkerConfig environupgrader.Config
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return struct{ environupgrader.Facade }{}, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			newWorkerConfig = config
			return expectWorker, nil
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.Equals, expectWorker)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newWorkerConfig.Environ, tc.Equals, expectEnviron)
}

func (*ManifoldSuite) TestNewWorkerSuccessWithoutEnviron(c *tc.C) {
	expectWorker := &struct{ worker.Worker }{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{},
		"environ":    dependency.ErrMissing,
		"gate":       struct{ gate.Unlocker }{},
	})
	var newWorkerConfig environupgrader.Config
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		GateName:      "gate",
		NewFacade: func(_ base.APICaller) (environupgrader.Facade, error) {
			return struct{ environupgrader.Facade }{}, nil
		},
		NewWorker: func(config environupgrader.Config) (worker.Worker, error) {
			newWorkerConfig = config
			return expectWorker, nil
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})

	worker, err := manifold.Start(context)
	c.Check(worker, tc.Equals, expectWorker)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newWorkerConfig.Environ, tc.IsNil)
}

func (*ManifoldSuite) TestFilterNil(c *tc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrModelRemoved(c *tc.C) {
	manifold := environupgrader.Manifold(environupgrader.ManifoldConfig{})
	err := manifold.Filter(environupgrader.ErrModelRemoved)
	c.Check(err, tc.Equals, dependency.ErrUninstall)
}
