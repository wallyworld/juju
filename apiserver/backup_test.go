// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

func TestBackupsSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &backupsSuite{})
}

type backupsSuite struct {
	apiserverBaseSuite
	backupURL string
	fake      *backupstesting.FakeBackups
}

func (s *backupsSuite) SetUpTest(c *tc.C) {
	s.apiserverBaseSuite.SetUpTest(c)

	s.backupURL = s.server.URL + fmt.Sprintf("/model/%s/backups", s.State.ModelUUID())
	s.fake = &backupstesting.FakeBackups{}
	s.PatchValue(apiserver.NewBackups,
		func(path *backups.Paths) backups.Backups {
			return s.fake
		},
	)
}

func (s *backupsSuite) assertErrorResponse(c *tc.C, resp *http.Response, statusCode int, msg string) *params.Error {
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(resp.StatusCode, tc.Equals, statusCode, tc.Commentf("body: %s", body))
	c.Assert(resp.Header.Get("Content-Type"), tc.Equals, params.ContentTypeJSON, tc.Commentf("body: %q", body))

	var failure params.Error
	err = json.Unmarshal(body, &failure)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&failure, tc.ErrorMatches, msg, tc.Commentf("body: %s", body))
	return &failure
}

func (s *backupsSuite) TestRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.backupURL})
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusUnauthorized)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(body), tc.Equals, "authentication failed: no credentials provided\n")
}

func (s *backupsSuite) checkInvalidMethod(c *tc.C, method, url string) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: method, URL: url})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "`+method+`"`)
}

func (s *backupsSuite) TestInvalidHTTPMethods(c *tc.C) {
	url := s.backupURL
	for _, method := range []string{"PUT", "POST", "DELETE", "OPTIONS"} {
		c.Log("testing HTTP method: " + method)
		s.checkInvalidMethod(c, method, url)
	}
}

func (s *backupsSuite) TestAuthRequiresClientNotMachine(c *tc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      machine.Tag().String(),
		Password: password,
		Method:   "GET",
		URL:      s.backupURL,
		Nonce:    "fake_nonce",
	})
	c.Assert(resp.StatusCode, tc.Equals, http.StatusForbidden)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(body), tc.Equals, "authorization failed: machine 0 is not a user\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.backupURL})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "POST"`)
}

// sendValid sends a valid GET request to the backups endpoint
// and returns the response and the expected contents of the
// archive if the request succeeds.
func (s *backupsSuite) sendValidGet(c *tc.C) (resp *http.Response, archiveBytes []byte) {
	meta := backupstesting.NewMetadata()
	archive, err := backupstesting.NewArchiveBasic(meta)
	c.Assert(err, tc.ErrorIsNil)
	archiveBytes = archive.Bytes()
	s.fake.Meta = meta
	s.fake.Archive = io.NopCloser(archive)

	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "GET",
		URL:         s.backupURL,
		ContentType: params.ContentTypeJSON,
		JSONBody: params.BackupsDownloadArgs{
			ID: meta.ID(),
		},
	}), archiveBytes
}

func (s *backupsSuite) TestCalls(c *tc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	c.Check(s.fake.Calls, tc.DeepEquals, []string{"Get"})
	c.Check(s.fake.IDArg, tc.Equals, s.fake.Meta.ID())
}

func (s *backupsSuite) TestResponse(c *tc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()
	meta := s.fake.Meta

	c.Check(resp.StatusCode, tc.Equals, http.StatusOK)
	expectedChecksum := base64.StdEncoding.EncodeToString([]byte(meta.Checksum()))
	c.Check(resp.Header.Get("Digest"), tc.Equals, string(params.DigestSHA256)+"="+expectedChecksum)
	c.Check(resp.Header.Get("Content-Type"), tc.Equals, params.ContentTypeRaw)
}

func (s *backupsSuite) TestBody(c *tc.C) {
	resp, archiveBytes := s.sendValidGet(c)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(body, tc.DeepEquals, archiveBytes)
}

func (s *backupsSuite) TestErrorWhenGetFails(c *tc.C) {
	s.fake.Error = errors.New("failed!")
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	s.assertErrorResponse(c, resp, http.StatusInternalServerError, "failed!")
}
