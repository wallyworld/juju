// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/equinix"
	"github.com/juju/juju/internal/testhelpers"
)

type providerSuite struct {
	provider environs.CloudEnvironProvider
	testhelpers.IsolationSuite
	dialStub testhelpers.Stub
	callCtx  context.ProviderCallContext
}

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dialStub.ResetCalls()
	s.provider = equinix.NewProvider()
	s.callCtx = context.NewEmptyCloudCallContext()
}

func TestProviderSuite(t *tctesting.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) TestRegistered(c *tc.C) {
	provider, err := environs.Provider("equinix")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provider, tc.NotNil)
}

func (s *providerSuite) TestOpen(c *tc.C) {
	config := fakeConfig(c)
	env, err := environs.Open(context.NewEmptyCloudCallContext(), s.provider, environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: config,
	})
	c.Check(err, tc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), tc.Equals, "testmodel")
}

func (s *providerSuite) TestPrepareConfig(c *tc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Config: fakeConfig(c),
		Cloud:  fakeCloudSpec(),
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg, tc.NotNil)
}

func (s *providerSuite) TestValidate(c *tc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(config, nil)
	c.Assert(err, tc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), tc.DeepEquals, validAttrs)
}
