// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	tctesting "testing"

	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	statetesting "github.com/juju/juju/state/testing"
)

type stateFixture struct {
	testhelpers.IsolationSuite
	statetesting.StateSuite
}

func TestStateFixture(t *tctesting.T) {
	tc.Run(t, &stateFixture{})
}

func (s *stateFixture) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)

	mgotesting.MgoServer.EnableReplicaSet = true
	err := mgotesting.MgoServer.Start(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.IsolationSuite.AddCleanup(func(*tc.C) { mgotesting.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *stateFixture) TearDownSuite(c *tc.C) {
	s.StateSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *stateFixture) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
}

func (s *stateFixture) TearDownTest(c *tc.C) {
	s.StateSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}
