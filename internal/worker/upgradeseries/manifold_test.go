// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/upgradeseries"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewFacade = nil

	c.Check(upgradeseries.Manifold(cfg).Inputs, tc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (*ManifoldSuite) TestStartMissingNewFacade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewFacade = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "nil NewFacade function not valid")
}

func (*ManifoldSuite) TestStartMissingNewWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewWorker = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "nil NewWorker function not valid")
}

func (*ManifoldSuite) TestStartMissingLogger(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.Logger = nil

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "nil Logger not valid")
}

func (s *ManifoldSuite) TestStartMissingAgentName(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	ctx := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      dependency.ErrMissing,
		"api-caller-name": &dummyAPICaller{},
	})

	work, err := upgradeseries.Manifold(cfg).Start(ctx)
	c.Check(work, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartMissingAPICallerName(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	ctx := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	work, err := upgradeseries.Manifold(cfg).Start(ctx)
	c.Check(work, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestStartError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg, _, _ := validManifoldConfig(ctrl)
	cfg.NewWorker = func(_ upgradeseries.Config) (worker.Worker, error) { return nil, errors.New("WHACK!") }

	work, err := upgradeseries.Manifold(cfg).Start(newStubContext())
	c.Check(work, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "starting machine upgrade series worker: WHACK!")
}

type dummyAPICaller struct {
	base.APICaller
}

type dummyConfig struct {
	agent.Config
}

type dummyAgent struct {
	agent.Agent
}

func (*dummyAgent) CurrentConfig() agent.Config {
	return &dummyConfig{}
}

func (*dummyConfig) Tag() names.Tag {
	return names.NewMachineTag("666")
}

func (*dummyConfig) DataDir() string {
	return "/unused"
}

func newStubContext() *dt.Context {
	return dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})
}
