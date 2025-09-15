// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdcontext "context"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/jujutest"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

const AdminSecret = "admin-secret"

func TestLiveSuite(t *stdtesting.T) {
	testing.MgoTestPackage(t, &liveSuite{
		LiveTests: jujutest.LiveTests{
			TestConfig:     dummy.SampleConfig(),
			CanOpenState:   true,
			HasProvisioner: false,
		},
	})
}

func TestSuite(t *stdtesting.T) {
	testing.MgoTestPackage(t, &suite{
		Tests: jujutest.Tests{
			TestConfig: dummy.SampleConfig(),
		},
	})
}

type liveSuite struct {
	testing.BaseSuite
	mgotesting.MgoSuite
	jujutest.LiveTests
}

func (s *liveSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.LiveTests.SetUpSuite(c)
}

func (s *liveSuite) TearDownSuite(c *tc.C) {
	s.LiveTests.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *liveSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
	s.BaseSuite.PatchValue(&dummy.LogDir, c.MkDir())
}

func (s *liveSuite) TearDownTest(c *tc.C) {
	s.Destroy(c)
	s.LiveTests.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

type suite struct {
	testing.BaseSuite
	mgotesting.MgoSuite
	jujutest.Tests

	callCtx context.ProviderCallContext
}

func (s *suite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *suite) TearDownSuite(c *tc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	s.MgoSuite.SetUpTest(c)
	s.Tests.SetUpTest(c)
	s.PatchValue(&dummy.LogDir, c.MkDir())
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *suite) TearDownTest(c *tc.C) {
	s.Tests.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *tc.C) environs.NetworkingEnviron {
	e, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(stdcontext.TODO(), c),
		s.ControllerStore,
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ModelConfig:      s.TestConfig,
			ControllerName:   s.TestConfig["name"].(string),
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      AdminSecret,
		},
	)
	c.Assert(err, tc.IsNil, tc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(e, tc.NotNil)
	env := e.(environs.Environ)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(stdcontext.TODO(), c), netenv,
		context.NewEmptyCloudCallContext(), bootstrap.BootstrapParams{
			SSHServerHostKey: testing.SSHServerHostKey,
			ControllerConfig: testing.FakeControllerConfig(),
			Cloud: cloud.Cloud{
				Name:      "dummy",
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			},
			AdminSecret:             AdminSecret,
			CAPrivateKey:            testing.CAKey,
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	return netenv
}

func (s *suite) TestAvailabilityZone(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, tc.ErrorIsNil)
	}()

	inst, hwc := jujutesting.AssertStartInstance(c, e, s.callCtx, s.ControllerUUID, "0")
	c.Assert(inst, tc.NotNil)
	c.Check(hwc.AvailabilityZone, tc.NotNil)
}

func (s *suite) TestSupportsSpaces(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, tc.ErrorIsNil)
	}()

	// Without change spaces are supported.
	ok, err := e.SupportsSpaces(s.callCtx)
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)

	// Now turn it off.
	isEnabled := dummy.SetSupportsSpaces(false)
	c.Assert(isEnabled, tc.IsTrue)
	ok, err = e.SupportsSpaces(s.callCtx)
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.Satisfies, errors.IsNotSupported)

	// And finally turn it on again.
	isEnabled = dummy.SetSupportsSpaces(true)
	c.Assert(isEnabled, tc.IsFalse)
	ok, err = e.SupportsSpaces(s.callCtx)
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *suite) TestSupportsSpaceDiscovery(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, tc.ErrorIsNil)
	}()

	// Without change space discovery is not supported.
	ok, err := e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)

	// Now turn it on.
	isEnabled := dummy.SetSupportsSpaceDiscovery(true)
	c.Assert(isEnabled, tc.IsFalse)
	ok, err = e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)

	// And finally turn it off again.
	isEnabled = dummy.SetSupportsSpaceDiscovery(false)
	c.Assert(isEnabled, tc.IsTrue)
	ok, err = e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *suite) breakMethods(c *tc.C, e environs.NetworkingEnviron, names ...string) {
	cfg := e.Config()
	brokenCfg, err := cfg.Apply(map[string]interface{}{
		"broken": strings.Join(names, " "),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = e.SetConfig(brokenCfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *suite) TestNetworkInterfaces(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, tc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := corenetwork.InterfaceInfos{{
		ProviderId:       "dummy-eth0",
		ProviderSubnetId: "dummy-private",
		DeviceIndex:      0,
		InterfaceName:    "eth0",
		InterfaceType:    "ethernet",
		VLANTag:          0,
		MACAddress:       "aa:bb:cc:dd:ee:f0",
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"0.10.0.2", corenetwork.WithCIDR("0.10.0.0/24"), corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		DNSServers:     corenetwork.NewMachineAddresses([]string{"ns1.dummy", "ns2.dummy"}).AsProviderAddresses(),
		GatewayAddress: corenetwork.NewMachineAddress("0.10.0.1").AsProviderAddress(),
		Origin:         corenetwork.OriginProvider,
	}, {
		ProviderId:       "dummy-eth1",
		ProviderSubnetId: "dummy-public",
		DeviceIndex:      1,
		InterfaceName:    "eth1",
		InterfaceType:    "ethernet",
		VLANTag:          1,
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		Disabled:         false,
		NoAutoStart:      true,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"0.20.0.2", corenetwork.WithCIDR("0.20.0.0/24"), corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		DNSServers:     corenetwork.NewMachineAddresses([]string{"ns1.dummy", "ns2.dummy"}).AsProviderAddresses(),
		GatewayAddress: corenetwork.NewMachineAddress("0.20.0.1").AsProviderAddress(),
		Origin:         corenetwork.OriginProvider,
	}, {
		ProviderId:       "dummy-eth2",
		ProviderSubnetId: "dummy-disabled",
		DeviceIndex:      2,
		InterfaceName:    "eth2",
		InterfaceType:    "ethernet",
		VLANTag:          2,
		MACAddress:       "aa:bb:cc:dd:ee:f2",
		Disabled:         true,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"0.30.0.2", corenetwork.WithCIDR("0.30.0.0/24"), corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		DNSServers:     corenetwork.NewMachineAddresses([]string{"ns1.dummy", "ns2.dummy"}).AsProviderAddresses(),
		GatewayAddress: corenetwork.NewMachineAddress("0.30.0.1").AsProviderAddress(),
		Origin:         corenetwork.OriginProvider,
	}}
	infoList, err := e.NetworkInterfaces(s.callCtx, []instance.Id{"i-42"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	info := infoList[0]

	c.Assert(info, tc.DeepEquals, expectInfo)
	assertInterfaces(c, e, opc, "i-42", expectInfo)

	// Test we can induce errors.
	s.breakMethods(c, e, "NetworkInterfaces")
	infoList, err = e.NetworkInterfaces(s.callCtx, []instance.Id{"i-any"})
	c.Assert(err, tc.ErrorMatches, `dummy\.NetworkInterfaces is broken`)
	c.Assert(infoList, tc.HasLen, 0)
}

func (s *suite) TestSubnets(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, tc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []corenetwork.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	ids := []corenetwork.Id{"dummy-private", "dummy-public", "foo-bar"}
	netInfo, err := e.Subnets(s.callCtx, "i-foo", ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netInfo, tc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", ids, expectInfo)

	// Test filtering by id(s).
	netInfo, err = e.Subnets(s.callCtx, "i-foo", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netInfo, tc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", nil, expectInfo)
	netInfo, err = e.Subnets(s.callCtx, "i-foo", ids[0:1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netInfo, tc.DeepEquals, expectInfo[0:1])
	assertSubnets(c, e, opc, "i-foo", ids[0:1], expectInfo[0:1])
	netInfo, err = e.Subnets(s.callCtx, "i-foo", ids[1:])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netInfo, tc.DeepEquals, expectInfo[1:])
	assertSubnets(c, e, opc, "i-foo", ids[1:], expectInfo[1:])

	// Test we can induce errors.
	s.breakMethods(c, e, "Subnets")
	netInfo, err = e.Subnets(s.callCtx, "i-any", nil)
	c.Assert(err, tc.ErrorMatches, `dummy\.Subnets is broken`)
	c.Assert(netInfo, tc.HasLen, 0)
}

func assertInterfaces(c *tc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectInfo corenetwork.InterfaceInfos) {
	select {
	case op := <-opc:
		netOp, ok := op.(dummy.OpNetworkInterfaces)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(netOp.Env, tc.Equals, e.Config().Name())
		c.Check(netOp.InstanceId, tc.Equals, expectInstId)
		c.Check(netOp.Info, tc.DeepEquals, expectInfo)
		return
	case <-time.After(testing.LongWait):
		c.Fatalf("time out wating for operation")
	}
}

func assertSubnets(
	c *tc.C,
	_ environs.Environ,
	opc chan dummy.Operation,
	instId instance.Id,
	subnetIds []corenetwork.Id,
	expectInfo []corenetwork.SubnetInfo,
) {
	select {
	case op := <-opc:
		netOp, ok := op.(dummy.OpSubnets)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(netOp.InstanceId, tc.Equals, instId)
		c.Check(netOp.SubnetIds, tc.DeepEquals, subnetIds)
		c.Check(netOp.Info, tc.DeepEquals, expectInfo)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}
