// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resources_mock.go github.com/juju/juju/state Resources

func TestResourcesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ResourcesSuite{})
}

type ResourcesSuite struct {
	ConnSuite

	ch *state.Charm
}

func (s *ResourcesSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.ch = s.ConnSuite.AddTestingCharm(c, "starsay")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "starsay",
		Charm: s.ch,
	})
}

func newResource(c *tc.C, name, data string) resources.Resource {
	opened := resourcetesting.NewResource(c, nil, name, "wordpress", data)
	res := opened.Resource
	res.Timestamp = time.Unix(res.Timestamp.Unix(), 0)
	return res
}

func newResourceFromCharm(ch charm.Charm, name string) resources.Resource {
	return resources.Resource{
		Resource: charmresource.Resource{
			Meta:   ch.Meta().Resources[name],
			Origin: charmresource.OriginUpload,
		},
		ID:            "starsay/" + name,
		ApplicationID: "starsay",
	}
}

func (s *ResourcesSuite) TestListResources(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{spam},
	})
}

func (s *ResourcesSuite) TestListResourcesNoResources(c *tc.C) {
	res := s.State.Resources()
	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultRes.Resources, tc.HasLen, 0)
}

func (s *ResourcesSuite) TestListResourcesIgnorePending(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	ham := newResource(c, "install-resource", "install-resource")
	_, err = res.AddPendingResource("wordpress", "user", ham.Resource)
	c.Assert(err, tc.ErrorIsNil)
	csResources := []charmresource.Resource{spam.Resource}
	err = res.SetCharmStoreResources("wordpress", csResources, testing.NonZeroTime())
	c.Assert(err, tc.ErrorIsNil)

	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources:           []resources.Resource{spam},
		CharmStoreResources: csResources,
	})
}

func (s *ResourcesSuite) TestListPendingResources(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, tc.ErrorIsNil)

	resultRes, err := res.ListPendingResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	ham.PendingID = pendingID
	ham.Username = ""
	ham.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{ham},
	})
}

func (s *ResourcesSuite) TestUpdatePending(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()

	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, tc.ErrorIsNil)

	data := "spamspamspam"
	ham.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	ham.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)

	r, err := res.UpdatePendingResource("wordpress", pendingID, ham.Username, ham.Resource, bytes.NewBufferString(data))
	c.Assert(err, tc.ErrorIsNil)

	ham.Timestamp = r.Timestamp
	ham.PendingID = pendingID
	c.Assert(r, tc.DeepEquals, ham)
}

func (s *ResourcesSuite) TestGetResource(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	_, err := res.GetResource("wordpress", "store-resource")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	r, err := res.GetResource("wordpress", "store-resource")
	c.Assert(err, tc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, tc.DeepEquals, spam)
}

func (s *ResourcesSuite) TestGetPendingResource(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, tc.ErrorIsNil)

	r, err := res.GetPendingResource("wordpress", "install-resource", pendingID)
	c.Assert(err, tc.ErrorIsNil)
	ham.PendingID = pendingID
	ham.Username = ""
	ham.Timestamp = r.Timestamp
	c.Assert(r, tc.DeepEquals, ham)
}

func (s *ResourcesSuite) TestSetResource(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	app, err := s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(app.CharmModifiedVersion(), tc.Equals, 0)

	res := s.State.Resources()

	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)

	_, err = res.AddPendingResource("wordpress", "user", spam.Resource)
	c.Assert(err, tc.ErrorIsNil)
	r, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, tc.DeepEquals, spam)
	c.Assert(r.PendingID, tc.Equals, "")

	r, err = res.GetResource("wordpress", "store-resource")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.PendingID, tc.Equals, "")

	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(app.CharmModifiedVersion(), tc.Equals, 1)
}

func (s *ResourcesSuite) TestSetCharmStoreResources(c *tc.C) {
	res := s.State.Resources()
	updatedRes := newResourceFromCharm(s.ch, "store-resource")
	updatedRes.Revision = 666
	csResources := []charmresource.Resource{updatedRes.Resource}
	err := res.SetCharmStoreResources("starsay", csResources, testing.NonZeroTime())
	c.Assert(err, tc.ErrorIsNil)

	resultRes, err := res.ListResources("starsay")
	c.Assert(err, tc.ErrorIsNil)

	sort.Slice(resultRes.Resources, func(i, j int) bool {
		return resultRes.Resources[i].Name < resultRes.Resources[j].Name
	})
	sort.Slice(resultRes.CharmStoreResources, func(i, j int) bool {
		return resultRes.CharmStoreResources[i].Name < resultRes.CharmStoreResources[j].Name
	})

	expected := []resources.Resource{
		newResourceFromCharm(s.ch, "install-resource"),
		newResourceFromCharm(s.ch, "store-resource"),
		newResourceFromCharm(s.ch, "upload-resource"),
	}
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources: expected,
		CharmStoreResources: []charmresource.Resource{
			expected[0].Resource,
			updatedRes.Resource,
			expected[2].Resource,
		},
	})
}

func (s *ResourcesSuite) TestUnitResource(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	_, err := res.SetUnitResource("wordpress/0", spam.Username, spam.Resource)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	file := bytes.NewBufferString(data)
	_, err = res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	r, err := res.SetUnitResource("wordpress/0", spam.Username, spam.Resource)
	c.Assert(err, tc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, tc.DeepEquals, spam)
	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	resultRes.UnitResources[0].Resources[0].Timestamp = spam.Timestamp
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{spam},
		UnitResources: []resources.UnitResources{{
			Tag:       names.NewUnitTag("wordpress/0"),
			Resources: []resources.Resource{spam},
		}},
	})
}

func (s *ResourcesSuite) TestOpenResource(c *tc.C) {
	app, err := s.State.Application("starsay")
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	res := s.State.Resources()

	_, _, err = res.OpenResource("starsay", "install-resource")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	spam := newResourceFromCharm(s.ch, "install-resource")
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("starsay", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)
	_, err = res.SetUnitResource("starsay/0", spam.Username, spam.Resource)
	c.Assert(err, tc.ErrorIsNil)

	r, rdr, err := res.OpenResource("starsay", "install-resource")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	spam.Timestamp = r.Timestamp
	c.Assert(r, tc.DeepEquals, spam)

	resData, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(resData), tc.Equals, data)

	resultRes, err := res.ListResources("starsay")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultRes.Resources, tc.HasLen, 3)

	sort.Slice(resultRes.Resources, func(i, j int) bool {
		return resultRes.Resources[i].Name < resultRes.Resources[j].Name
	})
	sort.Slice(resultRes.CharmStoreResources, func(i, j int) bool {
		return resultRes.CharmStoreResources[i].Name < resultRes.CharmStoreResources[j].Name
	})

	expected := []resources.Resource{
		newResourceFromCharm(s.ch, "install-resource"),
		newResourceFromCharm(s.ch, "store-resource"),
		newResourceFromCharm(s.ch, "upload-resource"),
	}
	chRes := []charmresource.Resource{
		expected[0].Resource,
		expected[1].Resource,
		expected[2].Resource,
	}
	expected[0].Resource = spam.Resource
	expected[0].Timestamp = resultRes.Resources[0].Timestamp

	resultRes.UnitResources[0].Resources[0].Timestamp = spam.Timestamp

	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		Resources:           expected,
		CharmStoreResources: chRes,
		UnitResources: []resources.UnitResources{{
			Tag:       names.NewUnitTag("starsay/0"),
			Resources: []resources.Resource{spam},
		}},
	})
}

func (s *ResourcesSuite) TestOpenResourceForUniter(c *tc.C) {
	app, err := s.State.Application("starsay")
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	res := s.State.Resources()

	spam := newResourceFromCharm(s.ch, "install-resource")
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("starsay", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)
	_, err = res.SetUnitResource("starsay/0", spam.Username, spam.Resource)
	c.Assert(err, tc.ErrorIsNil)

	unitRes, rdr, err := res.OpenResourceForUniter("starsay/0", "install-resource")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	buf := make([]byte, 2)
	_, err = rdr.Read(buf)
	c.Assert(err, tc.ErrorIsNil)

	resultRes, err := res.ListPendingResources("starsay")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(resultRes.UnitResources, tc.HasLen, 1)
	c.Assert(resultRes.UnitResources[0].Resources, tc.HasLen, 1)
	resultRes.UnitResources[0].Resources[0].PendingID = ""
	c.Assert(resultRes, tc.DeepEquals, resources.ApplicationResources{
		UnitResources: []resources.UnitResources{{
			Tag:              names.NewUnitTag("starsay/0"),
			Resources:        []resources.Resource{unitRes},
			DownloadProgress: map[string]int64{"install-resource": 2},
		}},
	})
}

func (s *ResourcesSuite) TestRemovePendingAppResources(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources()

	spam := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", spam.Username, spam.Resource)
	c.Assert(err, tc.ErrorIsNil)

	// Add some data so we force a cleanup.
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)

	_, err = res.UpdatePendingResource("wordpress", pendingID, spam.Username, spam.Resource, bytes.NewBufferString(data))
	c.Assert(err, tc.ErrorIsNil)

	err = res.RemovePendingAppResources("wordpress", map[string]string{"install-resource": pendingID})
	c.Assert(err, tc.ErrorIsNil)

	resources, err := res.ListPendingResources("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resources.Resources, tc.HasLen, 0)

	state.AssertCleanupsWithKind(c, s.State, "resourceBlob")
}
