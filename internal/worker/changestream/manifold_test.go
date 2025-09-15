// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	tctesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
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
	cfg.DBAccessor = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.FileNotifyWatcher = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.NewEventQueueWorker = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		DBAccessor:        "dbaccessor",
		FileNotifyWatcher: "filenotifywatcher",
		Clock:             s.clock,
		Logger:            s.logger,
		NewEventQueueWorker: func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error) {
			return nil, nil
		},
	}
}
