// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/singular"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config singular.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = singular.ManifoldConfig{
		Clock:         testclock.NewClock(time.Now()),
		APICallerName: "api-caller",
		Duration:      time.Minute,
		NewFacade: func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
			return nil, errors.NotImplementedf("NewFacade")
		},
		NewWorker: func(config singular.FlagConfig) (worker.Worker, error) {
			return nil, errors.NotImplementedf("NewWorker")
		},
	}
}

func (s *ManifoldSuite) TestValidate(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestValidateMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, "nil Clock not valid")
}

func (s *ManifoldSuite) TestValidateMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, "missing APICallerName not valid")
}

func (s *ManifoldSuite) TestValidateMissingNewFacade(c *tc.C) {
	s.config.NewFacade = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, "nil NewFacade not valid")
}

func (s *ManifoldSuite) TestValidateMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		APICallerName: "kim",
	})
	expectInputs := []string{"kim"}
	c.Check(manifold.Inputs, tc.DeepEquals, expectInputs)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	var out engine.Flag
	err := manifold.Output(&fakeWorker{}, &out)
	c.Check(err, tc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (s *ManifoldSuite) TestOutputBadResult(c *tc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		var out interface{}
		err := manifold.Output(flag, &out)
		c.Check(err, tc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
		c.Check(out, tc.IsNil)
	})
}

func (s *ManifoldSuite) TestOutputSuccess(c *tc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{})
	fix := newFixture(c)
	fix.Run(c, func(flag *singular.FlagWorker, _ *testclock.Clock, _ func()) {
		var out engine.Flag
		err := manifold.Output(flag, &out)
		c.Check(err, tc.ErrorIsNil)
		c.Check(out, tc.Equals, flag)
	})
}

func (s *ManifoldSuite) TestStartMissingClock(c *tc.C) {
	manifold := singular.Manifold(singular.ManifoldConfig{
		APICallerName: "api-caller",
	})
	context := dt.StubContext(nil, map[string]interface{}{})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), tc.ErrorMatches, `nil Clock not valid`)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *tc.C) {
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartNewFacadeError(c *tc.C) {
	expectAPICaller := &fakeAPICaller{}
	s.config.Claimant = names.NewMachineTag("123")
	s.config.Entity = coretesting.ModelTag
	s.config.NewFacade = func(apiCaller base.APICaller, claimant names.Tag, entity names.Tag) (singular.Facade, error) {
		c.Check(apiCaller, tc.Equals, expectAPICaller)
		c.Check(claimant.String(), tc.Equals, "machine-123")
		c.Check(entity, tc.Equals, coretesting.ModelTag)
		return nil, errors.New("grark plop")
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": expectAPICaller,
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorMatches, "grark plop")
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerError(c *tc.C) {
	expectFacade := &fakeFacade{}
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return expectFacade, nil
	}
	s.config.NewWorker = func(config singular.FlagConfig) (worker.Worker, error) {
		c.Check(config.Facade, tc.Equals, expectFacade)
		err := config.Validate()
		c.Check(err, tc.ErrorIsNil)
		return nil, errors.New("blomp tik")
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorMatches, "blomp tik")
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	var stub testhelpers.Stub
	expectWorker := newStubWorker(&stub)
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return &fakeFacade{}, nil
	}
	s.config.NewWorker = func(_ singular.FlagConfig) (worker.Worker, error) {
		return expectWorker, nil
	}
	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorIsNil)

	var out engine.Flag
	err = manifold.Output(worker, &out)
	c.Check(err, tc.ErrorIsNil)
	c.Check(out.Check(), tc.IsTrue)

	c.Check(worker.Wait(), tc.ErrorIsNil)
	stub.CheckCallNames(c, "Check", "Wait")
}

func (s *ManifoldSuite) TestWorkerBouncesOnRefresh(c *tc.C) {
	var stub testhelpers.Stub
	stub.SetErrors(singular.ErrRefresh)
	errWorker := newStubWorker(&stub)
	s.config.NewFacade = func(base.APICaller, names.Tag, names.Tag) (singular.Facade, error) {
		return &fakeFacade{}, nil
	}
	s.config.NewWorker = func(_ singular.FlagConfig) (worker.Worker, error) {
		return errWorker, nil
	}

	manifold := singular.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": &fakeAPICaller{},
	})

	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Wait(), tc.Equals, dependency.ErrBounce)
}
