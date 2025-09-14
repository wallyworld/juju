// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
)

type FlagSuite struct {
	testhelpers.IsolationSuite
}

func TestFlagSuite(t *tctesting.T) {
	tc.Run(t, &FlagSuite{})
}

func (*FlagSuite) TestFlagOutputBadWorker(c *tc.C) {
	in := &stubWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestFlagOutputBadTarget(c *tc.C) {
	in := &stubFlagWorker{}
	var out interface{}
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestFlagOutputSuccess(c *tc.C) {
	in := &stubFlagWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorIsNil)
	c.Check(out, tc.Equals, in)
}

func (*FlagSuite) TestStaticFlagWorker(c *tc.C) {
	testStaticFlagWorker(c, false)
	testStaticFlagWorker(c, true)
}

func testStaticFlagWorker(c *tc.C, value bool) {
	w := engine.NewStaticFlagWorker(value)
	c.Assert(w, tc.NotNil)
	defer workertest.CleanKill(c, w)

	c.Assert(w, tc.Implements, new(engine.Flag))
	c.Assert(w.(engine.Flag).Check(), tc.Equals, value)
}

type stubWorker struct {
	worker.Worker
}

type stubFlagWorker struct {
	engine.Flag
	worker.Worker
}
