// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

type dumpSuite struct {
	ConnSuite
}

func TestDumpSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &dumpSuite{})
}

func (s *dumpSuite) TestDumpAll(c *tc.C) {
	// Some of the state workers are responsible for creating
	// collections, so make sure they've started before running
	// the dump.
	state.EnsureWorkersStarted(s.State)

	value, err := s.State.DumpAll()
	c.Assert(err, tc.ErrorIsNil)
	models, ok := value["models"].(map[string]interface{})
	c.Assert(ok, tc.IsTrue)
	c.Assert(models["name"], tc.Equals, "testmodel")

	initialCollections := set.NewStrings()
	for name := range value {
		initialCollections.Add(name)
	}
	// check that there are some other collections there
	c.Check(initialCollections.Contains("modelusers"), tc.IsTrue)
	c.Check(initialCollections.Contains("statuses"), tc.IsTrue)
}
