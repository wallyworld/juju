// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"net/http"
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/packethost/packngo"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/equinix/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type networkSuite struct {
	testhelpers.IsolationSuite
	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
}

func TestNetworkSuite(t *tctesting.T) {
	tc.Run(t, &networkSuite{})
}

func (s *networkSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.provider = NewProvider()
	s.spec = environscloudspec.CloudSpec{
		Type:       "equnix",
		Name:       "equnix metal",
		Region:     "am",
		Endpoint:   "https://api.packet.net/",
		Credential: fakeServicePrincipalCredential(),
	}
}

func (s *networkSuite) TestListIPsByProjectIDAndRegion(c *tc.C) {
	cntrl := gomock.NewController(c)
	projectIP := mocks.NewMockProjectIPService(cntrl)
	projectIP.EXPECT().List(gomock.Eq("12345c2a-6789-4d4f-a3c4-7367d6b7cca8"), &packngo.ListOptions{
		Includes: []string{"available_in_metros"},
	}).Times(1).Return([]packngo.IPAddressReservation{
		{
			Facility: &packngo.Facility{
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
			IpAddressCommon: packngo.IpAddressCommon{
				ID:            "100",
				Address:       "147.75.81.53",
				Gateway:       "147.75.81.52",
				Network:       "147.75.81.52",
				AddressFamily: 4,
				Netmask:       "255.255.255.254",
				Public:        true,
				CIDR:          31,
				Management:    true,
				Manageable:    true,
				Global:        false,
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
		},
		{
			Facility: &packngo.Facility{
				Metro: &packngo.Metro{
					Code: "to",
				},
			},
			IpAddressCommon: packngo.IpAddressCommon{
				ID:            "106",
				Address:       "2604:1380:2000:9d00::3",
				Gateway:       "2604:1380:2000:9d00::2",
				Network:       "2604:1380:2000:9d00::2",
				AddressFamily: 6,
				Netmask:       "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe",
				Public:        true,
				CIDR:          127,
				Management:    true,
				Manageable:    true,
				Global:        false,
				Metro: &packngo.Metro{
					Code: "to",
				},
			},
		},
	}, &packngo.Response{}, nil)
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.ProjectIPs = projectIP
		return cl
	})
	ctx := envtesting.BootstrapTODOContext(c)
	env, err := environs.Open(ctx.Context(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)
	netEnviron, _ := env.(*environ)
	ips, err := netEnviron.listIPsByProjectIDAndRegion("12345c2a-6789-4d4f-a3c4-7367d6b7cca8", netEnviron.cloud.Region)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(ips), tc.DeepEquals, 1)
}

func (s *networkSuite) TestNetworkInterfaces(c *tc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Get(gomock.Eq("100"), nil).Times(2).Return(&packngo.Device{
		ID:   "100",
		Tags: []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		Network: []*packngo.IPAddressAssignment{
			{
				IpAddressCommon: packngo.IpAddressCommon{
					ID:            "100",
					Address:       "147.75.81.53",
					Gateway:       "147.75.81.52",
					Network:       "147.75.81.52",
					AddressFamily: 4,
					Netmask:       "255.255.255.254",
					Public:        true,
					CIDR:          31,
					Management:    true,
					Manageable:    true,
					Global:        false,
					Metro: &packngo.Metro{
						Code: "am",
					},
				},
			},
			{
				IpAddressCommon: packngo.IpAddressCommon{
					ID:            "106",
					Address:       "2604:1380:2000:9d00::3",
					Gateway:       "2604:1380:2000:9d00::2",
					Network:       "2604:1380:2000:9d00::2",
					AddressFamily: 6,
					Netmask:       "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe",
					Public:        true,
					CIDR:          127,
					Management:    true,
					Manageable:    true,
					Global:        false,
					Metro: &packngo.Metro{
						Code: "am",
					},
				},
			},
			{
				IpAddressCommon: packngo.IpAddressCommon{
					ID:            "101",
					Address:       "10.80.96.3",
					Gateway:       "10.80.96.2",
					Network:       "10.80.96.2",
					AddressFamily: 4,
					Netmask:       "255.255.255.254",
					Public:        false,
					CIDR:          31,
					Management:    true,
					Manageable:    true,
					Global:        false,
					Metro: &packngo.Metro{
						Code: "am",
					},
				},
			},
		},
		NetworkPorts: []packngo.Port{
			{
				ID:   "200",
				Name: "bond0",
				Type: "NetworkBondPort",
				Data: packngo.PortData{
					Bonded: true,
				},
				DisbondOperationSupported: true,
				NetworkType:               "layer3",
			},
			{
				ID:   "201",
				Name: "eth0",
				Type: "NetworkPort",
				Data: packngo.PortData{
					Bonded: true,
					MAC:    "24:8a:07:e7:71:70",
				},
				DisbondOperationSupported: true,
				NetworkType:               "layer3",
				Bond: &packngo.BondData{
					ID:   "200",
					Name: "bond0",
				},
			},
			{
				ID:   "202",
				Name: "eth1",
				Type: "NetworkPort",
				Data: packngo.PortData{
					Bonded: true,
					MAC:    "24:8a:07:e7:71:71",
				},
				DisbondOperationSupported: true,
				NetworkType:               "layer3",
				Bond: &packngo.BondData{
					ID:   "200",
					Name: "bond0",
				},
			},
		},
	}, &packngo.Response{
		Response: &http.Response{
			StatusCode: http.StatusOK,
		},
	}, nil)
	projectIP := mocks.NewMockProjectIPService(cntrl)
	projectIP.EXPECT().List(gomock.Eq("12345c2a-6789-4d4f-a3c4-7367d6b7cca8"), &packngo.ListOptions{
		Includes: []string{"available_in_metros"},
	}).Times(1).Return([]packngo.IPAddressReservation{
		{
			Facility: &packngo.Facility{
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
			IpAddressCommon: packngo.IpAddressCommon{
				ID:            "100",
				Address:       "147.75.81.53",
				Gateway:       "147.75.81.52",
				Network:       "147.75.81.52",
				AddressFamily: 4,
				Netmask:       "255.255.255.254",
				Public:        true,
				CIDR:          31,
				Management:    true,
				Manageable:    true,
				Global:        false,
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
		},
		{
			Facility: &packngo.Facility{
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
			IpAddressCommon: packngo.IpAddressCommon{
				ID:            "106",
				Address:       "2604:1380:2000:9d00::3",
				Gateway:       "2604:1380:2000:9d00::2",
				Network:       "2604:1380:2000:9d00::2",
				AddressFamily: 6,
				Netmask:       "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe",
				Public:        true,
				CIDR:          127,
				Management:    true,
				Manageable:    true,
				Global:        false,
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
		},
		{
			Facility: &packngo.Facility{
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
			IpAddressCommon: packngo.IpAddressCommon{
				ID:            "101",
				Address:       "10.80.96.3",
				Gateway:       "10.80.96.2",
				Network:       "10.80.96.2",
				AddressFamily: 4,
				Netmask:       "255.255.255.254",
				Public:        false,
				CIDR:          31,
				Management:    true,
				Manageable:    true,
				Global:        false,
				Metro: &packngo.Metro{
					Code: "am",
				},
			},
		},
	}, &packngo.Response{}, nil)
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		cl.ProjectIPs = projectIP
		return cl
	})
	ctx := envtesting.BootstrapTODOContext(c)
	env, err := environs.Open(ctx.Context(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)
	netEnviron, ok := env.(environs.NetworkingEnviron)
	c.Assert(ok, tc.IsTrue, tc.Commentf("expected environ to implement environs.Networking"))
	ii, err := netEnviron.NetworkInterfaces(&context.CloudCallContext{}, []instance.Id{"100"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(ii[0]), tc.DeepEquals, 3)
}
