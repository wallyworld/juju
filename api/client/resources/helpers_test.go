// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"strings"
	tctesting "testing"
	"time"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

const fingerprint = "123456789012345678901234567890123456789012345678"

type HelpersSuite struct {
	testhelpers.IsolationSuite
}

func TestHelpersSuite(t *tctesting.T) {
	tc.Run(t, &HelpersSuite{})
}

func (HelpersSuite) TestResource2API(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	res := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	apiRes := Resource2API(res)

	c.Check(apiRes, tc.DeepEquals, params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	})
}

func (HelpersSuite) TestAPIResult2ApplicationResourcesOkay(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	expected := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	unitExpected := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "unitspam",
				Type:        charmresource.TypeFile,
				Path:        "unitspam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = unitExpected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}

	unitRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}

	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("boo!"))
	c.Assert(err, tc.ErrorIsNil)

	chRes := params.CharmResource{
		Name:        "unitspam2",
		Type:        "file",
		Path:        "unitspam.tgz2",
		Description: "you need it2",
		Origin:      "upload",
		Revision:    2,
		Fingerprint: fp2.Bytes(),
		Size:        11,
	}

	chExpected := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "unitspam2",
			Type:        charmresource.TypeFile,
			Path:        "unitspam.tgz2",
			Description: "you need it2",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    2,
		Fingerprint: fp2,
		Size:        11,
	}

	res, err := apiResult2ApplicationResources(params.ResourcesResult{
		Resources: []params.Resource{
			apiRes,
		},
		CharmStoreResources: []params.CharmResource{
			chRes,
		},
		UnitResources: []params.UnitResources{
			{
				Entity: params.Entity{
					Tag: "unit-foo-0",
				},
				Resources: []params.Resource{
					unitRes,
				},
				DownloadProgress: map[string]int64{
					unitRes.Name: 8,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	serviceResource := resources.ApplicationResources{
		Resources: []resources.Resource{
			expected,
		},
		CharmStoreResources: []charmresource.Resource{
			chExpected,
		},
		UnitResources: []resources.UnitResources{
			{
				Tag: names.NewUnitTag("foo/0"),
				Resources: []resources.Resource{
					unitExpected,
				},
				DownloadProgress: map[string]int64{
					unitRes.Name: 8,
				},
			},
		},
	}

	c.Check(res, tc.DeepEquals, serviceResource)
}

func (HelpersSuite) TestAPIResult2ApplicationResourcesBadUnitTag(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	expected := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	unitExpected := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "unitspam",
				Type:        charmresource.TypeFile,
				Path:        "unitspam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = unitExpected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}

	unitRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}

	_, err = apiResult2ApplicationResources(params.ResourcesResult{
		Resources: []params.Resource{
			apiRes,
		},
		UnitResources: []params.UnitResources{
			{
				Entity: params.Entity{
					Tag: "THIS IS NOT A GOOD UNIT TAG",
				},
				Resources: []params.Resource{
					unitRes,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorMatches, ".*got bad data from server.*")
}

func (HelpersSuite) TestAPIResult2ApplicationResourcesFailure(c *tc.C) {
	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		ApplicationID: "a-application",
	}
	failure := errors.New("<failure>")

	_, err := apiResult2ApplicationResources(params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: failure.Error(),
			},
		},
		Resources: []params.Resource{
			apiRes,
		},
	})

	c.Check(err, tc.ErrorMatches, "<failure>")
	c.Check(errors.Cause(err), tc.Not(tc.Equals), failure)
}

func (HelpersSuite) TestAPIResult2ApplicationResourcesNotFound(c *tc.C) {
	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		ApplicationID: "a-application",
	}

	_, err := apiResult2ApplicationResources(params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: `application "a-application" not found`,
				Code:    params.CodeNotFound,
			},
		},
		Resources: []params.Resource{
			apiRes,
		},
	})

	c.Check(err, tc.Satisfies, errors.IsNotFound)
}

func (HelpersSuite) TestAPI2Resource(c *tc.C) {
	now := time.Now()
	res, err := API2Resource(params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	})
	c.Assert(err, tc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	expected := resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, expected)
}

func (HelpersSuite) TestCharmResource2API(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "spam",
			Type:        charmresource.TypeFile,
			Path:        "spam.tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        10,
	}
	err = res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	apiInfo := CharmResource2API(res)

	c.Check(apiInfo, tc.DeepEquals, params.CharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need it",
		Origin:      "upload",
		Revision:    1,
		Fingerprint: []byte(fingerprint),
		Size:        10,
	})
}

func (HelpersSuite) TestAPI2CharmResource(c *tc.C) {
	res, err := API2CharmResource(params.CharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need it",
		Origin:      "upload",
		Revision:    1,
		Fingerprint: []byte(fingerprint),
		Size:        10,
	})
	c.Assert(err, tc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	expected := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "spam",
			Type:        charmresource.TypeFile,
			Path:        "spam.tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        10,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, expected)
}

func (HelpersSuite) TestServiceResources2API(c *tc.C) {
	res1 := resourcetesting.NewResource(c, nil, "res1", "a-application", "data").Resource
	res2 := resourcetesting.NewResource(c, nil, "res2", "a-application", "data2").Resource

	tag0 := names.NewUnitTag("a-application/0")
	tag1 := names.NewUnitTag("a-application/1")

	chres1 := res1.Resource
	chres2 := res2.Resource
	chres1.Revision++
	chres2.Revision++

	svcRes := resources.ApplicationResources{
		Resources: []resources.Resource{
			res1,
			res2,
		},
		UnitResources: []resources.UnitResources{
			{
				Tag: tag0,
				Resources: []resources.Resource{
					res1,
					res2,
				},
				DownloadProgress: map[string]int64{
					res2.Name: 2,
				},
			},
			{
				Tag: tag1,
			},
		},
		CharmStoreResources: []charmresource.Resource{
			chres1,
			chres2,
		},
	}

	result := ApplicationResources2APIResult(svcRes)

	apiRes1 := Resource2API(res1)
	apiRes2 := Resource2API(res2)

	apiChRes1 := CharmResource2API(chres1)
	apiChRes2 := CharmResource2API(chres2)

	c.Check(result, tc.DeepEquals, params.ResourcesResult{
		Resources: []params.Resource{
			apiRes1,
			apiRes2,
		},
		UnitResources: []params.UnitResources{
			{
				Entity: params.Entity{
					Tag: "unit-a-application-0",
				},
				Resources: []params.Resource{
					apiRes1,
					apiRes2,
				},
				DownloadProgress: map[string]int64{
					res2.Name: 2,
				},
			},
			{
				// we should have a listing for every unit, even if they
				// have no resources.
				Entity: params.Entity{
					Tag: "unit-a-application-1",
				},
			},
		},
		CharmStoreResources: []params.CharmResource{
			apiChRes1,
			apiChRes2,
		},
	})
}
