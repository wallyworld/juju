// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase_test

import (
	tctesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/worker/upgradedatabase"
	. "github.com/juju/juju/internal/worker/upgradedatabase/mocks"
	"github.com/juju/juju/state"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.UpgradeDBGateName = ""
	c.Check(cfg.Validate(), tc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.OpenState = nil
	c.Check(cfg.Validate(), tc.Satisfies, errors.IsNotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.Satisfies, errors.IsNotValid)
}

func (s *manifoldSuite) getConfig() upgradedatabase.ManifoldConfig {
	return upgradedatabase.ManifoldConfig{
		AgentName:         "agent-name",
		UpgradeDBGateName: "upgrade-database-lock",
		Logger:            s.logger,
		OpenState:         func() (*state.StatePool, error) { return nil, nil },
		Clock:             clock.WallClock,
	}
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.ignoreLogging(c)

	return ctrl
}
