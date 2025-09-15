// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

// TODO (hml) lxd-profile
// Go back and add additional tests here

type CharmSuite struct {
	ConnSuite
	charm *state.Charm
	curl  string
}

func TestCharmSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &CharmSuite{})
}

func (s *CharmSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.curl = s.charm.URL()
}

func (s *CharmSuite) destroy(c *tc.C) {
	err := s.charm.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CharmSuite) remove(c *tc.C) {
	s.destroy(c)
	err := s.charm.Remove()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CharmSuite) checkRemoved(c *tc.C) {
	_, err := s.State.Charm(s.curl)
	c.Check(err, tc.ErrorMatches, `charm ".*" not found`)
	c.Check(err, tc.Satisfies, errors.IsNotFound)

	// Ensure the document is actually gone.
	coll, closer := state.GetCollection(s.State, "charms")
	defer closer()
	count, err := coll.FindId(s.curl).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func removeUnit(c *tc.C, unit *state.Unit) {
	ensureUnitDead(c, unit)
	err := unit.Remove()
	c.Assert(err, tc.ErrorIsNil)
}

func ensureUnitDead(c *tc.C, unit *state.Unit) {
	err := unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CharmSuite) TestAliveCharm(c *tc.C) {
	s.testCharm(c)
}

func (s *CharmSuite) TestDyingCharm(c *tc.C) {
	s.destroy(c)
	s.testCharm(c)
}

func (s *CharmSuite) testCharm(c *tc.C) {
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dummy.URL(), tc.Equals, s.curl)
	c.Assert(dummy.Revision(), tc.Equals, 1)
	c.Assert(dummy.StoragePath(), tc.Equals, "dummy-path")
	c.Assert(dummy.BundleSha256(), tc.Equals, "quantal-dummy-1-sha256")
	c.Assert(dummy.IsUploaded(), tc.IsTrue)
	meta := dummy.Meta()
	c.Assert(meta.Name, tc.Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], tc.Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the application.",
			Type:        "string",
		},
	)
	actions := dummy.Actions()
	c.Assert(actions, tc.NotNil)
	c.Assert(actions.ActionSpecs, tc.Not(tc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], tc.NotNil)
	c.Assert(actions.ActionSpecs["snapshot"].Params, tc.Not(tc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], tc.DeepEquals,
		charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"type":        "object",
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2",
					},
				},
			},
		})
}

func (s *CharmSuite) TestCharmFromSha256(c *tc.C) {
	ch, err := s.State.Charm(s.curl)
	c.Assert(err, tc.ErrorIsNil)

	dummy, err := s.State.CharmFromSha256(ch.BundleSha256()[0:7])

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dummy.URL(), tc.Equals, s.curl)
	c.Assert(dummy.Revision(), tc.Equals, 1)
	c.Assert(dummy.StoragePath(), tc.Equals, "dummy-path")
	c.Assert(dummy.BundleSha256(), tc.Equals, "quantal-dummy-1-sha256")
	c.Assert(dummy.IsUploaded(), tc.IsTrue)
	meta := dummy.Meta()
	c.Assert(meta.Name, tc.Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], tc.Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the application.",
			Type:        "string",
		},
	)
	actions := dummy.Actions()
	c.Assert(actions, tc.NotNil)
	c.Assert(actions.ActionSpecs, tc.Not(tc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], tc.NotNil)
	c.Assert(actions.ActionSpecs["snapshot"].Params, tc.Not(tc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], tc.DeepEquals,
		charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"type":        "object",
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2",
					},
				},
			},
		})
}

func (s *CharmSuite) TestRemovedCharmNotFound(c *tc.C) {
	s.remove(c)
	s.checkRemoved(c)
}

func (s *CharmSuite) TestRemovedCharmNotListed(c *tc.C) {
	s.remove(c)
	charms, err := s.State.AllCharms()
	c.Check(err, tc.ErrorIsNil)
	c.Check(charms, tc.HasLen, 0)
}

func (s *CharmSuite) TestRemoveWithoutDestroy(c *tc.C) {
	err := s.charm.Remove()
	c.Assert(err, tc.ErrorMatches, "still alive")
}

func (s *CharmSuite) TestCharmNotFound(c *tc.C) {
	curl := "local:anotherseries/dummy-1"
	_, err := s.State.Charm(curl)
	c.Assert(err, tc.ErrorMatches, `charm "local:anotherseries/dummy-1" not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) TestCharmFromSha256NotFound(c *tc.C) {
	_, err := s.State.CharmFromSha256("abcd0123")
	c.Assert(err, tc.ErrorMatches, `charm with sha256 "abcd0123" not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) dummyCharm(c *tc.C, curlOverride string) state.CharmInfo {
	info := state.CharmInfo{
		Charm:       testcharms.Repo.CharmDir("dummy"),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
		Version:     "dummy-146-g725cfd3-dirty",
	}
	if curlOverride != "" {
		info.ID = curlOverride
	} else {
		info.ID = fmt.Sprintf("local:quantal/%s-%d", info.Charm.Meta().Name, info.Charm.Revision())
	}
	return info
}

func (s *CharmSuite) TestRemoveDeletesStorage(c *tc.C) {
	// We normally don't actually set up charm storage in state
	// tests, but we need it here.
	path := s.charm.StoragePath()
	stor := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	err := stor.Put(path, strings.NewReader("abc"), 3)
	c.Assert(err, tc.ErrorIsNil)

	s.destroy(c)
	closer, _, err := stor.Get(path)
	c.Assert(err, tc.ErrorIsNil)
	closer.Close()

	s.remove(c)
	_, _, err = stor.Get(path)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) TestReferenceDyingCharm(c *tc.C) {

	s.destroy(c)

	args := state.AddApplicationArgs{
		Name:  "blah",
		Charm: s.charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}
	_, err := s.State.AddApplication(args)
	c.Check(err, tc.ErrorMatches, `cannot add application "blah": charm: not found or not alive`)
}

func (s *CharmSuite) TestReferenceDyingCharmRace(c *tc.C) {

	defer state.SetBeforeHooks(c, s.State, func() {
		s.destroy(c)
	}).Check()

	args := state.AddApplicationArgs{
		Name:  "blah",
		Charm: s.charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}
	_, err := s.State.AddApplication(args)
	c.Check(err, tc.ErrorMatches, `cannot add application "blah": charm: not found or not alive`)
}

func (s *CharmSuite) TestDestroyReferencedCharm(c *tc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})

	err := s.charm.Destroy()
	c.Check(err, tc.ErrorMatches, "charm in use")
}

func (s *CharmSuite) TestDestroyReferencedCharmRace(c *tc.C) {

	defer state.SetBeforeHooks(c, s.State, func() {
		s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.charm,
		})
	}).Check()

	err := s.charm.Destroy()
	c.Check(err, tc.ErrorMatches, "charm in use")
}

func (s *CharmSuite) TestDestroyUnreferencedCharm(c *tc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	err := app.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = s.charm.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CharmSuite) TestDestroyUnitReferencedCharm(c *tc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
		SetCharmURL: true,
	})

	// set app charm to something different
	info := s.dummyCharm(c, "ch:quantal/dummy-2")
	newCh, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	err = app.SetCharm(state.SetCharmConfig{Charm: newCh, CharmOrigin: defaultCharmOrigin(newCh.URL())})
	c.Assert(err, tc.ErrorIsNil)

	// unit should still reference original charm until updated
	err = s.charm.Destroy()
	c.Assert(err, tc.ErrorMatches, "charm in use")
	err = unit.SetCharmURL(info.ID)
	c.Assert(err, tc.ErrorIsNil)
	err = s.charm.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CharmSuite) TestDestroyFinalUnitReference(c *tc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	c.Logf("calling app.Destroy()")
	c.Assert(app.Destroy(), tc.ErrorIsNil)
	removeUnit(c, unit)

	assertCleanupCount(c, s.State, 2)
	s.checkRemoved(c)
}

func (s *CharmSuite) TestAddCharm(c *tc.C) {
	// Check that adding charms from scratch works correctly.
	info := s.dummyCharm(c, "")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dummy.URL(), tc.Equals, info.ID)

	doc := state.CharmDoc{}
	err = s.charms.FindId(state.DocID(s.State, info.ID)).One(&doc)
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(*doc.URL, tc.DeepEquals, info.ID)

	expVersion := "dummy-146-g725cfd3-dirty"
	c.Assert(doc.CharmVersion, tc.Equals, expVersion)
}

func (s *CharmSuite) TestAddCharmUpdatesPlaceholder(c *tc.C) {
	// Check that adding charms updates any existing placeholder charm
	// with the same URL.
	ch := testcharms.Repo.CharmDir("dummy")

	// Add a placeholder charm.
	curl := charm.MustParseURL("ch:quantal/dummy-1")
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, tc.ErrorIsNil)

	// Add a deployed charm.
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl.String(),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
	}
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dummy.URL(), tc.Equals, curl.String())

	// Charm doc has been updated.
	var docs []state.CharmDoc
	err = s.charms.FindId(state.DocID(s.State, curl.String())).All(&docs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(docs, tc.HasLen, 1)
	c.Assert(*docs[0].URL, tc.DeepEquals, curl.String())
	c.Assert(docs[0].StoragePath, tc.DeepEquals, info.StoragePath)

	// No more placeholder charm.
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) assertPendingCharmExists(c *tc.C, curl string) {
	// Find charm directly and verify only the charm URL and
	// PendingUpload are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl)).One(&doc)
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(*doc.URL, tc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, tc.IsTrue)
	c.Assert(doc.Placeholder, tc.IsFalse)
	c.Assert(doc.Meta, tc.IsNil)
	c.Assert(doc.Config, tc.IsNil)
	c.Assert(doc.StoragePath, tc.Equals, "")
	c.Assert(doc.BundleSha256, tc.Equals, "")
}

func (s *CharmSuite) TestAddCharmWithInvalidMetaData(c *tc.C) {
	check := func(munge func(meta *charm.Meta)) {
		info := s.dummyCharm(c, "")
		meta := info.Charm.Meta()
		munge(meta)
		_, err := s.State.AddCharm(info)
		c.Assert(err, tc.ErrorMatches, `invalid charm data: "\$foo" is not a valid field name`)
	}

	check(func(meta *charm.Meta) {
		meta.Provides = map[string]charm.Relation{"$foo": {}}
	})
	check(func(meta *charm.Meta) {
		meta.Requires = map[string]charm.Relation{"$foo": {}}
	})
	check(func(meta *charm.Meta) {
		meta.Peers = map[string]charm.Relation{"$foo": {}}
	})
}

func (s *CharmSuite) TestPrepareLocalCharmUpload(c *tc.C) {
	// First test the sanity checks.
	curl, err := s.State.PrepareLocalCharmUpload("local:quantal/dummy")
	c.Assert(err, tc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(curl, tc.IsNil)
	curl, err = s.State.PrepareLocalCharmUpload("ch:quantal/dummy")
	c.Assert(err, tc.ErrorMatches, "expected charm URL with local schema, got .*")
	c.Assert(curl, tc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := "local:quantal/missing-123"
	curl, err = s.State.PrepareLocalCharmUpload(testCurl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl.String(), tc.Equals, testCurl)
	s.assertPendingCharmExists(c, curl.String())

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl.String())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Try adding it again with the same revision and ensure it gets bumped.
	curl, err = s.State.PrepareLocalCharmUpload(curl.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl.Revision, tc.Equals, 124)

	// Also ensure the revision cannot decrease.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(42).String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl.Revision, tc.Equals, 125)

	// Check the given revision is respected.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(1234).String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl.Revision, tc.Equals, 1234)
}

func (s *CharmSuite) TestPrepareLocalCharmUploadRemoved(c *tc.C) {
	// Remove the fixture charm and try to re-add it; it gets a new
	// revision.
	s.remove(c)
	curl, err := s.State.PrepareLocalCharmUpload(s.curl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl.Revision, tc.Equals, charm.MustParseURL(s.curl).Revision+1)
}

func (s *CharmSuite) TestPrepareCharmUpload(c *tc.C) {
	// First test the sanity checks.
	sch, err := s.State.PrepareCharmUpload("ch:quantal/dummy")
	c.Assert(err, tc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(sch, tc.IsNil)
	sch, err = s.State.PrepareCharmUpload("local:quantal/dummy")
	c.Assert(err, tc.ErrorMatches, "expected charm URL with a valid schema, got .*")
	c.Assert(sch, tc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := "ch:quantal/missing-123"
	sch, err = s.State.PrepareCharmUpload(testCurl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, testCurl)
	c.Assert(sch.IsUploaded(), tc.IsFalse)

	s.assertPendingCharmExists(c, sch.URL())
	// Make sure we can find it with st.Charm().
	found, err := s.State.Charm(sch.URL())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.URL(), tc.Equals, sch.URL())

	// Try adding it again with the same revision and ensure we get the same document.
	schCopy, err := s.State.PrepareCharmUpload(testCurl)
	c.Assert(err, tc.ErrorIsNil)
	// URL is required to set the charmURL, so the test will succeed.
	_ = schCopy.URL()
	c.Assert(sch, tc.DeepEquals, schCopy)

	// Now add a charm and try again - we should get the same result
	// as with AddCharm.
	info := s.dummyCharm(c, "ch:precise/dummy-2")
	sch, err = s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	schCopy, err = s.State.PrepareCharmUpload(info.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch, tc.DeepEquals, schCopy)
}

func (s *CharmSuite) TestUpdateUploadedCharm(c *tc.C) {
	info := s.dummyCharm(c, "")
	_, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	// Test with already uploaded and a missing charms.
	sch, err := s.State.UpdateUploadedCharm(info)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("charm %q already uploaded", info.ID))
	c.Assert(sch, tc.IsNil)
	info.ID = "local:quantal/missing-1"
	info.SHA256 = "missing"
	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(sch, tc.IsNil)

	// Test with an uploaded local charm.
	_, err = s.State.PrepareLocalCharmUpload(info.ID)
	c.Assert(err, tc.ErrorIsNil)

	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, info.ID)
	c.Assert(sch.Revision(), tc.Equals, charm.MustParseURL(info.ID).Revision)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
	c.Assert(sch.IsPlaceholder(), tc.IsFalse)
	c.Assert(sch.Meta(), tc.DeepEquals, info.Charm.Meta())
	c.Assert(sch.Config(), tc.DeepEquals, info.Charm.Config())
	c.Assert(sch.StoragePath(), tc.DeepEquals, info.StoragePath)
	c.Assert(sch.BundleSha256(), tc.Equals, "missing")
}

func (s *CharmSuite) TestUpdateUploadedCharmEscapesSpecialCharsInConfig(c *tc.C) {
	// Make sure when we have mongodb special characters like "$" and
	// "." in the name of any charm config option, we do proper
	// escaping before storing them and unescaping after loading. See
	// also http://pad.lv/1308146.

	// Clone the dummy charm and change the config.
	configWithProblematicKeys := []byte(`
options:
  $bad.key: {default: bad, description: bad, type: string}
  not.ok.key: {description: not ok, type: int}
  valid-key: {description: all good, type: boolean}
  still$bad.: {description: not good, type: float}
  $.$: {description: awful, type: string}
  ...: {description: oh boy, type: int}
  just$: {description: no no, type: float}
`[1:])
	chDir := testcharms.Repo.ClonedDirPath(c.MkDir(), "dummy")
	err := utils.AtomicWriteFile(
		filepath.Join(chDir, "config.yaml"),
		configWithProblematicKeys,
		0666,
	)
	c.Assert(err, tc.ErrorIsNil)
	ch, err := charm.ReadCharmDir(chDir)
	c.Assert(err, tc.ErrorIsNil)
	missingCurl := "local:quantal/missing-1"
	storagePath := "dummy-1"

	preparedCurl, err := s.State.PrepareLocalCharmUpload(missingCurl)
	c.Assert(err, tc.ErrorIsNil)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          preparedCurl.String(),
		StoragePath: "dummy-1",
		SHA256:      "missing",
	}
	sch, err := s.State.UpdateUploadedCharm(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, missingCurl)
	c.Assert(sch.Revision(), tc.Equals, charm.MustParseURL(missingCurl).Revision)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
	c.Assert(sch.IsPlaceholder(), tc.IsFalse)
	c.Assert(sch.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(sch.Config(), tc.DeepEquals, ch.Config())
	c.Assert(sch.StoragePath(), tc.DeepEquals, storagePath)
	c.Assert(sch.BundleSha256(), tc.Equals, "missing")
}

func (s *CharmSuite) assertPlaceholderCharmExists(c *tc.C, curl string) {
	// Find charm directly and verify only the charm URL and
	// Placeholder are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl)).One(&doc)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*doc.URL, tc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, tc.IsFalse)
	c.Assert(doc.Placeholder, tc.IsTrue)
	c.Assert(doc.Meta, tc.IsNil)
	c.Assert(doc.Config, tc.IsNil)
	c.Assert(doc.StoragePath, tc.Equals, "")
	c.Assert(doc.BundleSha256, tc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) TestUpdateUploadedCharmRejectsInvalidMetadata(c *tc.C) {
	info := s.dummyCharm(c, "")
	_, err := s.State.PrepareLocalCharmUpload(info.ID)
	c.Assert(err, tc.ErrorIsNil)

	meta := info.Charm.Meta()
	meta.Provides = map[string]charm.Relation{
		"foo.bar": {},
	}
	_, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, tc.ErrorMatches, `invalid charm data: "foo.bar" is not a valid field name`)
}

func (s *CharmSuite) TestLatestPlaceholderCharm(c *tc.C) {
	// Add a deployed charm
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	_, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	// Deployed charm not found.
	_, err = s.State.LatestPlaceholderCharm(charm.MustParseURL(info.ID))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Add a charm reference
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2.String())

	// Use a URL with an arbitrary rev to search.
	curl := charm.MustParseURL("ch:quantal/dummy-23")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pending.URL(), tc.Equals, curl2.String())
	c.Assert(pending.IsPlaceholder(), tc.IsTrue)
	c.Assert(pending.Meta(), tc.IsNil)
	c.Assert(pending.Config(), tc.IsNil)
	c.Assert(pending.StoragePath(), tc.Equals, "")
	c.Assert(pending.BundleSha256(), tc.Equals, "")
}

func (s *CharmSuite) TestAddCharmPlaceholderErrors(c *tc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, tc.ErrorMatches, "expected charm URL with a valid schema, got .*")

	curl = charm.MustParseURL("ch:quantal/dummy")
	err = s.State.AddCharmPlaceholder(curl)
	c.Assert(err, tc.ErrorMatches, "expected charm URL with revision, got .*")
}

func (s *CharmSuite) TestAddCharmPlaceholder(c *tc.C) {
	curl := charm.MustParseURL("ch:quantal/dummy-1")
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl.String())

	// Add the same one again, should be a no-op
	err = s.State.AddCharmPlaceholder(curl)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl.String())
}

func (s *CharmSuite) assertAddCharmPlaceholder(c *tc.C) (string, *charm.URL, *state.Charm) {
	// Add a deployed charm
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	// Add a charm placeholder
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2.String())

	// Deployed charm is still there.
	existing, err := s.State.Charm(info.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(existing, tc.DeepEquals, dummy)

	return info.ID, curl2, dummy
}

func (s *CharmSuite) TestAddCharmPlaceholderLeavesDeployedCharmsAlone(c *tc.C) {
	s.assertAddCharmPlaceholder(c)
}

func (s *CharmSuite) TestAddCharmPlaceholderDeletesOlder(c *tc.C) {
	curl, curlOldRef, dummy := s.assertAddCharmPlaceholder(c)

	// Add a new charm placeholder
	curl3 := charm.MustParseURL("ch:quantal/dummy-3")
	err := s.State.AddCharmPlaceholder(curl3)
	c.Assert(err, tc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl3.String())

	// Deployed charm is still there.
	existing, err := s.State.Charm(curl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(existing, tc.DeepEquals, dummy)

	// Older charm placeholder is gone.
	doc := state.CharmDoc{}
	err = s.charms.FindId(curlOldRef).One(&doc)
	c.Assert(err, tc.Equals, mgo.ErrNotFound)
}

func (s *CharmSuite) TestAllCharms(c *tc.C) {
	// Add a deployed charm
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	sch, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	// Add a charm reference
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, tc.ErrorIsNil)

	charms, err := s.State.AllCharms()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charms, tc.HasLen, 3)

	c.Assert(charms[0].URL(), tc.Equals, "local:quantal/quantal-dummy-1")
	c.Assert(charms[1], tc.DeepEquals, sch)
	c.Assert(charms[2].URL(), tc.Equals, curl2.String())
}

func (s *CharmSuite) TestAddCharmMetadata(c *tc.C) {
	// Check that a charm with missing sha/storage path is flagged as pending
	// to be uploaded.
	dummy1 := s.dummyCharm(c, "ch:quantal/dummy-1")
	dummy1.SHA256 = ""
	dummy1.StoragePath = ""
	ch1, err := s.State.AddCharmMetadata(dummy1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch1.IsPlaceholder(), tc.IsFalse)
	c.Check(ch1.IsUploaded(), tc.IsFalse, tc.Commentf("expected charm with missing SHA/storage path to have the PendingUpload flag set"))

	// Check that uploading the same charm ID yields the same charm
	ch, err := s.State.AddCharmMetadata(dummy1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch1, tc.DeepEquals, ch)

	// Check that a charm with populated sha/storage path is flagged as
	// uploaded.
	dummy2 := s.dummyCharm(c, "ch:quantal/dummy-2")
	ch2, err := s.State.AddCharmMetadata(dummy2)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch2.IsPlaceholder(), tc.IsFalse)
	c.Check(ch2.IsUploaded(), tc.IsTrue, tc.Commentf("expected charm with populated SHA/storage path to have the PendingUpload flag unset"))
}

func (s *CharmSuite) TestAddCharmMetadataParallel(c *tc.C) {
	dummy1 := s.dummyCharm(c, "ch:quantal/dummy-1")
	dummy1.SHA256 = ""
	dummy1.StoragePath = ""

	// Below we attempt to add the same charm multiple times in parallel
	// and expect all operations to succeed.
	num := 20
	errors := make(chan error, num)
	for i := 0; i < 20; i++ {
		go func() {
			_, err := s.State.AddCharmMetadata(dummy1)
			select {
			case errors <- err:
			default:
			}
		}()
	}

	for i := 0; i < num; i++ {
		select {
		case err := <-errors:
			c.Check(err, tc.ErrorIsNil)
		case <-time.After(time.Second):
			c.Fatalf("timeout reached")
		}
	}
}

func (s *CharmSuite) TestAddCharmMetadataUpdatesPlaceholder(c *tc.C) {
	// The charm revision updater adds a placeholder charm doc into the db.
	// Ensure that AddCharmMetadata can handle that.
	err := s.State.AddCharmPlaceholder(charm.MustParseURL("ch:quantal/testme-2"))
	c.Assert(err, tc.ErrorIsNil)

	testme := s.dummyCharm(c, "ch:quantal/testme-2")
	ch2, err := s.State.AddCharmMetadata(testme)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch2.IsPlaceholder(), tc.IsFalse)
}

func (s *CharmSuite) TestAllCharmURLs(c *tc.C) {
	ch2 := state.AddTestingCharmhubCharmForSeries(c, s.State, "jammy", "dummy")
	state.AddTestingApplication(c, s.State, "testme-jammy", ch2)

	curls, err := s.State.AllCharmURLs()
	c.Assert(err, tc.ErrorIsNil)
	// One application from SetUpTest
	c.Assert(len(curls), tc.Equals, 2, tc.Commentf("%v", curls))
}

type CharmTestHelperSuite struct {
	ConnSuite
}

func TestCharmTestHelperSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &CharmTestHelperSuite{})
}

func assertCustomCharm(
	c *tc.C,
	ch *state.Charm,
	series string,
	meta *charm.Meta,
	config *charm.Config,
	metrics *charm.Metrics,
	revision int,
) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), tc.DeepEquals, meta)
	c.Assert(ch.Config(), tc.DeepEquals, config)
	c.Assert(ch.Metrics(), tc.DeepEquals, metrics)
	c.Assert(ch.Revision(), tc.DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := charm.MustParseURL(ch.URL())
	c.Assert(url.Series, tc.Equals, series)
	c.Assert(url.Revision, tc.Equals, ch.Revision())

	// Ignore the StoragePath and BundleSHA256 methods, they're irrelevant.
}

func forEachStandardCharm(c *tc.C, f func(name string)) {
	for _, name := range []string{
		"logging", "mysql", "riak", "wordpress",
	} {
		c.Logf("checking %s", name)
		f(name)
	}
}

func (s *CharmTestHelperSuite) TestSimple(c *tc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		config := chd.Config()
		metrics := chd.Metrics()
		revision := chd.Revision()

		ch := s.AddTestingCharm(c, name)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, revision)

		ch = s.AddSeriesCharm(c, name, "bionic")
		assertCustomCharm(c, ch, "bionic", meta, config, metrics, revision)
	})
}

var configYaml = `
options:
  working:
    description: when set to false, prevents application from functioning correctly
    default: true
    type: boolean
`

func (s *CharmTestHelperSuite) TestConfigCharm(c *tc.C) {
	config, err := charm.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	c.Assert(err, tc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		metrics := chd.Metrics()
		ch := s.AddConfigCharm(c, name, configYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

var actionsYaml = `
actions:
   dump:
      description: Dump the database to STDOUT.
      params:
         redirect-file:
            description: Redirect to a log file.
            type: string
`

func (s *CharmTestHelperSuite) TestActionsCharm(c *tc.C) {
	forEachStandardCharm(c, func(name string) {
		actions, err := charm.ReadActionsYaml(name, bytes.NewBuffer([]byte(actionsYaml)))
		c.Assert(err, tc.ErrorIsNil)
		ch := s.AddActionsCharm(c, name, actionsYaml, 123)
		c.Assert(ch.Actions(), tc.DeepEquals, actions)
	})
}

var metaYamlSnippet = `
summary: blah
description: blah blah
`

func (s *CharmTestHelperSuite) TestMetaCharm(c *tc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		config := chd.Config()
		metrics := chd.Metrics()
		metaYaml := "name: " + name + metaYamlSnippet
		meta, err := charm.ReadMeta(bytes.NewBuffer([]byte(metaYaml)))
		c.Assert(err, tc.ErrorIsNil)

		ch := s.AddMetaCharm(c, name, metaYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

func (s *CharmTestHelperSuite) TestLXDProfileCharm(c *tc.C) {
	chd := testcharms.Repo.CharmDir("lxd-profile")
	c.Assert(chd.LXDProfile(), tc.DeepEquals, &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":       "true",
			"security.privileged":    "true",
			"linux.kernel_modules":   "openvswitch,nbd,ip_tables,ip6_tables",
			"environment.http_proxy": "",
		},
		Description: "lxd profile for testing, will pass validation",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
				"type": "unix-char",
			},
			"sony": {
				"type":      "usb",
				"vendorid":  "0fce",
				"productid": "51da",
			},
			"bdisk": {
				"source": "/dev/loop0",
				"type":   "unix-block",
			},
			"gpu": {
				"type": "gpu",
			},
		},
	})
}

var manifestYaml = `
bases:
  - name: ubuntu
    channel: "18.04"
  - name: ubuntu
    channel: "20.04"
`

func (s *CharmTestHelperSuite) TestManifestCharm(c *tc.C) {
	manifest, err := charm.ReadManifest(bytes.NewBuffer([]byte(manifestYaml)))
	c.Assert(err, tc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		ch := s.AddManifestCharm(c, name, manifestYaml, 123)
		c.Assert(ch.Manifest(), tc.DeepEquals, manifest)
	})
}
