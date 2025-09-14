// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	tctesting "testing"

	"github.com/juju/tc"

	apitesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type objectsSuite struct {
	apiserverBaseSuite
	method      string
	contentType string
}

func (s *objectsSuite) SetUpSuite(c *tc.C) {
	s.apiserverBaseSuite.SetUpSuite(c)
}

func (s *objectsSuite) objectsCharmsURL(charmRef string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.State.ModelUUID(), charmRef), nil)
}

func (s *objectsSuite) objectsCharmsURI(charmRef string) string {
	return s.objectsCharmsURL(charmRef).String()
}

func (s *objectsSuite) assertResponse(c *tc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("body: %s", body))
	return charmResponse
}

func (s *objectsSuite) assertErrorResponse(c *tc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, tc.Matches, expError)
}

func (s *objectsSuite) TestRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authentication failed: no credentials provided\n")
}

func (s *objectsSuite) TestFailsWithInvalidObjectSha256(c *tc.C) {
	uri := s.objectsCharmsURI("invalidsha256")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, ContentType: s.contentType, URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*"invalidsha256" is not a valid charm object path$`,
	)
}

func (s *objectsSuite) TestInvalidBucket(c *tc.C) {
	wrongURL := s.URL("modelwrongbucket/charms/somecharm-abcd0123", nil)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusNotFound, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "404 page not found\n")
}

func (s *objectsSuite) TestInvalidModel(c *tc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *objectsSuite) TestInvalidObject(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, ContentType: s.contentType, URL: s.objectsCharmsURI("invalidcharm")})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "application/json")
	c.Assert(string(body), tc.Matches, `{"error":".*\\"invalidcharm\\" is not a valid charm object path","error-code":"bad request"}`)
}

type getObjectsSuite struct {
	objectsSuite
}

func TestGetObjectsSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &getObjectsSuite{})
}

func (s *getObjectsSuite) SetUpSuite(c *tc.C) {
	s.objectsSuite.SetUpSuite(c)
	s.objectsSuite.method = "GET"
}

func (s *getObjectsSuite) TestObjectsCharmsServedSecurely(c *tc.C) {
	url := s.objectsCharmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

type putObjectsSuite struct {
	objectsSuite
}

func TestPutObjectsSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &putObjectsSuite{})
}

func (s *putObjectsSuite) SetUpSuite(c *tc.C) {
	s.objectsSuite.SetUpSuite(c)
	s.objectsSuite.method = "PUT"
	s.objectsSuite.contentType = "application/zip"
}

func (s *putObjectsSuite) uploadRequest(c *tc.C, url, contentType, curl string, content io.Reader) *http.Response {
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         url,
		ContentType: contentType,
		Body:        content,
		ExtraHeaders: map[string]string{
			"Juju-Curl": curl,
		},
	})
}

func (s *putObjectsSuite) assertUploadResponse(c *tc.C, resp *http.Response, expCharmURL string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, tc.Equals, "")
	c.Check(charmResponse.CharmURL, tc.Equals, expCharmURL)
}

func (s *putObjectsSuite) TestUploadFailsWithInvalidZip(c *tc.C) {
	empty := strings.NewReader("")

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp := s.uploadRequest(c, s.objectsCharmsURI("somecharm-"+getCharmHash(c, empty)), "application/zip", "local:somecharm", empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*zip: not a valid zip file$")

	// Now try with the default Content-Type.
	resp = s.uploadRequest(c, s.objectsCharmsURI("somecharm-"+getCharmHash(c, empty)), "application/octet-stream", "local:somecharm", empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip, got: application/octet-stream$")
}

func (s *putObjectsSuite) TestCannotUploadCharmhubCharm(c *tc.C) {
	// We should run verifications like this before processing the charm.
	empty := strings.NewReader("")
	resp := s.uploadRequest(c, s.objectsCharmsURI("somecharm-"+getCharmHash(c, empty)), "application/zip", "ch:somecharm", empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, `.*non-local charms may only be uploaded during model migration import`)
}

func (s *putObjectsSuite) TestUploadBumpsRevision(c *tc.C) {
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
	resp := s.uploadRequest(c, s.objectsCharmsURI("dummy-"+getCharmHash(c, f)), "application/zip", "local:quantal/dummy", f)
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

func getCharmHash(c *tc.C, stream io.ReadSeeker) string {
	hash := sha256.New()
	_, err := io.Copy(hash, stream)
	c.Assert(err, tc.ErrorIsNil)
	_, err = stream.Seek(0, os.SEEK_SET)
	c.Assert(err, tc.ErrorIsNil)
	return hex.EncodeToString(hash.Sum(nil))[0:7]
}
