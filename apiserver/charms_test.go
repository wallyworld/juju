// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/apiserver/common"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

type charmsSuite struct {
	apiserverBaseSuite
}

func TestCharmsSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &charmsSuite{})
}

func (s *charmsSuite) charmsURL(query string) *url.URL {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.State.ModelUUID()), nil)
	url.RawQuery = query
	return url
}

func (s *charmsSuite) charmsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(query).String()
}

func (s *charmsSuite) uploadRequest(c *tc.C, url, contentType string, content io.Reader) *http.Response {
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

func (s *charmsSuite) assertUploadResponse(c *tc.C, resp *http.Response, expCharmURL string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, tc.Equals, "")
	c.Check(charmResponse.CharmURL, tc.Equals, expCharmURL)
}

func (s *charmsSuite) assertGetFileResponse(c *tc.C, resp *http.Response, expBody, expContentType string) {
	body := apitesting.AssertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), tc.Equals, expBody)
}

func (s *charmsSuite) assertGetFileListResponse(c *tc.C, resp *http.Response, expFiles []string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, tc.Equals, "")
	c.Check(charmResponse.Files, tc.DeepEquals, expFiles)
}

func (s *charmsSuite) assertErrorResponse(c *tc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, tc.Matches, expError)
}

func (s *charmsSuite) assertResponse(c *tc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("body: %s", body))
	return charmResponse
}

func (s *charmsSuite) setModelImporting(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmsSuite) SetUpSuite(c *tc.C) {
	s.apiserverBaseSuite.SetUpSuite(c)
}

func (s *charmsSuite) TestCharmsServedSecurely(c *tc.C) {
	url := s.charmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *charmsSuite) TestPOSTRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmsSuite) TestGETRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmsSuite) TestRequiresPOSTorGET(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "Method Not Allowed\n")
}

func (s *charmsSuite) TestPOSTRejectsNonUserAuth(c *tc.C) {
	// Add a machine and try to login.
	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "noncy",
	})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         machine.Tag().String(),
		Password:    password,
		Method:      "POST",
		URL:         s.charmsURI(""),
		Nonce:       "noncy",
		ContentType: "foo/bar",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authorization failed: permission denied\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.charmsURI("")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip.+")
}

func (s *charmsSuite) TestPOSTRejectsUserWithoutPermission(c *tc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "oryx",
		Password:    "gardener",
		NoModelUser: true,
	})

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         u.Tag().String(),
		Password:    "gardener",
		Method:      "POST",
		URL:         s.charmsURI(""),
		Nonce:       "noncy",
		ContentType: "foo/bar",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authorization failed: permission denied\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.charmsURI("")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip.+")
}

func (s *charmsSuite) TestPOSTAllowsUserWithWritePermission(c *tc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
		Access:   permission.WriteAccess,
	})

	pathToArchive := testcharms.Repo.CharmArchivePath(c.MkDir(), "dummy")
	ch, err := charm.ReadCharmArchive(pathToArchive)
	c.Assert(err, tc.IsNil)
	f, err := os.Open(ch.Path)
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         u.Tag().String(),
		Password:    "gardener",
		Method:      "POST",
		URL:         s.charmsURI("?series=quantal"),
		Nonce:       "noncy",
		ContentType: "application/zip",
		Body:        f,
	})

	inputURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, inputURL.String())
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *tc.C) {
	var empty bytes.Buffer

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*cannot open charm archive: zip: not a valid zip file$")

	// Now try with the default Content-Type.
	resp = s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/octet-stream", &empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip, got: application/octet-stream$")
}

func (s *charmsSuite) TestUploadBumpsRevision(c *tc.C) {
	// Add the dummy charm with revision 1.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision())
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleSha256 is calculated.
	f, err := os.Open(ch.Path)
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", f)
	expectedURL := "local:quantal/dummy-2"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.Equals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 2)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
	// No more checks for the hash here, because it is
	// verified in TestUploadRespectsLocalRevision.
	c.Assert(sch.BundleSha256(), tc.Not(tc.Equals), "")
}

func (s *charmsSuite) TestUploadVersion(c *tc.C) {
	expectedVersion := "dummy-146-g725cfd3-dirty"

	// Add the dummy charm with version "juju-2.4-beta3-146-g725cfd3-dirty".
	pathToArchive := testcharms.Repo.CharmArchivePath(c.MkDir(), "dummy")
	err := testcharms.InjectFilesToCharmArchive(pathToArchive, map[string]string{
		"version": expectedVersion,
	})
	c.Assert(err, tc.IsNil)
	ch, err := charm.ReadCharmArchive(pathToArchive)
	c.Assert(err, tc.IsNil)

	f, err := os.Open(ch.Path)
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", f)

	inputURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, inputURL)
	sch, err := s.State.Charm(inputURL)
	c.Assert(err, tc.ErrorIsNil)

	version := sch.Version()
	c.Assert(version, tc.Equals, expectedVersion)
}

func (s *charmsSuite) TestUploadRespectsLocalRevision(c *tc.C) {
	// Make a dummy charm dir with revision 123.
	dir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	dir.SetDiskRevision(123)
	// Now bundle the dir.
	var buf bytes.Buffer
	err := dir.ArchiveTo(&buf)
	c.Assert(err, tc.ErrorIsNil)
	hash := sha256.New()
	hash.Write(buf.Bytes())
	expectedSHA256 := hex.EncodeToString(hash.Sum(nil))

	// Now try uploading it and ensure the revision persists.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &buf)
	expectedURL := "local:quantal/dummy-123"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.Equals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 123)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
	c.Assert(sch.BundleSha256(), tc.Equals, expectedSHA256)

	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	reader, _, err := storage.Get(sch.StoragePath())
	c.Assert(err, tc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(downloadedSHA256, tc.Equals, expectedSHA256)
}

func (s *charmsSuite) TestUploadWithMultiSeriesCharm(c *tc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	resp := s.uploadRequest(c, s.charmsURL("").String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := "local:dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
}

func (s *charmsSuite) TestUploadAllowsTopLevelPath(c *tc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	// Backwards compatibility check, that we can upload charms to
	// https://host:port/charms
	url := s.charmsURL("series=quantal")
	url.Path = "/charms"
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
}

func (s *charmsSuite) TestUploadAllowsModelUUIDPath(c *tc.C) {
	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
}

func (s *charmsSuite) TestUploadAllowsOtherModelUUIDPath(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
}

func (s *charmsSuite) TestUploadRepackagesNestedArchives(c *tc.C) {
	// Make a clone of the dummy charm in a nested directory.
	rootDir := c.MkDir()
	dirPath := filepath.Join(rootDir, "subdir1", "subdir2")
	err := os.MkdirAll(dirPath, 0755)
	c.Assert(err, tc.ErrorIsNil)
	dir := testcharms.Repo.ClonedDir(dirPath, "dummy")
	// Now tweak the path the dir thinks it is in and bundle it.
	dir.Path = rootDir
	var buf bytes.Buffer
	err = dir.ArchiveTo(&buf)
	c.Assert(err, tc.ErrorIsNil)

	// Try reading it as a bundle - should fail due to nested dirs.
	_, err = charm.ReadCharmArchiveBytes(buf.Bytes())
	c.Assert(err, tc.ErrorMatches, `archive file "metadata.yaml" not found`)

	// Now try uploading it - should succeed and be repackaged.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &buf)
	expectedURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.Equals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 1)
	c.Assert(sch.IsUploaded(), tc.IsTrue)

	// Get it from the storage and try to read it as a bundle - it
	// should succeed, because it was repackaged during upload to
	// strip nested dirs.
	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	reader, _, err := storage.Get(sch.StoragePath())
	c.Assert(err, tc.ErrorIsNil)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	downloadedFile, err := os.CreateTemp(c.MkDir(), "downloaded")
	c.Assert(err, tc.ErrorIsNil)
	defer downloadedFile.Close()
	defer os.Remove(downloadedFile.Name())
	err = os.WriteFile(downloadedFile.Name(), data, 0644)
	c.Assert(err, tc.ErrorIsNil)

	bundle, err := charm.ReadCharmArchive(downloadedFile.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bundle.Revision(), tc.DeepEquals, sch.Revision())
	c.Assert(bundle.Meta(), tc.DeepEquals, sch.Meta())
	c.Assert(bundle.Config(), tc.DeepEquals, sch.Config())
}

func (s *charmsSuite) TestNonLocalCharmUploadFailsIfNotMigrating(c *tc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := fmt.Sprintf("ch:quantal/%s-%d", ch.Meta().Name, ch.Revision())
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	resp := s.uploadRequest(c, s.charmsURI("?schema=ch&series=quantal"), "application/zip", &fileReader{path: ch.Path})
	s.assertErrorResponse(c, resp, 400, ".*charms may only be uploaded during model migration import$")
}

func (s *charmsSuite) TestNonLocalCharmUpload(c *tc.C) {
	// Check that upload of charms with the "ch:" schema works (for
	// model migrations).
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=ch&series=quantal"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := "ch:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 1)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
}

func (s *charmsSuite) TestCharmHubCharmUpload(c *tc.C) {
	// Check that upload of charms with the "ch:" schema works (for
	// model migrations).
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	expectedURL := "ch:s390x/bionic/dummy-15"
	info := state.CharmInfo{
		Charm:       ch,
		ID:          expectedURL,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.State.AddCharm(info)
	c.Assert(err, tc.ErrorIsNil)

	resp := s.uploadRequest(c, s.charmsURI("?arch=s390x&revision=15&schema=ch&series=bionic"), "application/zip", &fileReader{path: ch.Path})

	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 15)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
}

func (s *charmsSuite) TestUnsupportedSchema(c *tc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=zz"), "application/zip", &fileReader{path: ch.Path})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`cannot upload charm: unsupported schema "zz"`,
	)
}

func (s *charmsSuite) TestCharmUploadWithUserOverride(c *tc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=ch"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := "ch:dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, expectedURL)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
}

func (s *charmsSuite) TestNonLocalCharmUploadWithRevisionOverride(c *tc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=ch&&revision=99"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := "ch:dummy-99"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sch.URL(), tc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), tc.Equals, 99)
	c.Assert(sch.IsUploaded(), tc.IsTrue)
}

func (s *charmsSuite) TestMigrateCharm(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	importedModel, err := newSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = importedModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	// The default user is just a normal user, not a controller admin
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url.String(),
		ContentType: "application/zip",
		Body:        &fileReader{path: ch.Path},
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: importedModel.UUID(),
		},
	})
	expectedURL := "local:quantal/dummy-1"
	s.assertUploadResponse(c, resp, expectedURL)

	// The charm was added to the migrated model.
	_, err = newSt.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmsSuite) TestMigrateCharmName(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	importedModel, err := newSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = importedModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	// The default user is just a normal user, not a controller admin
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal&name=meshuggah")
	url.Path = "/migrate/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url.String(),
		ContentType: "application/zip",
		Body:        &fileReader{path: ch.Path},
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: importedModel.UUID(),
		},
	})
	expectedURL := "local:quantal/meshuggah-1"
	s.assertUploadResponse(c, resp, expectedURL)

	// The charm was added to the migrated model.
	_, err = newSt.Charm(expectedURL)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *charmsSuite) TestMigrateCharmNotMigrating(c *tc.C) {
	migratedModel := s.Factory.MakeModel(c, nil)
	defer migratedModel.Close()

	// The default user is just a normal user, not a controller admin
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url.String(),
		ContentType: "application/zip",
		Body:        &fileReader{path: ch.Path},
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: migratedModel.ModelUUID(),
		},
	})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`cannot upload charm: model migration mode is "" instead of "importing"`,
	)
}

func (s *charmsSuite) TestMigrateCharmUnauthorized(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "hunter2"})
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "POST",
		URL:      url.String(),
		Tag:      user.Tag().String(),
		Password: "hunter2",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Matches, "authorization failed: user .* not a controller admin\n")
}

func (s *charmsSuite) TestGetRequiresCharmURL(c *tc.C) {
	uri := s.charmsURI("?file=hooks/install")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		".*expected url=CharmURL query argument$",
	)
}

func (s *charmsSuite) TestGetFailsWithInvalidCharmURL(c *tc.C) {
	uri := s.charmsURI("?url=local:precise/no-such")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`.*cannot get charm from state: charm "local:precise/no-such" not found$`,
	)
}

func (s *charmsSuite) TestGetReturnsNotFoundWhenMissing(c *tc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure a 404 is returned for files not included in the charm.
	for i, file := range []string{
		"no-such-file", "..", "../../../etc/passwd", "hooks/delete",
	} {
		c.Logf("test %d: %s", i, file)
		uri := s.charmsURI("?url=local:quantal/dummy-1&file=" + file)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestGetReturnsNotYetAvailableForPendingCharms(c *tc.C) {
	// Add a charm in pending mode.
	chInfo := state.CharmInfo{
		ID:          "ch:focal/dummy-1",
		Charm:       testcharms.Repo.CharmArchive(c.MkDir(), "dummy"),
		StoragePath: "", // indicates that we don't have the data in the blobstore yet.
		SHA256:      "", // indicates that we don't have the data in the blobstore yet.
		Version:     "42",
	}
	_, err := s.State.AddCharmMetadata(chInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure a 490 is returned if the charm is pending to be downloaded.
	uri := s.charmsURI("?url=ch:focal/dummy-1")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	c.Assert(resp.StatusCode, tc.Equals, http.StatusConflict, tc.Commentf("expected to get 409 for charm that is pending to be downloaded"))
}

func (s *charmsSuite) TestGetReturnsForbiddenWithDirectory(c *tc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI("?url=local:quantal/dummy-1&file=hooks")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	c.Assert(resp.StatusCode, tc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestGetReturnsFileContents(c *tc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure the file contents are properly returned.
	for i, t := range []struct {
		summary  string
		file     string
		response string
	}{{
		summary:  "relative path",
		file:     "revision",
		response: "1",
	}, {
		summary:  "exotic path",
		file:     "./hooks/../revision",
		response: "1",
	}, {
		summary:  "sub-directory path",
		file:     "hooks/install",
		response: "#!/bin/bash\necho \"Done!\"\n",
	},
	} {
		c.Logf("test %d: %s", i, t.summary)
		uri := s.charmsURI("?url=local:quantal/dummy-1&file=" + t.file)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestGetCharmIcon(c *tc.C) {
	// Upload the local charms.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "mysql")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	ch = testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Prepare the tests.
	svgMimeType := mime.TypeByExtension(".svg")
	iconPath := filepath.Join(testcharms.Repo.CharmDirPath("mysql"), "icon.svg")
	icon, err := os.ReadFile(iconPath)
	c.Assert(err, tc.ErrorIsNil)
	tests := []struct {
		about      string
		query      string
		expectType string
		expectBody string
	}{{
		about:      "icon found",
		query:      "?url=local:quantal/mysql-1&file=icon.svg",
		expectBody: string(icon),
	}, {
		about: "icon not found",
		query: "?url=local:quantal/dummy-1&file=icon.svg",
	}, {
		about:      "default icon requested: icon found",
		query:      "?url=local:quantal/mysql-1&icon=1",
		expectBody: string(icon),
	}, {
		about:      "default icon requested: icon not found",
		query:      "?url=local:quantal/dummy-1&icon=1",
		expectBody: common.DefaultCharmIcon,
	}, {
		about:      "default icon request ignored",
		query:      "?url=local:quantal/mysql-1&file=revision&icon=1",
		expectType: "text/plain; charset=utf-8",
		expectBody: "1",
	}}

	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		uri := s.charmsURI(test.query)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		if test.expectBody == "" {
			s.assertErrorResponse(c, resp, http.StatusNotFound, ".*charm file not found$")
			continue
		}
		if test.expectType == "" {
			test.expectType = svgMimeType
		}
		s.assertGetFileResponse(c, resp, test.expectBody, test.expectType)
	}
}

func (s *charmsSuite) TestGetWorksForControllerMachines(c *tc.C) {
	// Make a controller machine.
	const nonce = "noncey"
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: nonce,
	})

	// Create a hosted model and upload a charm for it.
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	curl := charm.MustParseURL("local:quantal/dummy-1")
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, curl, ch, false)
	c.Assert(err, tc.ErrorIsNil)

	// Controller machine should be able to download the charm from
	// the hosted model. This is required for controller workers which
	// are acting on behalf of a particular hosted model.
	url := s.charmsURL("url=" + curl.String() + "&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	params := apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      url.String(),
		Tag:      m.Tag().String(),
		Password: password,
		Nonce:    nonce,
	}
	resp := apitesting.SendHTTPRequest(c, params)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetStarReturnsArchiveBytes(c *tc.C) {
	// Add the dummy charm.
	ch, err := charm.ReadCharmDir(
		testcharms.RepoWithSeries("quantal").ClonedDirPath(c.MkDir(), "dummy"))
	c.Assert(err, tc.ErrorIsNil)
	// Create an archive from the charm dir.
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, tc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, tc.ErrorIsNil)
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: tempFile.Name()})

	data, err := os.ReadFile(tempFile.Name())
	c.Assert(err, tc.ErrorIsNil)

	uri := s.charmsURI("?url=local:quantal/dummy-1&file=*")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertGetFileResponse(c, resp, string(data), "application/zip")
}

func (s *charmsSuite) TestGetAllowsTopLevelPath(c *tc.C) {
	// Backwards compatibility check, that we can GET from charms at
	// https://host:port/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	url := s.charmsURL("url=local:quantal/dummy-1&file=revision")
	url.Path = "/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsModelUUIDPath(c *tc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	url := s.charmsURL("url=local:quantal/dummy-1&file=revision")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsOtherEnvironment(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	curl := charm.MustParseURL("local:quantal/dummy-1")
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, curl, ch, false)
	c.Assert(err, tc.ErrorIsNil)

	url := s.charmsURL("url=" + curl.String() + "&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetReturnsManifest(c *tc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure charm files are properly listed.
	uri := s.charmsURI("?url=local:quantal/dummy-1")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	manifest, err := ch.ArchiveMembers()
	c.Assert(err, tc.ErrorIsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, tc.Equals, params.ContentTypeJSON)
}

func (s *charmsSuite) TestNoTempFilesLeftBehind(c *tc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Download it.
	uri := s.charmsURI("?url=local:quantal/dummy-1&file=*")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	apitesting.AssertResponse(c, resp, http.StatusOK, "application/zip")

	// Ensure the tmp directory exists but nothing is in it.
	files, err := os.ReadDir(filepath.Join(s.config.DataDir, "charm-get-tmp"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.HasLen, 0)
}

type fileReader struct {
	path string
	r    io.Reader
}

func (r *fileReader) Read(out []byte) (int, error) {
	if r.r == nil {
		content, err := os.ReadFile(r.path)
		if err != nil {
			return 0, err
		}
		r.r = bytes.NewReader(content)
	}
	return r.r.Read(out)
}
