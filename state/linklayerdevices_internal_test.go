// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

// linkLayerDevicesInternalSuite contains white-box tests for link-layer network
// devices' internals, which do not actually access mongo. The rest of the logic
// is tested in linkLayerDevicesStateSuite.
type linkLayerDevicesInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestLinkLayerDevicesInternalSuite(t *tctesting.T) {
	tc.Run(t, &linkLayerDevicesInternalSuite{})
}

func (s *linkLayerDevicesInternalSuite) TestNewLinkLayerDeviceCreatesLinkLayerDevice(c *tc.C) {
	result := newLinkLayerDevice(nil, linkLayerDeviceDoc{})
	c.Assert(result, tc.NotNil)
	c.Assert(result.st, tc.IsNil)
	c.Assert(result.doc, tc.DeepEquals, linkLayerDeviceDoc{})
}

func (s *linkLayerDevicesInternalSuite) TestDocIDIncludesModelUUID(c *tc.C) {
	const localDocID = "foo"
	globalDocID := coretesting.ModelTag.Id() + ":" + localDocID

	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{DocID: localDocID})
	c.Assert(result.DocID(), tc.Equals, globalDocID)

	result = s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{DocID: globalDocID})
	c.Assert(result.DocID(), tc.Equals, globalDocID)
}

func (s *linkLayerDevicesInternalSuite) newLinkLayerDeviceWithDummyState(doc linkLayerDeviceDoc) *LinkLayerDevice {
	// We only need the model UUID set for localID() and docID() to work.
	// The rest is tested in linkLayerDevicesStateSuite.
	dummyState := &State{modelTag: coretesting.ModelTag}
	return newLinkLayerDevice(dummyState, doc)
}

func (s *linkLayerDevicesInternalSuite) TestProviderIDIsEmptyWhenNotSet(c *tc.C) {
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{})
	c.Assert(result.ProviderID(), tc.Equals, network.Id(""))
}

func (s *linkLayerDevicesInternalSuite) TestProviderIDDoesNotIncludeModelUUIDWhenSet(c *tc.C) {
	const localProviderID = "foo"
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{ProviderID: localProviderID})
	c.Assert(result.ProviderID(), tc.Equals, network.Id(localProviderID))
}

func (s *linkLayerDevicesInternalSuite) TestParentDeviceReturnsNoErrorWhenParentNameNotSet(c *tc.C) {
	result := s.newLinkLayerDeviceWithDummyState(linkLayerDeviceDoc{})
	parent, err := result.ParentDevice()
	c.Check(parent, tc.IsNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *linkLayerDevicesInternalSuite) TestLinkLayerDeviceGlobalKeyHelper(c *tc.C) {
	result := linkLayerDeviceGlobalKey("42", "eno1")
	c.Assert(result, tc.Equals, "m#42#d#eno1")

	result = linkLayerDeviceGlobalKey("", "")
	c.Assert(result, tc.Equals, "")
}

func (s *linkLayerDevicesInternalSuite) TestParseLinkLayerParentNameAsGlobalKey(c *tc.C) {
	for i, test := range []struct {
		about              string
		input              string
		expectedError      string
		expectedMachineID  string
		expectedParentName string
	}{{
		about: "empty input - empty outputs and no error",
		input: "",
	}, {
		about: "name only as input - empty outputs and no error",
		input: "some-parent",
	}, {
		about:              "global key as input - parsed outputs and no error",
		input:              "m#42#d#br-eth1",
		expectedMachineID:  "42",
		expectedParentName: "br-eth1",
	}, {
		about:         "invalid name as input - empty outputs and NotValidError",
		input:         "some name with not enough # in it",
		expectedError: `ParentName "some name with not enough # in it" format not valid`,
	}, {
		about:         "almost a global key as input - empty outputs and NotValidError",
		input:         "x#foo#y#bar",
		expectedError: `ParentName "x#foo#y#bar" format not valid`,
	}} {
		c.Logf("test #%d: %q", i, test.about)
		gotMachineID, gotParentName, gotError := parseLinkLayerDeviceParentNameAsGlobalKey(test.input)
		if test.expectedError != "" {
			c.Check(gotError, tc.ErrorMatches, test.expectedError)
			c.Check(gotError, tc.Satisfies, errors.IsNotValid)
		} else {
			c.Check(gotError, tc.ErrorIsNil)
		}
		c.Check(gotMachineID, tc.Equals, test.expectedMachineID)
		c.Check(gotParentName, tc.Equals, test.expectedParentName)
	}
}

func (s *linkLayerDevicesInternalSuite) TestStringIncludesTypeNameAndMachineID(c *tc.C) {
	doc := linkLayerDeviceDoc{
		MachineID: "42",
		Name:      "foo",
		Type:      network.BondDevice,
	}
	result := s.newLinkLayerDeviceWithDummyState(doc)
	expectedString := `bond device "foo" on machine "42"`

	c.Assert(result.String(), tc.Equals, expectedString)
}

func (s *linkLayerDevicesInternalSuite) TestRemainingSimpleGetterMethods(c *tc.C) {
	doc := linkLayerDeviceDoc{
		Name:        "bond0",
		MachineID:   "99",
		MTU:         uint(9000),
		Type:        network.BondDevice,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
		ParentName:  "br-bond0",
	}
	result := s.newLinkLayerDeviceWithDummyState(doc)

	c.Check(result.Name(), tc.Equals, "bond0")
	c.Check(result.MachineID(), tc.Equals, "99")
	c.Check(result.MTU(), tc.Equals, uint(9000))
	c.Check(result.Type(), tc.Equals, network.BondDevice)
	c.Check(result.MACAddress(), tc.Equals, "aa:bb:cc:dd:ee:f0")
	c.Check(result.IsAutoStart(), tc.IsTrue)
	c.Check(result.IsUp(), tc.IsTrue)
	c.Check(result.ParentName(), tc.Equals, "br-bond0")
}
