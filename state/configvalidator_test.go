// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type ConfigValidatorSuite struct {
	ConnSuite
	configValidator mockConfigValidator
}

func TestConfigValidatorSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ConfigValidatorSuite{})
}

type mockConfigValidator struct {
	validateError error
	validateCfg   *config.Config
	validateOld   *config.Config
	validateValid *config.Config
}

// To test UpdateModelConfig updates state, Validate returns a config
// different to both input configs
func mockValidCfg() (valid *config.Config, err error) {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	if err != nil {
		return nil, err
	}
	valid, err = cfg.Apply(map[string]interface{}{
		"arbitrary-key": "cptn-marvel",
	})
	if err != nil {
		return nil, err
	}
	return valid, nil
}

func (p *mockConfigValidator) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	p.validateCfg = cfg
	p.validateOld = old
	p.validateValid, p.validateError = mockValidCfg()
	return p.validateValid, p.validateError
}

func (s *ConfigValidatorSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.configValidator = mockConfigValidator{}
	s.policy.GetConfigValidator = func() (config.Validator, error) {
		return &s.configValidator, nil
	}
}

func (s *ConfigValidatorSuite) updateModelConfig(c *tc.C) error {
	updateAttrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	return s.Model.UpdateModelConfig(updateAttrs, nil)
}

func (s *ConfigValidatorSuite) TestConfigValidate(c *tc.C) {
	err := s.updateModelConfig(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ConfigValidatorSuite) TestUpdateModelConfigFailsOnConfigValidateError(c *tc.C) {
	var configValidatorErr error
	s.policy.GetConfigValidator = func() (config.Validator, error) {
		configValidatorErr = errors.NotFoundf("")
		return &s.configValidator, configValidatorErr
	}

	err := s.updateModelConfig(c)
	c.Assert(err, tc.ErrorMatches, " not found")
}

func (s *ConfigValidatorSuite) TestUpdateModelConfigUpdatesState(c *tc.C) {
	s.updateModelConfig(c)
	stateCfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	newValidCfg, err := mockValidCfg()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stateCfg.AllAttrs()["arbitrary-key"], tc.Equals, newValidCfg.AllAttrs()["arbitrary-key"])
}

func (s *ConfigValidatorSuite) TestConfigValidateUnimplemented(c *tc.C) {
	var configValidatorErr error
	s.policy.GetConfigValidator = func() (config.Validator, error) {
		return nil, configValidatorErr
	}

	err := s.updateModelConfig(c)
	c.Assert(err, tc.ErrorMatches, "policy returned nil configValidator without an error")
	configValidatorErr = errors.NotImplementedf("Validator")
	err = s.updateModelConfig(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateNoPolicy(c *tc.C) {
	s.policy.GetConfigValidator = func() (config.Validator, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}

	state.SetPolicy(s.State, nil)
	err := s.updateModelConfig(c)
	c.Assert(err, tc.ErrorIsNil)
}
