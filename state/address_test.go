// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type AddressSuite struct{}

func TestAddressSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &AddressSuite{})
}

func (s *AddressSuite) TestAddressConversion(c *tc.C) {
	netAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "0.0.0.0",
			Type:  network.IPv4Address,
			Scope: network.ScopeUnknown,
		},
	}
	state.AssertAddressConversion(c, netAddress)
}

func (s *AddressSuite) TestHostPortConversion(c *tc.C) {
	netAddress := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "0.0.0.0",
			Type:  network.IPv4Address,
			Scope: network.ScopeUnknown,
		},
	}
	netHostPort := network.SpaceHostPort{
		SpaceAddress: netAddress,
		NetPort:      4711,
	}
	state.AssertHostPortConversion(c, netHostPort)
}

type ControllerAddressesSuite struct {
	ConnSuite
}

func TestControllerAddressesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ControllerAddressesSuite{})
}

func (s *ControllerAddressesSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	// Make sure there is a machine with manage state in existence.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel, state.JobHostUnits},
		Addresses: network.SpaceAddresses{
			network.NewSpaceAddress("192.168.2.144"),
			network.NewSpaceAddress("10.0.1.2"),
		},
	})
	c.Logf("machine addresses: %#v", machine.Addresses())
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ControllerAddressesSuite) TestControllerModel(c *tc.C) {
	addresses, err := s.State.Addresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *ControllerAddressesSuite) TestOtherModel(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	addresses, err := st.Addresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.SameContents, []string{"10.0.1.2:1234"})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpace(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}, {
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.6.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	newHostPorts = []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      13,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	gotHostPorts, err = s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentSame(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// API host ports are concurrently changed to the same
	// desired value; second arrival will fail its assertion,
	// refresh finding nothing to do, and then issue a
	// read-only assertion that succeeds.
	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts(hostPorts)
		c.Assert(err, tc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, tc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, tc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(prevRevno, tc.Not(tc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Equals, prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Equals, prevAgentsRevno)
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentDifferent(c *tc.C) {
	hostPorts0 := []network.SpaceHostPort{{
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}
	hostPorts1 := []network.SpaceHostPort{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}

	// API host ports are concurrently changed to different
	// values; second arrival will fail its assertion, refresh
	// finding and reattempt.

	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts0})
		c.Assert(err, tc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, tc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, tc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts1})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(prevRevno, tc.Not(tc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Not(tc.Equals), prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Not(tc.Equals), prevAgentsRevno)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	hostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostPorts, tc.DeepEquals, []network.SpaceHostPorts{hostPorts1})

	hostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostPorts, tc.DeepEquals, []network.SpaceHostPorts{hostPorts1})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsWithMgmtSpace(c *tc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	hostPort1 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}
	hostPort2 := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}
	hostPort3 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.4.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}
	newHostPorts := []network.SpaceHostPorts{{hostPort1, hostPort2}, {hostPort3}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	// First slice filtered down to the address in the management space.
	// Second filtered to zero elements, so retains the supplied slice.
	c.Assert(gotHostPorts, tc.DeepEquals, []network.SpaceHostPorts{{hostPort2}, {hostPort3}})
}

func (s *ControllerAddressesSuite) TestSetAPIHostPortsForAgentsNoDocument(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// Delete the addresses for agents document before setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), tc.Equals, mgo.ErrNotFound)

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestAPIHostPortsForAgentsNoDocument(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	// Delete the addresses for agents document after setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), tc.Equals, mgo.ErrNotFound)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *ControllerAddressesSuite) TestWatchAPIHostPortsForClients(c *tc.C) {
	w := s.State.WatchAPIHostPortsForClients()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{network.NewSpaceHostPorts(99, "0.1.2.3")})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertOneChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ControllerAddressesSuite) TestWatchAPIHostPortsForAgents(c *tc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	w := s.State.WatchAPIHostPortsForAgents()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	mgmtHP := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}

	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{{mgmtHP}})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// This should cause no change to APIHostPortsForAgents.
	// We expect only one watcher notification.
	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{{
		mgmtHP,
		network.SpaceHostPort{
			SpaceAddress: network.NewSpaceAddress("0.1.2.3", network.WithScope(network.ScopeCloudLocal)),
			NetPort:      99,
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

type CAASAddressesSuite struct {
	statetesting.StateSuite
}

func TestCAASAddressesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &CAASAddressesSuite{})
}

func (s *CAASAddressesSuite) SetUpTest(c *tc.C) {
	s.ControllerConfig = map[string]interface{}{
		controller.ControllerName: "trump",
	}
	s.StateSuite.SetUpTest(c)
	state.SetModelTypeToCAAS(c, s.State, s.Model)
}

func (s *CAASAddressesSuite) TestAPIHostPortsCloudLocalOnly(c *tc.C) {
	machineAddr := network.MachineAddress{
		Value: "10.10.10.10",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}
	localDNSAddr := network.MachineAddress{
		Value: "controller-service.controller-trump.svc.cluster.local",
		Type:  network.HostName,
		Scope: network.ScopeCloudLocal,
	}

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	_, err = ctrlSt.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses:  network.SpaceAddresses{{MachineAddress: machineAddr}},
	})
	c.Assert(err, tc.ErrorIsNil)

	exp := []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: localDNSAddr},
		NetPort:      17777,
	}, {
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}

	addrs, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, exp)

	exp = []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}
	addrs, err = ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, exp)
}

func (s *CAASAddressesSuite) TestAPIHostPortsPublicOnly(c *tc.C) {
	machineAddr := network.MachineAddress{
		Value: "10.10.10.10",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	localDNSAddr := network.MachineAddress{
		Value: "controller-service.controller-trump.svc.cluster.local",
		Type:  network.HostName,
		Scope: network.ScopeCloudLocal,
	}

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	_, err = ctrlSt.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses:  network.SpaceAddresses{{MachineAddress: machineAddr}},
	})
	c.Assert(err, tc.ErrorIsNil)

	exp := []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: localDNSAddr},
		NetPort:      17777,
	}, {
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}

	addrs, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, exp)

	exp = []network.SpaceHostPorts{{{
		SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr},
		NetPort:      17777,
	}}}
	addrs, err = ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, exp)
}

func (s *CAASAddressesSuite) TestAPIHostPortsMultiple(c *tc.C) {
	machineAddr1 := network.MachineAddress{
		Value: "10.10.10.1",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	machineAddr2 := network.MachineAddress{
		Value: "10.10.10.2",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	machineAddr3 := network.MachineAddress{
		Value: "100.10.10.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}
	machineAddr4 := network.MachineAddress{
		Value: "100.10.10.2",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}
	localDNSAddr := network.MachineAddress{
		Value: "controller-service.controller-trump.svc.cluster.local",
		Type:  network.HostName,
		Scope: network.ScopeCloudLocal,
	}

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	_, err = ctrlSt.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.Model.ControllerUUID(),
		ProviderId: "whatever",
		Addresses: network.SpaceAddresses{
			{MachineAddress: machineAddr1},
			{MachineAddress: machineAddr2},
			{MachineAddress: machineAddr3},
			{MachineAddress: machineAddr4},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)

	// Local-cloud addresses must come first.
	c.Assert(addrs[0][:3], tc.SameContents, network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: localDNSAddr},
			NetPort:      17777,
		},
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr3},
			NetPort:      17777,
		},
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr4},
			NetPort:      17777,
		},
	})

	exp := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr1},
			NetPort:      17777,
		},
		{
			SpaceAddress: network.SpaceAddress{MachineAddress: machineAddr2},
			NetPort:      17777,
		},
	}

	// Public ones should also follow.
	c.Assert(addrs[0][3:], tc.SameContents, exp)

	// Only the public ones should be returned.
	addrs, err = ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, []network.SpaceHostPorts{exp})
}
