// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	api "github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/resourceshookcontext"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func TestUnitFacadeSuite(t *tctesting.T) {
	tc.Run(t, &UnitFacadeSuite{})
}

type UnitFacadeSuite struct {
	testhelpers.IsolationSuite

	stub *testhelpers.Stub
}

func (s *UnitFacadeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
}

func (s *UnitFacadeSuite) TestNewUnitFacade(c *tc.C) {
	expected := &stubUnitDataStore{Stub: s.stub}

	uf := resourceshookcontext.NewUnitFacade(expected)

	s.stub.CheckNoCalls(c)
	c.Check(uf.DataStore, tc.Equals, expected)
}

func (s *UnitFacadeSuite) TestGetResourceInfoOkay(c *tc.C) {
	opened1 := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	res1 := opened1.Resource
	opened2 := resourcetesting.NewResource(c, s.stub, "eggs", "a-application", "other data")
	res2 := opened2.Resource
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{res1, res2},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(params.ListUnitResourcesArgs{
		ResourceNames: []string{"spam", "eggs"},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, tc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			Resource: api.Resource2API(res1),
		}, {
			Resource: api.Resource2API(res2),
		}},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoEmpty(c *tc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{opened.Resource},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(params.ListUnitResourcesArgs{
		ResourceNames: []string{},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, tc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoNotFound(c *tc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{opened.Resource},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(params.ListUnitResourcesArgs{
		ResourceNames: []string{"eggs"},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, tc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			ErrorResult: params.ErrorResult{
				Error: apiservererrors.ServerError(errors.NotFoundf(`resource "eggs"`)),
			},
		}},
	})
}
