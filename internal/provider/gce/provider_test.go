// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	stdcontext "context"
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	testing.BaseSuite

	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
	config   *config.Config
}

func TestProviderSuite(t *tctesting.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Check(err, tc.ErrorIsNil)

	s.spec = gce.MakeTestCloudSpec()

	uuid := utils.MustNewUUID().String()
	s.config = testing.CustomModelConfig(c, testing.Attrs{
		"uuid": uuid,
		"type": "gce",
	})
}

func (s *providerSuite) TestRegistered(c *tc.C) {
	c.Assert(s.provider, tc.Equals, gce.Provider)
}

func (s *providerSuite) TestOpen(c *tc.C) {
	env, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: s.config,
	})
	c.Check(err, tc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), tc.Equals, "testmodel")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *tc.C) {
	s.spec.Name = ""
	s.testOpenError(c, s.spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *tc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *providerSuite) TestMissingServiceAccount(c *tc.C) {
	credential := cloud.NewCredential(cloud.ServiceAccountAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: missing service account name not valid`)
}

func (s *providerSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: s.config,
	})
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *providerSuite) TestPrepareConfig(c *tc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Config: s.config,
		Cloud:  gce.MakeTestCloudSpec(),
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg, tc.NotNil)
}

func (s *providerSuite) TestValidate(c *tc.C) {
	validCfg, err := s.provider.Validate(s.config, nil)
	c.Check(err, tc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.config.AllAttrs(), tc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestUpgradeConfig(c *tc.C) {
	c.Assert(s.provider, tc.Implements, new(environs.ModelConfigUpgrader))
	upgrader := s.provider.(environs.ModelConfigUpgrader)

	_, ok := s.config.StorageDefaultBlockSource()
	c.Assert(ok, tc.IsFalse)

	outConfig, err := upgrader.UpgradeConfig(s.config)
	c.Assert(err, tc.ErrorIsNil)
	source, ok := outConfig.StorageDefaultBlockSource()
	c.Assert(ok, tc.IsTrue)
	c.Assert(source, tc.Equals, "gce")
}
