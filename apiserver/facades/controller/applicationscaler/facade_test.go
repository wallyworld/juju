// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/applicationscaler"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *tctesting.T) {
	tc.Run(t, &FacadeSuite{})
}

func (s *FacadeSuite) TestModelManager(c *tc.C) {
	facade, err := applicationscaler.NewFacade(nil, nil, auth(true))
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (s *FacadeSuite) TestNotModelManager(c *tc.C) {
	facade, err := applicationscaler.NewFacade(nil, nil, auth(false))
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, tc.IsNil)
}

func (s *FacadeSuite) TestWatchError(c *tc.C) {
	fix := newWatchFixture(c, false)
	result, err := fix.Facade.Watch()
	c.Check(err, tc.ErrorMatches, "blammo")
	c.Check(result, tc.DeepEquals, params.StringsWatchResult{})
	c.Check(fix.Resources.Count(), tc.Equals, 0)
}

func (s *FacadeSuite) TestWatchSuccess(c *tc.C) {
	fix := newWatchFixture(c, true)
	result, err := fix.Facade.Watch()
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Changes, tc.DeepEquals, []string{"pow", "zap", "kerblooie"})
	c.Check(fix.Resources.Count(), tc.Equals, 1)
	resource := fix.Resources.Get(result.StringsWatcherId)
	c.Check(resource, tc.NotNil)
}

func (s *FacadeSuite) TestRescaleNonsense(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("burble plink"))
	c.Assert(result.Results, tc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, tc.ErrorMatches, `"burble plink" is not a valid tag`)
}

func (s *FacadeSuite) TestRescaleUnauthorized(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("unit-foo-27"))
	c.Assert(result.Results, tc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, tc.ErrorMatches, "permission denied")
	c.Check(err, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestRescaleNotFound(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("application-missing"))
	c.Assert(result.Results, tc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, tc.ErrorMatches, "application not found")
	c.Check(err, tc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestRescaleError(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("application-error"))
	c.Assert(result.Results, tc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, tc.ErrorMatches, "blammo")
}

func (s *FacadeSuite) TestRescaleSuccess(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("application-expected"))
	c.Assert(result.Results, tc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, tc.IsNil)
}

func (s *FacadeSuite) TestRescaleMultiple(c *tc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(entities("application-error", "application-expected"))
	c.Assert(result.Results, tc.HasLen, 2)
	err0 := result.Results[0].Error
	c.Check(err0, tc.ErrorMatches, "blammo")
	err1 := result.Results[1].Error
	c.Check(err1, tc.IsNil)
}
