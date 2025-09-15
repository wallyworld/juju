// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/secretspruner"
	"github.com/juju/juju/internal/worker/secretspruner/mocks"
)

type workerSuite struct {
	testhelpers.IsolationSuite
	logger loggo.Logger

	facade *mocks.MockSecretsFacade

	done      chan struct{}
	changedCh chan []string
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) getWorkerNewer(c *tc.C, calls ...*gomock.Call) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggo.GetLogger("test")
	s.facade = mocks.NewMockSecretsFacade(ctrl)

	s.changedCh = make(chan []string, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchRevisionsToPrune().Return(watchertest.NewMockStringsWatcher(s.changedCh), nil)

	start := func(expectedErr string) {
		w, err := secretspruner.NewWorker(secretspruner.Config{
			Logger:        s.logger,
			SecretsFacade: s.facade,
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(w, tc.NotNil)
		workertest.CheckAlive(c, w)
		s.AddCleanup(func(c *tc.C) {
			if expectedErr == "" {
				workertest.CleanKill(c, w)
			} else {
				err := workertest.CheckKilled(c, w)
				c.Assert(err, tc.ErrorMatches, expectedErr)
			}
		})
		s.waitDone(c)
	}
	return start, ctrl
}

func (s *workerSuite) waitDone(c *tc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestPrune(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	var revisions []string
	revisions = append(revisions, uri1.String()+"/1")
	revisions = append(revisions, uri2.String()+"/1")
	revisions = append(revisions, uri2.String()+"/2")
	revisions = append(revisions, uri3.String()+"/1")
	revisions = append(revisions, uri3.String()+"/2")
	revisions = append(revisions, uri3.String()+"/3")
	s.changedCh <- revisions

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		wg.Wait()
		close(s.done)
	}()

	s.facade.EXPECT().DeleteRevisions(uri1, 1).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})
	s.facade.EXPECT().DeleteRevisions(uri2, 1, 2).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})
	s.facade.EXPECT().DeleteRevisions(uri3, 1, 2, 3).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})

	start("")
}
