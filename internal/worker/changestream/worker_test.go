// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestream

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.FileNotifyWatcher = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	cfg = s.getConfig()
	cfg.NewEventQueueWorker = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)
}

func (s *workerSuite) getConfig() WorkerConfig {
	return WorkerConfig{
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		NewEventQueueWorker: func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error) {
			return nil, nil
		},
	}
}

func (s *workerSuite) TestEventQueue(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TrackedDB(), nil)
	s.eventQueueWorker.EXPECT().EventQueue().Return(s.eventQueue)
	s.eventQueueWorker.EXPECT().Kill().AnyTimes()
	s.eventQueueWorker.EXPECT().Wait().MinTimes(1)

	w := s.newWorker(c, 1)
	defer workertest.DirtyKill(c, w)

	stream, ok := w.(ChangeStream)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement ChangeStream"))

	_, err := stream.EventQueue("controller")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestEventQueueCalledTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TrackedDB(), nil)
	s.eventQueueWorker.EXPECT().EventQueue().Return(s.eventQueue).Times(2)
	s.eventQueueWorker.EXPECT().Kill().AnyTimes()
	s.eventQueueWorker.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	w := s.newWorker(c, 1)
	defer workertest.DirtyKill(c, w)

	stream, ok := w.(ChangeStream)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement ChangeStream"))

	// Ensure that the event queue is only created once.
	_, err := stream.EventQueue("controller")
	c.Assert(err, tc.ErrorIsNil)

	_, err = stream.EventQueue("controller")
	c.Assert(err, tc.ErrorIsNil)

	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C, attempts int) worker.Worker {
	cfg := WorkerConfig{
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		NewEventQueueWorker: func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error) {
			attempts--
			if attempts < 0 {
				c.Fatal("NewEventQueueWorker called too many times")
			}
			return s.eventQueueWorker, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	return w
}
