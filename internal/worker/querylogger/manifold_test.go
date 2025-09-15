// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
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

	cfg.LogDir = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		LogDir: "log dir",
		Clock:  s.clock,
		Logger: s.logger,
	}
}
