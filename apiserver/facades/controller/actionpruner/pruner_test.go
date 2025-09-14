// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/actionpruner"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestActionPrunerSuite(t *tctesting.T) {
	tc.Run(t, &ActionPrunerSuite{})
}

type ActionPrunerSuite struct {
	coretesting.BaseSuite

	context facadetest.Context
	api     *actionpruner.API
}

func (s *ActionPrunerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.PatchValue(&actionpruner.Model, func(_ facade.Context) (state.ModelAccessor, error) {
		return nil, nil
	})
	s.context.Auth_ = testing.FakeAuthorizer{Controller: true}

	var err error
	s.api, err = actionpruner.NewAPI(s.context)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ActionPrunerSuite) TestPruneNonController(c *tc.C) {
	s.context.Auth_ = testing.FakeAuthorizer{}
	api, err := actionpruner.NewAPI(s.context)
	c.Assert(err, tc.ErrorIsNil)
	err = api.Prune(params.ActionPruneArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *ActionPrunerSuite) TestPrune(c *tc.C) {
	called := false
	s.PatchValue(&actionpruner.Prune, func(_ <-chan struct{}, st *state.State, maxHistoryTime time.Duration, maxHistoryMB int) error {
		c.Assert(maxHistoryTime, tc.Equals, time.Hour)
		c.Assert(maxHistoryMB, tc.Equals, 666)
		called = true
		return nil
	})
	err := s.api.Prune(params.ActionPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}
