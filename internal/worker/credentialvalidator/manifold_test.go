// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/credentialvalidator"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (*ManifoldSuite) TestInputs(c *tc.C) {
	manifold := credentialvalidator.Manifold(validManifoldConfig())
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"api-caller"})
}

func (*ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	in := &struct{ worker.Worker }{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, "expected in to implement Flag; got a .*")
}

func (*ManifoldSuite) TestFilterNil(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*ManifoldSuite) TestFilterErrChanged(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrValidityChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterErrModelCredentialChanged(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	err := manifold.Filter(credentialvalidator.ErrModelCredentialChanged)
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*ManifoldSuite) TestFilterOther(c *tc.C) {
	manifold := credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{})
	expect := errors.New("whatever")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*ManifoldSuite) TestStartMissingAPICallerName(c *tc.C) {
	config := validManifoldConfig()
	config.APICallerName = ""
	checkManifoldNotValid(c, config, "empty APICallerName not valid")
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *tc.C) {
	config := validManifoldConfig()
	config.NewFacade = nil
	checkManifoldNotValid(c, config, "nil NewFacade not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *tc.C) {
	config := validManifoldConfig()
	config.NewWorker = nil
	checkManifoldNotValid(c, config, "nil NewWorker not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *tc.C) {
	config := validManifoldConfig()
	config.Logger = nil
	checkManifoldNotValid(c, config, "nil Logger not valid")
}

func (*ManifoldSuite) TestStartMissingAPICaller(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})
	manifold := credentialvalidator.Manifold(validManifoldConfig())

	w, err := manifold.Start(context)
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*ManifoldSuite) TestStartNewFacadeError(c *tc.C) {
	expectCaller := &stubCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectCaller,
	})
	config := validManifoldConfig()
	config.NewFacade = func(caller base.APICaller) (credentialvalidator.Facade, error) {
		c.Check(caller, tc.Equals, expectCaller)
		return nil, errors.New("bort")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "bort")
}

func (*ManifoldSuite) TestStartNewWorkerError(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectFacade := &struct{ credentialvalidator.Facade }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return expectFacade, nil
	}
	config.NewWorker = func(workerConfig credentialvalidator.Config) (worker.Worker, error) {
		c.Check(workerConfig.Facade, tc.Equals, expectFacade)
		return nil, errors.New("snerk")
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "snerk")
}

func (*ManifoldSuite) TestStartSuccess(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &stubCaller{},
	})
	expectWorker := &struct{ worker.Worker }{}
	config := validManifoldConfig()
	config.NewFacade = func(base.APICaller) (credentialvalidator.Facade, error) {
		return &struct{ credentialvalidator.Facade }{}, nil
	}
	config.NewWorker = func(credentialvalidator.Config) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := credentialvalidator.Manifold(config)

	w, err := manifold.Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWorker)
}
