// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
)

type GetOpsForHostPortsSuite struct {
	internalStateSuite
}

func TestGetOpsForHostPortsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &GetOpsForHostPortsSuite{})
}

func (s *GetOpsForHostPortsSuite) TestGetOpsForHostPortsChangeWithSpaces(c *tc.C) {
	addresses := map[string]network.SpaceHostPorts{
		"0": {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.113",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
					SpaceID: "foo",
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		},
		"1": {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.62",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
					SpaceID: "foo",
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		},
		"2": {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.56",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
					SpaceID: "foo",
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		},
	}
	// Iterate the map each time to generate the slice with the machines
	// in a different order.
	addressSlice := func() []network.SpaceHostPorts {
		var result []network.SpaceHostPorts
		for _, value := range addresses {
			result = append(result, value)
		}
		return result
	}

	controllers, closer := s.state.db().GetCollection(controllersC)
	defer closer()

	ops, err := s.state.getOpsForHostPortsChange(controllers, apiHostPortsKey, addressSlice())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(ops), tc.GreaterThan, 0)
	// Run the ops.
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, tc.ErrorIsNil)

	// Now iterate over the map a few times to get different ordering, and assert the
	// ops to update the host ports is empty.
	for i := 0; i < 5; i++ {
		ops, err := s.state.getOpsForHostPortsChange(controllers, apiHostPortsKey, addressSlice())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ops, tc.HasLen, 0)
	}
}

type AddressEqualitySuite struct{}

func TestAddressEqualitySuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &AddressEqualitySuite{})
}

func (*AddressEqualitySuite) TestHostPortsEqual(c *tc.C) {
	first := []network.SpaceHostPorts{
		{
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.113",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		}, {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.62",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		}, {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.56",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		},
	}
	// second is the same as first with the first set of machines at the
	// end rather than the start.
	second := []network.SpaceHostPorts{
		{
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.62",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		}, {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.56",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		}, {
			{
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "10.144.9.113",
						Type:  "ipv4",
						Scope: "local-cloud",
					},
				},
				NetPort: 17070,
			}, {
				SpaceAddress: network.SpaceAddress{
					MachineAddress: network.MachineAddress{
						Value: "127.0.0.1",
						Type:  "ipv4",
						Scope: "local-machine",
					},
				},
				NetPort: 17070,
			},
		},
	}
	c.Assert(hostsPortsEqual(first, second), tc.IsTrue)
}

func (s *AddressEqualitySuite) TestAddressConversion(c *tc.C) {
	machineAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "foo",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
			CIDR:  "foo/24",
		},
	}
	stateAddress := fromNetworkAddress(machineAddress, "machine")
	c.Assert(machineAddress, tc.DeepEquals, stateAddress.networkAddress())

	providerAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "bar",
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
			CIDR:  "bar/24",
		},
		SpaceID: "666",
	}
	stateAddress = fromNetworkAddress(providerAddress, "provider")
	c.Assert(providerAddress, tc.DeepEquals, stateAddress.networkAddress())
}
