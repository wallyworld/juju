// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/pruner"
	"github.com/juju/juju/internal/worker/pruner/mocks"
	"github.com/juju/juju/internal/worker/statushistorypruner"
)

type PrunerSuite struct{}

func TestPrunerSuite(t *tctesting.T) {
	tc.Run(t, &PrunerSuite{})
}

func (s *PrunerSuite) TestRunStop(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"max-status-history-size": "0",
		"max-status-history-age":  "0",
	})
	modelConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)

	facade := mocks.NewMockFacade(ctrl)
	facade.EXPECT().WatchForModelConfigChanges().Return(w, nil)
	facade.EXPECT().ModelConfig().Return(modelConfig, nil).AnyTimes()

	updater, err := statushistorypruner.New(pruner.Config{
		Facade:        facade,
		PruneInterval: 0,
		Clock:         testclock.NewClock(time.Now()),
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}
