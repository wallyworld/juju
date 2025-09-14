// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallersidecar_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewallersidecar"
	"github.com/juju/juju/internal/worker/caasfirewallersidecar/mocks"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	context  dependency.Context

	apiCaller *mocks.MockAPICaller
	broker    *caasmocks.MockBroker
	client    *mocks.MockClient

	ctrl *gomock.Controller
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.ctrl = gomock.NewController(c)
	s.apiCaller = mocks.NewMockAPICaller(s.ctrl)
	s.broker = caasmocks.NewMockBroker(s.ctrl)
	s.client = mocks.NewMockClient(s.ctrl)

	s.context = s.newContext(nil)
	s.manifold = caasfirewallersidecar.Manifold(s.validConfig())
}

func (s *manifoldSuite) validConfig() caasfirewallersidecar.ManifoldConfig {
	return caasfirewallersidecar.ManifoldConfig{
		APICallerName:  "api-caller",
		BrokerName:     "broker",
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		NewClient:      s.newClient,
		NewWorker:      s.newWorker,
		Logger:         loggo.GetLogger("test"),
	}
}

func (s *manifoldSuite) newClient(apiCaller base.APICaller) caasfirewallersidecar.Client {
	s.MethodCall(s, "NewClient", apiCaller)
	return s.client
}

func (s *manifoldSuite) newWorker(config caasfirewallersidecar.Config) (worker.Worker, error) {
	s.MethodCall(s, "NewWorker", config)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"api-caller": s.apiCaller,
		"broker":     s.broker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) TestMissingControllerUUID(c *tc.C) {
	config := s.validConfig()
	config.ControllerUUID = ""
	s.checkConfigInvalid(c, config, "empty ControllerUUID not valid")
}

func (s *manifoldSuite) TestMissingModelUUID(c *tc.C) {
	config := s.validConfig()
	config.ModelUUID = ""
	s.checkConfigInvalid(c, config, "empty ModelUUID not valid")
}

func (s *manifoldSuite) TestMissingAPICallerName(c *tc.C) {
	config := s.validConfig()
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *tc.C) {
	config := s.validConfig()
	config.BrokerName = ""
	s.checkConfigInvalid(c, config, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *tc.C) {
	config := s.validConfig()
	config.NewWorker = nil
	s.checkConfigInvalid(c, config, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *tc.C) {
	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *tc.C, config caasfirewallersidecar.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

var expectedInputs = []string{"api-caller", "broker"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewClient", "NewWorker")
	s.CheckCall(c, 0, "NewClient", s.apiCaller)

	s.CheckCall(c, 1, "NewWorker", caasfirewallersidecar.Config{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		FirewallerAPI:  s.client,
		LifeGetter:     s.client,
		Broker:         s.broker,
		Logger:         loggo.GetLogger("test"),
	})
}
