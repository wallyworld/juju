// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	tctesting "testing"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type ServiceResourcesSuite struct {
	testhelpers.IsolationSuite
}

func TestServiceResourcesSuite(t *tctesting.T) {
	tc.Run(t, &ServiceResourcesSuite{})
}

func (s *ServiceResourcesSuite) TestUpdatesUploaded(c *tc.C) {
	csRes := newStoreResource(c, "spam", "a-application", 2)
	res := csRes // a copy
	res.Origin = charmresource.OriginUpload
	sr := resources.ApplicationResources{
		Resources: []resources.Resource{
			res,
		},
		CharmStoreResources: []charmresource.Resource{
			csRes.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.HasLen, 0)
}

func (s *ServiceResourcesSuite) TestUpdatesDifferent(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resources.ApplicationResources{
		Resources: []resources.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			spam.Resource,
			expected,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ServiceResourcesSuite) TestUpdatesBadOrdering(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resources.ApplicationResources{
		Resources: []resources.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			expected,
			spam.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ServiceResourcesSuite) TestUpdatesNone(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	sr := resources.ApplicationResources{
		Resources: []resources.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			spam.Resource,
			eggs.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.HasLen, 0)
}

func newStoreResource(c *tc.C, name, applicationID string, revision int) resources.Resource {
	content := name
	opened := resourcetesting.NewResource(c, nil, name, applicationID, content)
	res := opened.Resource
	res.Origin = charmresource.OriginStore
	res.Revision = revision
	err := res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	return res
}
