// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/applicationscaler"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) TestValidate(c *tc.C) {
	config := applicationscaler.Config{}
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, "nil Facade not valid")
		c.Check(err, tc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := applicationscaler.New(config)
	check(err)
	c.Check(worker, tc.IsNil)
}

func (s *WorkerSuite) TestWatchError(c *tc.C) {
	fix := newFixture(c, errors.New("zap ouch"))
	fix.Run(c, func(worker worker.Worker) {
		err := worker.Wait()
		c.Check(err, tc.ErrorMatches, "zap ouch")
	})
	fix.CheckCallNames(c, "Watch")
}

func (s *WorkerSuite) TestRescaleThenError(c *tc.C) {
	fix := newFixture(c, nil, nil, errors.New("pew squish"))
	fix.Run(c, func(worker worker.Worker) {
		err := worker.Wait()
		c.Check(err, tc.ErrorMatches, "pew squish")
	})
	fix.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Watch",
	}, {
		FuncName: "Rescale",
		Args:     []interface{}{[]string{"expected", "first"}},
	}, {
		FuncName: "Rescale",
		Args:     []interface{}{[]string{"expected", "second"}},
	}})
}
