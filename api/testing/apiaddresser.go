// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

type APIAddresserTests struct {
	ctrlSt                   *state.State
	state                    *state.State
	facade                   APIAddresserFacade
	waitForModelWatchersIdle func(c *tc.C)
}

func NewAPIAddresserTests(facade APIAddresserFacade, ctrlSt, st *state.State, waitForModelWatchersIdle func(c *tc.C)) *APIAddresserTests {
	return &APIAddresserTests{
		ctrlSt:                   ctrlSt,
		state:                    st,
		facade:                   facade,
		waitForModelWatchersIdle: waitForModelWatchersIdle,
	}
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
	APIHostPorts() ([]network.ProviderHostPorts, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func (s *APIAddresserTests) TestAPIAddresses(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}

	err := s.state.SetAPIHostPorts(hostPorts)
	c.Assert(err, tc.ErrorIsNil)

	addresses, err := s.facade.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *APIAddresserTests) TestAPIHostPorts(c *tc.C) {
	ipv6Addr := network.NewSpaceAddress("2001:DB8::1", network.WithScope(network.ScopeCloudLocal))

	setServerAddrs := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(999, "0.1.2.24"),
		network.NewSpaceHostPorts(1234, "example.com"),
		network.SpaceAddressesWithPort([]network.SpaceAddress{ipv6Addr}, 999),
	}
	err := s.state.SetAPIHostPorts(setServerAddrs)
	c.Assert(err, tc.ErrorIsNil)

	expectServerAddrs := []network.ProviderHostPorts{
		{network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("0.1.2.24").AsProviderAddress(), NetPort: 999}},
		{network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("example.com").AsProviderAddress(), NetPort: 1234}},
		{network.ProviderHostPort{ProviderAddress: network.NewMachineAddress(ipv6Addr.Value).AsProviderAddress(), NetPort: 999}},
	}
	expectServerAddrs[2][0].Scope = network.ScopeCloudLocal

	serverAddrs, err := s.facade.APIHostPorts()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverAddrs, tc.DeepEquals, expectServerAddrs)
}

func (s *APIAddresserTests) TestWatchAPIHostPorts(c *tc.C) {
	hostports, err := s.ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	expectServerAddrs := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(5678, "0.1.2.3"),
	}
	// Make sure we are changing the value
	c.Assert(hostports, tc.Not(tc.DeepEquals), expectServerAddrs)
	s.waitForModelWatchersIdle(c)

	c.Logf("starting api host port watcher")
	w, err := s.facade.WatchAPIHostPorts()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()
	c.Logf("got initial event")

	// Change the state addresses and check that we get a notification
	err = s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertOneChange()
	c.Logf("saw change event")

	// And that we can change it again and see the notification
	expectServerAddrs[0][0].Value = "0.1.99.99"

	err = s.state.SetAPIHostPorts(expectServerAddrs)
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("saw second change event")

	wc.AssertOneChange()
}
