// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasunitprovisioner"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	context  dependency.Context

	apiCaller fakeAPICaller
	broker    fakeBroker
	client    fakeClient
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = caasunitprovisioner.Manifold(s.validConfig())
}

func (s *ManifoldSuite) validConfig() caasunitprovisioner.ManifoldConfig {
	return caasunitprovisioner.ManifoldConfig{
		APICallerName: "api-caller",
		BrokerName:    "broker",
		NewClient:     s.newClient,
		NewWorker:     s.newWorker,
		Logger:        loggo.GetLogger("test"),
	}
}

func (s *ManifoldSuite) newClient(apiCaller base.APICaller) caasunitprovisioner.Client {
	s.MethodCall(s, "NewClient", apiCaller)
	return &s.client
}

func (s *ManifoldSuite) newWorker(config caasunitprovisioner.Config) (worker.Worker, error) {
	s.MethodCall(s, "NewWorker", config)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"api-caller": &s.apiCaller,
		"broker":     &s.broker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *tc.C) {
	config := s.validConfig()
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingBrokerName(c *tc.C) {
	config := s.validConfig()
	config.BrokerName = ""
	s.checkConfigInvalid(c, config, "empty BrokerName not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *tc.C) {
	config := s.validConfig()
	config.NewWorker = nil
	s.checkConfigInvalid(c, config, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *tc.C) {
	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *ManifoldSuite) checkConfigInvalid(c *tc.C, config caasunitprovisioner.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

var expectedInputs = []string{"api-caller", "broker"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewClient", "NewWorker")
	s.CheckCall(c, 0, "NewClient", &s.apiCaller)

	args := s.Calls()[1].Args
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, caasunitprovisioner.Config{})
	config := args[0].(caasunitprovisioner.Config)

	c.Assert(config, tc.DeepEquals, caasunitprovisioner.Config{
		ApplicationGetter:        &s.client,
		ApplicationUpdater:       &s.client,
		ServiceBroker:            &s.broker,
		ContainerBroker:          &s.broker,
		ProvisioningInfoGetter:   &s.client,
		ProvisioningStatusSetter: &s.client,
		LifeGetter:               &s.client,
		CharmGetter:              &s.client,
		UnitUpdater:              &s.client,
		Logger:                   loggo.GetLogger("test"),
	})
}
