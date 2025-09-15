// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"context"
	stdcontext "context"
	"fmt"
	"io"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/manual"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testhelpers.Stub
}

func TestProviderSuite(t *tctesting.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, privateKey string, stdin io.Reader, stdout io.Writer) error {
		s.AddCall("InitUbuntuUser", host, user, keys, privateKey, stdin, stdout)
		return s.NextErr()
	})
}

func (s *providerSuite) TestPrepareForBootstrapCloudEndpointAndRegion(c *tc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "endpoint", "region")
	c.Assert(err, tc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "endpoint", "", "", "", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapUserHost(c *tc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "user@host", "")
	c.Assert(err, tc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "host", "user", "", "", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapNoCloudEndpoint(c *tc.C) {
	_, err := s.testPrepareForBootstrap(c, "", "region")
	c.Assert(err, tc.ErrorMatches,
		`missing address of host to bootstrap: please specify "juju bootstrap manual/\[user@\]<host>"`)
}

func (s *providerSuite) testPrepareForBootstrap(c *tc.C, endpoint, region string) (environs.BootstrapContext, error) {
	minimal := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, tc.ErrorIsNil)
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint: endpoint,
		Region:   region,
	}
	testConfig, err = manual.ProviderInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: testConfig,
		Cloud:  cloudSpec,
	})
	if err != nil {
		return nil, err
	}
	env, err := manual.ProviderInstance.Open(stdcontext.TODO(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: testConfig,
	})
	if err != nil {
		return nil, err
	}
	ctx := envtesting.BootstrapContext(context.TODO(), c)
	return ctx, env.PrepareForBootstrap(ctx, "controller-1")
}

func (s *providerSuite) TestNullAlias(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(p, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
	p, err = environs.Provider("null")
	c.Assert(p, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestDisablesUpdatesByDefault(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, tc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	testConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(testConfig.EnableOSRefreshUpdate(), tc.IsTrue)
	c.Check(testConfig.EnableOSUpgrade(), tc.IsTrue)

	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Unless specified, update should default to true,
	// upgrade to false.
	c.Check(validCfg.EnableOSRefreshUpdate(), tc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), tc.IsFalse)
}

func (s *providerSuite) TestDefaultsCanBeOverriden(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, tc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	attrs["enable-os-refresh-update"] = true
	attrs["enable-os-upgrade"] = true

	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Our preferences should not have been overwritten.
	c.Check(validCfg.EnableOSRefreshUpdate(), tc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), tc.IsTrue)
}

func (s *providerSuite) TestSchema(c *tc.C) {
	vals := map[string]interface{}{"endpoint": "http://foo.com/bar"}

	p, err := environs.Provider("manual")
	c.Assert(err, tc.ErrorIsNil)
	err = p.CloudSchema().Validate(vals)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestPingEndpointWithUser(c *tc.C) {
	endpoint := "user@IP"
	called := false
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(s, tc.Equals, endpoint)
		called = true
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Ping(nil, endpoint), tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *providerSuite) TestPingIP(c *tc.C) {
	endpoint := "P"
	called := 0
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(called < 2, tc.IsTrue)
		if called == 0 {
			c.Assert(s, tc.Equals, endpoint)
		} else {
			c.Assert(s, tc.Equals, fmt.Sprintf("ubuntu@%v", endpoint))
		}
		called++
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p.Ping(nil, endpoint), tc.ErrorIsNil)
	// Expect the call to be made twice.
	c.Assert(called, tc.Equals, 1)
}
