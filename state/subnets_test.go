// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type SubnetSuite struct {
	ConnSuite
}

func TestSubnetSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SubnetSuite{})
}

func (s *SubnetSuite) TestAddSubnetSucceedsWithFullyPopulatedInfo(c *tc.C) {
	space, err := s.State.AddSpace("foo", "4", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	fanOverlaySubnetInfo := network.SubnetInfo{
		ProviderId: "foo2",
		CIDR:       "10.0.0.0/8",
		SpaceID:    space.Id(),
	}
	subnet, err := s.State.AddSubnet(fanOverlaySubnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, fanOverlaySubnetInfo)
	subnetInfo := network.SubnetInfo{
		ProviderId:        "foo",
		CIDR:              "192.168.1.0/24",
		VLANTag:           79,
		AvailabilityZones: []string{"Timbuktu"},
		ProviderNetworkId: "wildbirds",
		IsPublic:          true,
	}
	subnetInfo.SetFan("10.0.0.0/8", "172.16.0.0/16")

	subnet, err = s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Set the expected space after adding the subnet to state.
	// When retrieved, it should inherit the space of its underlay.
	subnetInfo.SpaceID = space.Id()

	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	// check it's been stored in state by fetching it back again
	subnetFromDB, err := s.State.SubnetByCIDR("192.168.1.0/24")
	c.Assert(err, tc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnetFromDB, subnetInfo)
}

func (s *SubnetSuite) assertSubnetMatchesInfo(c *tc.C, subnet *state.Subnet, info network.SubnetInfo) {
	c.Assert(subnet.ProviderId(), tc.Equals, info.ProviderId)
	c.Assert(subnet.CIDR(), tc.Equals, info.CIDR)
	c.Assert(subnet.VLANTag(), tc.Equals, info.VLANTag)
	c.Assert(subnet.AvailabilityZones(), tc.DeepEquals, info.AvailabilityZones)
	c.Assert(subnet.String(), tc.Equals, info.CIDR)
	c.Assert(subnet.GoString(), tc.Equals, info.CIDR)
	expectedSubnetID := info.SpaceID
	if expectedSubnetID == "" {
		expectedSubnetID = "0"
	}
	c.Check(subnet.SpaceID(), tc.Equals, expectedSubnetID)
	c.Assert(subnet.ProviderNetworkId(), tc.Equals, info.ProviderNetworkId)
	c.Assert(subnet.FanLocalUnderlay(), tc.Equals, info.FanLocalUnderlay())
	c.Assert(subnet.FanOverlay(), tc.Equals, info.FanOverlay())
	c.Assert(subnet.IsPublic(), tc.Equals, info.IsPublic)
}

func (s *SubnetSuite) TestAddSubnetFailsWithEmptyCIDR(c *tc.C) {
	subnetInfo := network.SubnetInfo{}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "missing CIDR")
}

func (s *SubnetSuite) assertAddSubnetForInfoFailsWithSuffix(c *tc.C, subnetInfo network.SubnetInfo, errorSuffix string) error {
	subnet, err := s.State.AddSubnet(subnetInfo)
	errorMessage := fmt.Sprintf("adding subnet %q: %s", subnetInfo.CIDR, errorSuffix)
	c.Assert(err, tc.ErrorMatches, errorMessage)
	c.Assert(subnet, tc.IsNil)
	return err
}

func (s *SubnetSuite) TestAddSubnetFailsWithInvalidCIDR(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "foobar"}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid CIDR address: foobar")
}

func (s *SubnetSuite) TestAddSubnetFailsWithOutOfRangeVLANTag(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24", VLANTag: 4095}
	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, "invalid VLAN tag 4095: must be between 0 and 4094")
}

func (s *SubnetSuite) TestAddSubnetFailsWithAlreadyExistsForDuplicateCIDRInSameModel(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	err = s.assertAddSubnetForInfoFailsWithSuffix(c, subnetInfo, `subnet "192.168.0.1/24" already exists`)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *SubnetSuite) TestAddSubnetSuccessForDuplicateCIDRDiffProviderIDInSameModel(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSubnetMatchesInfo(c, subnet, subnetInfo)

	subnetInfo.ProviderId = "testme"
	subnet2, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.ID(), tc.Not(tc.Equals), subnet2.ID())
	c.Assert(subnet.CIDR(), tc.Equals, subnet2.CIDR())
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDuplicateCIDRInDifferentModels(c *tc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c *tc.C, info1, info2 network.SubnetInfo, otherState *state.State) (*state.Subnet, *state.Subnet) {
	subnet1, err := otherState.AddSubnet(info1)
	c.Assert(err, tc.ErrorIsNil)
	subnet2, err := s.State.AddSubnet(info2)
	c.Assert(err, tc.ErrorIsNil)

	return subnet1, subnet2
}

func (s *SubnetSuite) TestAddSubnetFailsWhenProviderIdNotUniqueInSameModel(c *tc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}

	s.addTwoSubnetsAndAssertSecondFailsWithSuffix(c, subnetInfo1, subnetInfo2, `provider ID "foo" not unique`)
}

func (s *SubnetSuite) addTwoSubnetsAndAssertSecondFailsWithSuffix(c *tc.C, info1, info2 network.SubnetInfo, errorSuffix string) {
	s.addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c, info1, info2, s.State, errorSuffix)
}

func (s *SubnetSuite) addTwoSubnetsInDifferentModelsAndAssertSecondFailsWithSuffix(c *tc.C, info1, info2 network.SubnetInfo, otherState *state.State, errorSuffix string) {
	_, err := otherState.AddSubnet(info1)
	c.Assert(err, tc.ErrorIsNil)

	_ = s.assertAddSubnetForInfoFailsWithSuffix(c, info2, errorSuffix)
}

func (s *SubnetSuite) TestAddSubnetSucceedsWhenProviderIdNotUniqueInDifferentModels(c *tc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: "foo"}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: "foo"}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInSameModel(c *tc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}

	subnet1, subnet2 := s.addTwoSubnetsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) addTwoSubnetsAssertSuccessAndReturnBoth(c *tc.C, info1, info2 network.SubnetInfo) (*state.Subnet, *state.Subnet) {
	return s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, info1, info2, s.State)
}

func (s *SubnetSuite) TestAddSubnetSucceedsForDifferentCIDRsAndEmptyProviderIdInDifferentModels(c *tc.C) {
	subnetInfo1 := network.SubnetInfo{CIDR: "192.168.0.1/24", ProviderId: ""}
	subnetInfo2 := network.SubnetInfo{CIDR: "10.0.0.0/24", ProviderId: ""}
	subnet1State := s.NewStateForModelNamed(c, "other-model")

	subnet1, subnet2 := s.addTwoSubnetsInDifferentModelsAssertSuccessAndReturnBoth(c, subnetInfo1, subnetInfo2, subnet1State)
	s.assertSubnetMatchesInfo(c, subnet1, subnetInfo1)
	s.assertSubnetMatchesInfo(c, subnet2, subnetInfo2)
}

func (s *SubnetSuite) TestEnsureDeadSetsLifeToDeadWhenAlive(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.refreshAndAssertSubnetLifeIs(c, subnet, state.Dead)
}

func (s *SubnetSuite) addAliveSubnet(c *tc.C, cidr string) *state.Subnet {
	subnetInfo := network.SubnetInfo{CIDR: cidr}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.Life(), tc.Equals, state.Alive)

	return subnet
}

func (s *SubnetSuite) ensureDeadAndAssertLifeIsDead(c *tc.C, subnet *state.Subnet) {
	err := subnet.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.Life(), tc.Equals, state.Dead)
}

func (s *SubnetSuite) refreshAndAssertSubnetLifeIs(c *tc.C, subnet *state.Subnet, expectedLife state.Life) {
	err := subnet.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.Life(), tc.Equals, expectedLife)
}

func (s *SubnetSuite) TestEnsureDeadSetsLifeToDeadWhenNotAlive(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
}

func (s *SubnetSuite) TestRemoveFailsIfStillAlive(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")

	err := subnet.Remove()
	c.Assert(err, tc.ErrorMatches, `cannot remove subnet "192.168.0.1/24": subnet is not dead`)
	s.refreshAndAssertSubnetLifeIs(c, subnet, state.Alive)
}

func (s *SubnetSuite) TestRemoveSucceedsWhenSubnetIsNotAlive(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)

	s.removeSubnetAndAssertNotFound(c, subnet)
}

func (s *SubnetSuite) removeSubnetAndAssertNotFound(c *tc.C, subnet *state.Subnet) {
	err := subnet.Remove()
	c.Assert(err, tc.ErrorIsNil)
	s.assertSubnetWithCIDRNotFound(c, subnet.CIDR())
}

func (s *SubnetSuite) assertSubnetWithCIDRNotFound(c *tc.C, cidr string) {
	_, err := s.State.SubnetByCIDR(cidr)
	s.assertSubnetNotFoundError(c, err)
}

func (s *SubnetSuite) assertSubnetNotFoundError(c *tc.C, err error) {
	c.Assert(err, tc.ErrorMatches, "subnet .* not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SubnetSuite) TestRemoveSucceedsWhenCalledTwice(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.0.1/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.removeSubnetAndAssertNotFound(c, subnet)

	err := subnet.Remove()
	c.Assert(err, tc.ErrorMatches, `cannot remove subnet "192.168.0.1/24": not found or not dead`)
}

func (s *SubnetSuite) TestRefreshUpdatesStaleDocData(c *tc.C) {
	subnet := s.addAliveSubnet(c, "fc00::/64")
	subnetCopy, err := s.State.SubnetByCIDR("fc00::/64")
	c.Assert(err, tc.ErrorIsNil)

	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	c.Assert(subnetCopy.Life(), tc.Equals, state.Alive)

	err = subnetCopy.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnetCopy.Life(), tc.Equals, state.Dead)
}

func (s *SubnetSuite) TestRefreshFailsWithNotFoundWhenRemoved(c *tc.C) {
	subnet := s.addAliveSubnet(c, "192.168.1.0/24")
	s.ensureDeadAndAssertLifeIsDead(c, subnet)
	s.removeSubnetAndAssertNotFound(c, subnet)

	err := subnet.Refresh()
	s.assertSubnetNotFoundError(c, err)
}

func (s *SubnetSuite) TestAllSubnets(c *tc.C) {
	space1, err := s.State.AddSpace("bar", "4", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	space2, err := s.State.AddSpace("notreally", "5", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	subnetInfos := []network.SubnetInfo{
		{CIDR: "192.168.1.0/24"},
		{CIDR: "8.8.8.0/24", SpaceID: space1.Id()},
		{CIDR: "10.0.2.0/24", ProviderId: "foo"},
		{CIDR: "2001:db8::/64", AvailabilityZones: []string{"zone1"}},
		{CIDR: "253.0.0.0/8", SpaceID: space2.Id()},
	}
	subnetInfos[4].SetFan("8.8.8.0/24", "")

	for _, info := range subnetInfos {
		_, err := s.State.AddSubnet(info)
		c.Assert(err, tc.ErrorIsNil)
	}

	subnets, err := s.State.AllSubnets()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, len(subnetInfos))

	for i, subnet := range subnets {
		c.Check(subnet.CIDR(), tc.Equals, subnetInfos[i].CIDR)
		c.Check(subnet.ProviderId(), tc.Equals, subnetInfos[i].ProviderId)
		if subnet.FanLocalUnderlay() == "" {
			expectedSubnetID := subnetInfos[i].SpaceID
			if expectedSubnetID == "" {
				expectedSubnetID = "0"
			}
			c.Check(subnet.SpaceID(), tc.Equals, expectedSubnetID)
		} else {
			// Special case
			c.Check(subnet.SpaceID(), tc.Equals, space1.Id())
		}
		c.Check(subnet.AvailabilityZones(), tc.DeepEquals, subnetInfos[i].AvailabilityZones)
	}
}

func (s *SubnetSuite) TestAllSubnetInfosPopulatesOverlaySpace(c *tc.C) {
	space1, err := s.State.AddSpace("bar", "4", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	subnetInfos := []network.SubnetInfo{
		{CIDR: "8.8.8.0/24", SpaceID: space1.Id()},
		{CIDR: "253.0.0.0/8"},
	}
	subnetInfos[1].SetFan("8.8.8.0/24", "")

	for _, info := range subnetInfos {
		_, err := s.State.AddSubnet(info)
		c.Assert(err, tc.ErrorIsNil)
	}

	subnets, err := s.State.AllSubnetInfos()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, len(subnetInfos))

	for _, subnet := range subnets {
		if subnet.FanLocalUnderlay() == "" {
			c.Check(subnet.SpaceID, tc.Equals, space1.Id())
		}
	}
}

func (s *SubnetSuite) TestUpdateMAASUndefinedSpace(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "8.8.8.0/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddSpace(names.NewSpaceTag("undefined").Id(), "-1", []string{subnet.ID()}, false)
	c.Assert(err, tc.ErrorIsNil)

	subnetInfo.SpaceName = "testme"
	_, err = s.State.AddSpace(names.NewSpaceTag(subnetInfo.SpaceName).Id(), "2", []string{}, false)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Update(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.SpaceName(), tc.Equals, subnetInfo.SpaceName)
}

func (s *SubnetSuite) TestUpdateEmpty(c *tc.C) {
	subnetInfo := network.SubnetInfo{CIDR: "8.8.8.0/24"}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	subnetInfo.VLANTag = 76
	subnetInfo.AvailabilityZones = []string{"testme-az"}
	subnetInfo.SpaceName = "testme"
	_, err = s.State.AddSpace(names.NewSpaceTag(subnetInfo.SpaceName).Id(), "2", []string{}, false)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Update(subnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.VLANTag(), tc.Equals, subnetInfo.VLANTag)
	c.Assert(subnet.AvailabilityZones(), tc.DeepEquals, subnetInfo.AvailabilityZones)
	c.Assert(subnet.SpaceName(), tc.Equals, subnetInfo.SpaceName)
}

func (s *SubnetSuite) TestUpdateNonEmpty(c *tc.C) {
	expectedSubnetInfo := network.SubnetInfo{
		CIDR: "8.8.8.0/24", VLANTag: 42, AvailabilityZones: []string{"changeme-az", "testme-az"}}
	subnet, err := s.State.AddSubnet(expectedSubnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	expectedSpace, err := s.State.AddSpace("changeme", "2", []string{subnet.ID()}, false)
	c.Assert(err, tc.ErrorIsNil)

	newSubnetInfo := network.SubnetInfo{
		CIDR:              subnet.CIDR(),
		SpaceName:         "testme",
		VLANTag:           76,
		AvailabilityZones: []string{"testme-az"},
	}
	_, err = s.State.AddSpace(names.NewSpaceTag(newSubnetInfo.SpaceName).Id(), "7", []string{}, false)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Update(newSubnetInfo)
	c.Assert(err, tc.ErrorIsNil)

	err = subnet.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnet.SpaceID(), tc.Equals, expectedSpace.Id())
	c.Assert(subnet.VLANTag(), tc.Equals, expectedSubnetInfo.VLANTag)
	c.Assert(subnet.AvailabilityZones(), tc.DeepEquals, expectedSubnetInfo.AvailabilityZones)
}

func (s *SubnetSuite) TestUniqueAdditionAndRetrievalByCIDR(c *tc.C) {
	cidr := "1.1.1.1/24"

	sub1 := network.SubnetInfo{
		CIDR:              cidr,
		ProviderId:        "1",
		ProviderNetworkId: "1",
	}
	_, err := s.State.AddSubnet(sub1)
	c.Assert(err, tc.ErrorIsNil)

	sub2 := network.SubnetInfo{
		CIDR:              cidr,
		ProviderId:        "2",
		ProviderNetworkId: "2",
	}
	_, err = s.State.AddSubnet(sub2)
	c.Assert(err, tc.ErrorIsNil)

	subs, err := s.State.SubnetsByCIDR(cidr)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subs, tc.HasLen, 2)

	_, err = s.State.SubnetByCIDR(cidr)
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("multiple subnets matching %q", cidr))
}

func (s *SubnetSuite) TestUpdateSubnetSpaceOps(c *tc.C) {
	space, err := s.State.AddSpace("space-0", "0", []string{}, false)
	c.Assert(err, tc.ErrorIsNil)

	arg := network.SubnetInfo{
		CIDR:              "10.10.10.0/24",
		ProviderId:        "1",
		ProviderNetworkId: "1",
		SpaceID:           space.Id(),
	}

	sub, err := s.State.AddSubnet(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.State.UpdateSubnetSpaceOps(sub.ID(), space.Id()), tc.IsNil)

	ops := s.State.UpdateSubnetSpaceOps(sub.ID(), "666")
	c.Assert(ops, tc.HasLen, 2)
	for _, op := range ops {
		if op.C == "spaces" {
			c.Check(op.Id, tc.Equals, fmt.Sprintf("%s:666", s.State.ModelUUID()))
			c.Check(op.Assert, tc.DeepEquals, txn.DocExists)
		} else if op.C == "subnets" {
			c.Check(op.Id, tc.Equals, fmt.Sprintf("%s:%s", s.State.ModelUUID(), sub.ID()))
			c.Check(op.Update, tc.DeepEquals, bson.D{{"$set", bson.D{{"space-id", "666"}}}})
			c.Check(op.Assert, tc.DeepEquals, bson.D{{"life", state.Alive}})
		} else {
			c.Fatalf("unexpected txn.Op collection: %q", op.C)
		}
	}
}
