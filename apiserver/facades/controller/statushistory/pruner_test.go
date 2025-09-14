// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/statushistory"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestStatusHistoryPrunerSuite(t *tctesting.T) {
	tc.Run(t, &StatusHistoryPrunerSuite{})
}

type StatusHistoryPrunerSuite struct {
	coretesting.BaseSuite

	context facadetest.Context
	api     *statushistory.API
}

func (s *StatusHistoryPrunerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.PatchValue(&statushistory.Model, func(_ facade.Context) (state.ModelAccessor, error) {
		return nil, nil
	})
	s.context.Auth_ = testing.FakeAuthorizer{Controller: true}

	var err error
	s.api, err = statushistory.NewAPI(s.context)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StatusHistoryPrunerSuite) TestPruneNonController(c *tc.C) {
	s.context.Auth_ = testing.FakeAuthorizer{}
	api, err := statushistory.NewAPI(s.context)
	c.Assert(err, tc.ErrorIsNil)
	err = api.Prune(params.StatusHistoryPruneArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *StatusHistoryPrunerSuite) TestPrune(c *tc.C) {
	called := false
	s.PatchValue(&statushistory.Prune, func(_ <-chan struct{}, st *state.State, maxHistoryTime time.Duration, maxHistoryMB int) error {
		c.Assert(maxHistoryTime, tc.Equals, time.Hour)
		c.Assert(maxHistoryMB, tc.Equals, 666)
		called = true
		return nil
	})
	err := s.api.Prune(params.StatusHistoryPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}
