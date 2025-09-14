// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

type upgradeModelConfigSuite struct {
	coretesting.BaseSuite
	stub     testhelpers.Stub
	cfg      *config.Config
	reader   upgrades.ModelConfigReader
	updater  upgrades.ModelConfigUpdater
	registry *mockProviderRegistry
}

func TestUpgradeModelConfigSuite(t *tctesting.T) {
	tc.Run(t, &upgradeModelConfigSuite{})
}

func (s *upgradeModelConfigSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = testhelpers.Stub{}
	s.cfg = coretesting.ModelConfig(c)
	s.registry = &mockProviderRegistry{
		providers: make(map[string]environs.EnvironProvider),
	}

	s.reader = environConfigFunc(func() (*config.Config, error) {
		s.stub.AddCall("ModelConfig")
		return s.cfg, s.stub.NextErr()
	})

	s.updater = updateModelConfigFunc(func(
		update map[string]interface{}, remove []string, validate ...state.ValidateConfigFunc,
	) error {
		s.stub.AddCall("UpdateModelConfig", update, remove, validate)
		return s.stub.NextErr()
	})
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigModelConfigError(c *tc.C) {
	s.stub.SetErrors(errors.New("cannot read environ config"))
	err := upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorMatches, "reading model config: cannot read environ config")
	s.stub.CheckCallNames(c, "ModelConfig")
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigProviderNotRegistered(c *tc.C) {
	s.registry.SetErrors(errors.New(`no registered provider for "someprovider"`))
	err := upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorMatches, `getting provider: no registered provider for "someprovider"`)
	s.stub.CheckCallNames(c, "ModelConfig")
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigProviderNotConfigUpgrader(c *tc.C) {
	s.registry.providers["someprovider"] = &mockEnvironProvider{}
	err := upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorIsNil)
	s.registry.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Provider", Args: []interface{}{"someprovider"},
	}})
	s.stub.CheckCallNames(c, "ModelConfig")
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigProviderConfigUpgrader(c *tc.C) {
	var err error
	s.cfg, err = s.cfg.Apply(map[string]interface{}{"test-key": "test-value"})
	c.Assert(err, tc.ErrorIsNil)

	s.registry.providers["someprovider"] = &mockModelConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return cfg.Remove([]string{"test-key"})
		},
	}
	err = upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ModelConfig", "UpdateModelConfig")
	updateCall := s.stub.Calls()[1]
	expectedAttrs := s.cfg.AllAttrs()
	delete(expectedAttrs, "test-key")
	c.Assert(updateCall.Args, tc.HasLen, 3)
	c.Assert(updateCall.Args[0], tc.DeepEquals, expectedAttrs)
	c.Assert(updateCall.Args[1], tc.SameContents, []string{"test-key"})
	c.Assert(updateCall.Args[2], tc.IsNil)
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigUpgradeConfigError(c *tc.C) {
	s.registry.providers["someprovider"] = &mockModelConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return nil, errors.New("cannot upgrade config")
		},
	}
	err := upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorMatches, "upgrading config: cannot upgrade config")
	s.stub.CheckCallNames(c, "ModelConfig")
}

func (s *upgradeModelConfigSuite) TestUpgradeModelConfigUpdateConfigError(c *tc.C) {
	s.stub.SetErrors(nil, errors.New("cannot update environ config"))
	s.registry.providers["someprovider"] = &mockModelConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return cfg, nil
		},
	}
	err := upgrades.UpgradeModelConfig(s.reader, s.updater, s.registry)
	c.Assert(err, tc.ErrorMatches, "updating config in state: cannot update environ config")

	s.stub.CheckCallNames(c, "ModelConfig", "UpdateModelConfig")
	updateCall := s.stub.Calls()[1]
	c.Assert(updateCall.Args, tc.HasLen, 3)
	c.Assert(updateCall.Args[0], tc.DeepEquals, s.cfg.AllAttrs())
	c.Assert(updateCall.Args[1], tc.IsNil)
	c.Assert(updateCall.Args[2], tc.IsNil)
}

type environConfigFunc func() (*config.Config, error)

func (f environConfigFunc) ModelConfig() (*config.Config, error) {
	return f()
}

type updateModelConfigFunc func(map[string]interface{}, []string, ...state.ValidateConfigFunc) error

func (f updateModelConfigFunc) UpdateModelConfig(
	update map[string]interface{}, remove []string, validate ...state.ValidateConfigFunc,
) error {
	return f(update, remove, validate...)
}

type mockProviderRegistry struct {
	environs.ProviderRegistry
	testhelpers.Stub
	providers map[string]environs.EnvironProvider
}

func (r *mockProviderRegistry) Provider(name string) (environs.EnvironProvider, error) {
	r.MethodCall(r, "Provider", name)
	return r.providers[name], r.NextErr()
}

type mockEnvironProvider struct {
	testhelpers.Stub
	environs.EnvironProvider
}

type mockModelConfigUpgrader struct {
	mockEnvironProvider
	upgradeConfig func(*config.Config) (*config.Config, error)
}

func (u *mockModelConfigUpgrader) UpgradeConfig(cfg *config.Config) (*config.Config, error) {
	u.MethodCall(u, "UpgradeConfig", cfg)
	return u.upgradeConfig(cfg)
}
