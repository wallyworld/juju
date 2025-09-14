// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *tctesting.T) {
	tc.Run(t, &FacadeSuite{})
}

func (*FacadeSuite) TestFacadeAuthFailure(c *tc.C) {
	facade, err := lifeflag.NewFacade(nil, nil, auth(false))
	c.Check(facade, tc.IsNil)
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
}

func (*FacadeSuite) TestLifeBadEntity(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(entities("archibald snookums"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestLifeAuthFailure(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(entities("unit-foo-1"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestLifeNotFound(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(modelEntity())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))
	c.Check(result.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (*FacadeSuite) TestLifeSuccess(c *tc.C) {
	backend := &mockBackend{exist: true}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Life(modelEntity())
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{Life: life.Dying}},
	})
}

func (*FacadeSuite) TestWatchBadEntity(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(entities("archibald snookums"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestWatchAuthFailure(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(entities("unit-foo-1"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestWatchNotFound(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (*FacadeSuite) TestWatchBadWatcher(c *tc.C) {
	backend := &mockBackend{exist: true}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.ErrorMatches, "blammo")
}

func (*FacadeSuite) TestWatchSuccess(c *tc.C) {
	backend := &mockBackend{exist: true, watch: true}
	resources := common.NewResources()
	facade, err := lifeflag.NewFacade(backend, resources, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}},
	})
}
