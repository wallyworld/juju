// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

// ipAddressesInternalSuite contains black-box tests for IP addresses'
// internals, which do not actually access mongo. The rest of the logic is
// tested in ipAddressesStateSuite.
type ipAddressesInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestIpAddressesInternalSuite(t *tctesting.T) {
	tc.Run(t, &ipAddressesInternalSuite{})
}

func (s *ipAddressesInternalSuite) TestNewIPAddressCreatesAddress(c *tc.C) {
	result := newIPAddress(nil, ipAddressDoc{})
	c.Assert(result, tc.NotNil)
	c.Assert(result.st, tc.IsNil)
	c.Assert(result.doc, tc.DeepEquals, ipAddressDoc{})
}

func (s *ipAddressesInternalSuite) TestDocIDIncludesModelUUID(c *tc.C) {
	const localDocID = "foo"
	globalDocID := coretesting.ModelTag.Id() + ":" + localDocID

	result := s.newIPAddressWithDummyState(ipAddressDoc{DocID: localDocID})
	c.Assert(result.DocID(), tc.Equals, globalDocID)

	result = s.newIPAddressWithDummyState(ipAddressDoc{DocID: globalDocID})
	c.Assert(result.DocID(), tc.Equals, globalDocID)
}

func (s *ipAddressesInternalSuite) newIPAddressWithDummyState(doc ipAddressDoc) *Address {
	// We only need the model UUID set for localID() and docID() to work.
	// The rest is tested in ipAddressesStateSuite.
	dummyState := &State{modelTag: coretesting.ModelTag}
	return newIPAddress(dummyState, doc)
}

func (s *ipAddressesInternalSuite) TestProviderIDIsEmptyWhenNotSet(c *tc.C) {
	result := s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Assert(result.ProviderID(), tc.Equals, network.Id(""))
}

func (s *ipAddressesInternalSuite) TestProviderID(c *tc.C) {
	result := s.newIPAddressWithDummyState(ipAddressDoc{ProviderID: "foo"})
	c.Assert(result.ProviderID(), tc.Equals, network.Id("foo"))
}

func (s *ipAddressesInternalSuite) TestIPAddressGlobalKeyHelper(c *tc.C) {
	result := ipAddressGlobalKey("42", "eth0", "0.1.2.3")
	c.Assert(result, tc.Equals, "m#42#d#eth0#ip#0.1.2.3")

	result = ipAddressGlobalKey("", "ignored", "anything")
	c.Assert(result, tc.Equals, "")

	result = ipAddressGlobalKey("ignored", "", "anything")
	c.Assert(result, tc.Equals, "")

	result = ipAddressGlobalKey("", "", "anything")
	c.Assert(result, tc.Equals, "")

	result = ipAddressGlobalKey("", "", "")
	c.Assert(result, tc.Equals, "")
}

func (s *ipAddressesInternalSuite) TestGlobalKeyMethod(c *tc.C) {
	doc := ipAddressDoc{
		MachineID:  "99",
		DeviceName: "br-eth1.250",
		Value:      "fc00:1234::/64",
	}
	address := s.newIPAddressWithDummyState(doc)
	c.Check(address.globalKey(), tc.Equals, "m#99#d#br-eth1.250#ip#fc00:1234::/64")

	address = s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Check(address.globalKey(), tc.Equals, "")
}

func (s *ipAddressesInternalSuite) TestStringIncludesConfigMethodAndValue(c *tc.C) {
	doc := ipAddressDoc{
		ConfigMethod: network.ConfigManual,
		Value:        "0.1.2.3",
		MachineID:    "42",
		DeviceName:   "eno1",
	}
	result := s.newIPAddressWithDummyState(doc)
	expectedString := `manual address "0.1.2.3" of device "eno1" on machine "42"`

	c.Assert(result.String(), tc.Equals, expectedString)

	result = s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Assert(result.String(), tc.Equals, ` address "" of device "" on machine ""`)
}

func (s *ipAddressesInternalSuite) TestRemainingSimpleGetterMethods(c *tc.C) {
	doc := ipAddressDoc{
		DeviceName:       "eth0",
		MachineID:        "42",
		SubnetCIDR:       "10.20.30.0/24",
		ConfigMethod:     network.ConfigStatic,
		Value:            "10.20.30.40",
		DNSServers:       []string{"ns1.example.com", "ns2.example.org"},
		DNSSearchDomains: []string{"example.com", "example.org"},
		GatewayAddress:   "10.20.30.1",
		IsShadow:         true,
		IsSecondary:      true,
	}
	result := s.newIPAddressWithDummyState(doc)

	c.Check(result.DeviceName(), tc.Equals, "eth0")
	c.Check(result.MachineID(), tc.Equals, "42")
	c.Check(result.SubnetCIDR(), tc.Equals, "10.20.30.0/24")
	c.Check(result.ConfigMethod(), tc.Equals, network.ConfigStatic)
	c.Check(result.Value(), tc.Equals, "10.20.30.40")
	c.Check(result.DNSServers(), tc.DeepEquals, []string{"ns1.example.com", "ns2.example.org"})
	c.Check(result.DNSSearchDomains(), tc.DeepEquals, []string{"example.com", "example.org"})
	c.Check(result.GatewayAddress(), tc.Equals, "10.20.30.1")
	c.Check(result.IsShadow(), tc.IsTrue)
	c.Check(result.IsSecondary(), tc.IsTrue)
}
