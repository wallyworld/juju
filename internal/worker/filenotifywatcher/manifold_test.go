// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

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

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.NewWatcher = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.NewINotifyWatcher = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewWatcher: func(string, ...Option) (FileWatcher, error) {
			return nil, nil
		},
		NewINotifyWatcher: func() (INotifyWatcher, error) {
			return nil, nil
		},
	}
}
