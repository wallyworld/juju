// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type brokerSuite struct {
	testhelpers.IsolationSuite
}

func TestBrokerSuite(t *tctesting.T) {
	tc.Run(t, &brokerSuite{})
}

func (s *brokerSuite) TestAssociateDNSConfigSetsDomainsAndServers(c *tc.C) {
	nics := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("192.168.10.6", network.WithCIDR("192.168.10.0/24")).AsProviderAddress(),
			},
		},
		{
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("192.168.20.6", network.WithCIDR("192.168.20.0/24")).AsProviderAddress(),
			},
		},
	}
	dnsCfg := &network.DNSConfig{
		Nameservers: []network.ProviderAddress{
			network.NewMachineAddress("192.168.20.2").AsProviderAddress(),
			network.NewMachineAddress("192.168.10.2").AsProviderAddress(),
			network.NewMachineAddress("8.8.8.8").AsProviderAddress(),
			network.NewMachineAddress("1.1.1.1").AsProviderAddress(),
		},
		SearchDomains: []string{"example.com"},
	}

	results := associateDNSConfig(nics, dnsCfg)
	c.Assert(results, tc.HasLen, 2)

	c.Assert(results[0].DNSSearchDomains, tc.DeepEquals, []string{"example.com"})
	c.Assert(results[0].DNSServers.Values(), tc.DeepEquals, []string{"192.168.10.2", "8.8.8.8", "1.1.1.1"})

	c.Assert(results[1].DNSSearchDomains, tc.DeepEquals, []string{"example.com"})
	c.Assert(results[1].DNSServers.Values(), tc.DeepEquals, []string{"192.168.20.2", "8.8.8.8", "1.1.1.1"})
}
