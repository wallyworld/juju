// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strconv"
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type bridgePolicySuite struct {
	testhelpers.IsolationSuite

	netBondReconfigureDelay   int
	containerNetworkingMethod string

	spaces network.SpaceInfos
	host   *MockContainer
	guest  *MockContainer
}

func TestBridgePolicySuite(t *tctesting.T) {
	tc.Run(t, &bridgePolicySuite{})
}

func (s *bridgePolicySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.netBondReconfigureDelay = 13
	s.containerNetworkingMethod = "local"
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)

	obtained, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, tc.ErrorIsNil)
	expected := network.SpaceInfos{
		*s.spaces.GetByName("foo"),
		*s.spaces.GetByName("bar"),
	}
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestDetermineContainerNoSpacesConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse(""), nil)

	obtained, err := s.policy().determineContainerSpaces(s.host, s.guest)
	c.Assert(err, tc.ErrorIsNil)
	expected := network.SpaceInfos{
		*s.spaces.GetByName(network.AlphaSpaceName),
	}
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *bridgePolicySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.host = NewMockContainer(ctrl)
	s.guest = NewMockContainer(ctrl)

	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()

	s.spaces = make(network.SpaceInfos, 4)
	for i, space := range []string{network.AlphaSpaceName, "foo", "bar", "fizz"} {
		// 0 is the AlphaSpaceId
		id := strconv.Itoa(i)
		s.spaces[i] = network.SpaceInfo{ID: id, Name: network.SpaceName(space)}
	}
	return ctrl
}

func (s *bridgePolicySuite) policy() *BridgePolicy {
	return &BridgePolicy{
		spaces:                    s.spaces,
		netBondReconfigureDelay:   s.netBondReconfigureDelay,
		containerNetworkingMethod: s.containerNetworkingMethod,
	}
}
