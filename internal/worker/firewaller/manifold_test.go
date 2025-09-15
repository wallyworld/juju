// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/firewaller"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestManifoldFirewallModeNone(c *tc.C) {
	ctx := &mockDependencyContext{
		env: &mockEnviron{
			config: coretesting.CustomModelConfig(c, coretesting.Attrs{
				"firewall-mode": config.FwNone,
			}),
		},
	}

	manifold := firewaller.Manifold(validConfig())
	_, err := manifold.Start(ctx)
	c.Assert(err, tc.Equals, dependency.ErrUninstall)
}

type mockDependencyContext struct {
	dependency.Context
	env *mockEnviron
}

func (m *mockDependencyContext) Get(name string, out interface{}) error {
	if name == "environ" {
		*(out.(*environs.Environ)) = m.env
	}
	return nil
}

type mockEnviron struct {
	environs.Environ
	config *config.Config
}

func (e *mockEnviron) Config() *config.Config {
	return e.config
}

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config firewaller.ManifoldConfig
}

func TestManifoldConfigSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validConfig()
}

func validConfig() firewaller.ManifoldConfig {
	return firewaller.ManifoldConfig{
		AgentName:                    "agent",
		APICallerName:                "api-caller",
		EnvironName:                  "environ",
		Logger:                       loggo.GetLogger("test"),
		NewControllerConnection:      func(*api.Info) (api.Connection, error) { return nil, nil },
		NewFirewallerFacade:          func(base.APICaller) (firewaller.FirewallerAPI, error) { return nil, nil },
		NewFirewallerWorker:          func(firewaller.Config) (worker.Worker, error) { return nil, nil },
		NewRemoteRelationsFacade:     func(base.APICaller) *remoterelations.Client { return nil },
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAgentName(c *tc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingEnvironName(c *tc.C) {
	s.config.EnvironName = ""
	s.checkNotValid(c, "empty EnvironName not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerFacade(c *tc.C) {
	s.config.NewFirewallerFacade = nil
	s.checkNotValid(c, "nil NewFirewallerFacade not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerWorker(c *tc.C) {
	s.config.NewFirewallerWorker = nil
	s.checkNotValid(c, "nil NewFirewallerWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewControllerConnection(c *tc.C) {
	s.config.NewControllerConnection = nil
	s.checkNotValid(c, "nil NewControllerConnection not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationsFacade(c *tc.C) {
	s.config.NewRemoteRelationsFacade = nil
	s.checkNotValid(c, "nil NewRemoteRelationsFacade not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewCredentialValidatorFacade(c *tc.C) {
	s.config.NewCredentialValidatorFacade = nil
	s.checkNotValid(c, "nil NewCredentialValidatorFacade not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}
