// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/upgradeseries"
	. "github.com/juju/juju/internal/worker/upgradeseries/mocks"
)

type upgraderSuite struct {
	testing.BaseSuite

	machineService string

	logger  upgradeseries.Logger
	manager *MockSystemdServiceManager
}

func TestUpgraderSuite(t *tctesting.T) {
	tc.Run(t, &upgraderSuite{})
}

func (s *upgraderSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test.upgrader")
	s.machineService = "jujud-machine-0"
}

func (s *upgraderSuite) TestPerformUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	upg := s.newUpgrader(c)
	c.Assert(upg.PerformUpgrade(), tc.ErrorIsNil)
}

func (s *upgraderSuite) setupMocks(ctrl *gomock.Controller) {
	s.manager = NewMockSystemdServiceManager(ctrl)

	// FindAgents would look through the dataDir to find agents.
	// Here we return unit agents, but they should have no impact on methods.
	s.manager.EXPECT().FindAgents(paths.NixDataDir).Return(s.machineService, []string{"jujud-unit-ubuntu-0", "jujud-unit-redis-0"}, nil, nil)
}

func (s *upgraderSuite) newUpgrader(c *tc.C) upgradeseries.Upgrader {
	upg, err := upgradeseries.NewUpgrader(s.manager, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	return upg
}
