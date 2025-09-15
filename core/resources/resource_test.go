// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	tctesting "testing"
	"time"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/testhelpers"
)

type ResourceSuite struct {
	testhelpers.IsolationSuite
}

func TestResourceSuite(t *tctesting.T) {
	tc.Run(t, &ResourceSuite{})
}

func (ResourceSuite) TestValidateUploadUsed(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadPending(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *tc.C) {
	var res resources.Resource

	err := res.Validate()

	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadInfo(c *tc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), tc.NotNil)

	res := resources.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateMissingID(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingApplicationID(c *tc.C) {
	res := resources.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-application/spam",
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `.*missing application ID.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *tc.C) {
	res := resources.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Time{},
	}

	err := res.Validate()

	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `.*missing timestamp.*`)
}

func (ResourceSuite) TestRevisionStringNone(c *tc.C) {
	res := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin: charmresource.OriginUpload,
		},
		ApplicationID: "svc",
	}

	err := res.Validate()
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "-")
}

func (ResourceSuite) TestRevisionStringTime(c *tc.C) {
	res := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin: charmresource.OriginUpload,
		},
		ApplicationID: "svc",
		Username:      "a-user",
		Timestamp:     time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "2012-07-08 15:59:05 +0000 UTC")
}

func (ResourceSuite) TestRevisionStringNumber(c *tc.C) {
	res := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin:   charmresource.OriginStore,
			Revision: 7,
		},
		ApplicationID: "svc",
		Username:      "a-user",
		Timestamp:     time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "7")
}

func (s *ResourceSuite) TestAsMap(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	res := []resources.Resource{
		spam,
		eggs,
	}

	resMap := resources.AsMap(res)

	c.Check(resMap, tc.DeepEquals, map[string]resources.Resource{
		"spam": spam,
		"eggs": eggs,
	})
}
