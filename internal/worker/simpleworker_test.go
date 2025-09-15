// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type simpleWorkerSuite struct {
	testing.BaseSuite
}

func TestSimpleWorkerSuite(t *tctesting.T) {
	tc.Run(t, &simpleWorkerSuite{})
}

var testError = errors.New("test error")

func (s *simpleWorkerSuite) TestWait(c *tc.C) {
	doWork := func(_ <-chan struct{}) error {
		return testError
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), tc.Equals, testError)
}

func (s *simpleWorkerSuite) TestWaitNil(c *tc.C) {
	doWork := func(_ <-chan struct{}) error {
		return nil
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), tc.Equals, nil)
}

func (s *simpleWorkerSuite) TestKill(c *tc.C) {
	doWork := func(stopCh <-chan struct{}) error {
		<-stopCh
		return testError
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), tc.Equals, testError)

	// test we can kill again without a panic
	w.Kill()
}
