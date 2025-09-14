// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type containerTestNetworkLessEnviron struct {
	environs.Environ
}

type containerTestNetworkedEnviron struct {
	environs.NetworkingEnviron

	stub                       *testhelpers.Stub
	supportsContainerAddresses bool
	superSubnets               []string
}

type ContainerNetworkingSuite struct {
	ConnSuite
}

func TestContainerNetworkingSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ContainerNetworkingSuite{})
}

func (e *containerTestNetworkedEnviron) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	e.stub.AddCall("SuperSubnets", ctx)
	return e.superSubnets, e.stub.NextErr()
}

func (e *containerTestNetworkedEnviron) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	e.stub.AddCall("SupportsContainerAddresses", ctx)
	return e.supportsContainerAddresses, e.stub.NextErr()
}

var _ environs.NetworkingEnviron = (*containerTestNetworkedEnviron)(nil)

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingNetworkless(c *tc.C) {
	err := s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{})
	c.Assert(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "local")
	c.Assert(attrs["fan-config"], tc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDoesntChangeDefault(c *tc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "provider",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{})
	c.Assert(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "provider")
	c.Assert(attrs["fan-config"], tc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingAlreadyConfigured(c *tc.C) {
	environ := containerTestNetworkedEnviron{
		stub:         &testhelpers.Stub{},
		superSubnets: []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "local",
		"fan-config":                  "1.2.3.4/24=5.6.7.8/16",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "local")
	c.Assert(attrs["fan-config"], tc.Equals, "1.2.3.4/24=5.6.7.8/16")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingNoSuperSubnets(c *tc.C) {
	environ := containerTestNetworkedEnviron{
		stub: &testhelpers.Stub{},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "local")
	c.Assert(attrs["fan-config"], tc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingSupportsContainerAddresses(c *tc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testhelpers.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "provider")
	c.Assert(attrs["fan-config"], tc.Equals, "172.31.0.0/16=252.0.0.0/8 192.168.1.0/24=253.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDefault(c *tc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testhelpers.Stub{},
		supportsContainerAddresses: false,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "fan")
	c.Assert(attrs["fan-config"], tc.Equals, "172.31.0.0/16=252.0.0.0/8 192.168.1.0/24=253.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingIgnoresIPv6(c *tc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testhelpers.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "2000::dead:beef:1/64"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "provider")
	c.Assert(attrs["fan-config"], tc.Equals, "172.31.0.0/16=252.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingIgnoresNonFan(c *tc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "provider",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	environ := containerTestNetworkedEnviron{
		stub:                       &testhelpers.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err = s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], tc.Equals, "provider")
	c.Assert(attrs["fan-config"], tc.Equals, "")
}
