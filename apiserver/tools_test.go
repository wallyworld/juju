// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	coretools "github.com/juju/juju/tools"
)

type baseToolsSuite struct {
	apiserverBaseSuite
}

func (s *baseToolsSuite) toolsURL(query string) *url.URL {
	return s.modelToolsURL(s.Model.UUID(), query)
}

func (s *baseToolsSuite) modelToolsURL(model, query string) *url.URL {
	u := s.URL(fmt.Sprintf("/model/%s/tools", model), nil)
	u.RawQuery = query
	return u
}

func (s *baseToolsSuite) toolsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.toolsURL(query).String()
}

func (s *baseToolsSuite) uploadRequest(c *tc.C, url, contentType string, content io.Reader) *http.Response {
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

func (s *baseToolsSuite) downloadRequest(c *tc.C, version version.Binary, uuid string) *http.Response {
	url := s.toolsURL("")
	if uuid == "" {
		url.Path = fmt.Sprintf("/tools/%s", version)
	} else {
		url.Path = fmt.Sprintf("/model/%s/tools/%s", uuid, version)
	}
	return apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
}

func (s *baseToolsSuite) assertUploadResponse(c *tc.C, resp *http.Response, agentTools *coretools.Tools) {
	toolsResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(toolsResponse.Error, tc.IsNil)
	c.Check(toolsResponse.ToolsList, tc.DeepEquals, coretools.List{agentTools})
}

func (s *baseToolsSuite) assertJSONErrorResponse(c *tc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := s.assertResponse(c, resp, expCode)
	c.Check(toolsResponse.ToolsList, tc.IsNil)
	c.Check(toolsResponse.Error, tc.NotNil)
	c.Check(toolsResponse.Error.Message, tc.Matches, expError)
}

func (s *baseToolsSuite) assertPlainErrorResponse(c *tc.C, resp *http.Response, expCode int, expError string) {
	body := apitesting.AssertResponse(c, resp, expCode, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Matches, expError+"\n")
}

func (s *baseToolsSuite) assertResponse(c *tc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("Body: %s", body))
	return toolsResponse
}

func (s *baseToolsSuite) storeFakeTools(c *tc.C, st *state.State, content string, metadata binarystorage.Metadata) *coretools.Tools {
	storage, err := st.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer storage.Close()
	err = storage.Add(strings.NewReader(content), metadata)
	c.Assert(err, tc.ErrorIsNil)
	return &coretools.Tools{
		Version: version.MustParseBinary(metadata.Version),
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
	}
}

func (s *baseToolsSuite) getToolsFromStorage(c *tc.C, st *state.State, vers string) (binarystorage.Metadata, []byte) {
	storage, err := st.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer storage.Close()
	metadata, r, err := storage.Open(vers)
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(r)
	r.Close()
	c.Assert(err, tc.ErrorIsNil)
	return metadata, data
}

func (s *baseToolsSuite) getToolsMetadataFromStorage(c *tc.C, st *state.State) []binarystorage.Metadata {
	storage, err := st.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.AllMetadata()
	c.Assert(err, tc.ErrorIsNil)
	return metadata
}

func (s *baseToolsSuite) testDownload(c *tc.C, tools *coretools.Tools, uuid string) []byte {
	resp := s.downloadRequest(c, tools.Version, uuid)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), tc.Equals, tools.SHA256)
	return data
}

type toolsSuite struct {
	baseToolsSuite
}

func TestToolsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &toolsSuite{})
}

func (s *toolsSuite) TestToolsUploadedSecurely(c *tc.C) {
	url := s.toolsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "PUT",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *toolsSuite) TestRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.toolsURI("")})
	s.assertPlainErrorResponse(c, resp, http.StatusUnauthorized, "authentication failed: no credentials provided")
}

func (s *toolsSuite) TestRequiresPOST(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *toolsSuite) TestAuthRejectsNonsUser(c *tc.C) {
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
		Method:   "POST",
		URL:      s.toolsURI(""),
		Nonce:    "fake_nonce",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestAuthRejectsUserWithoutPermission(c *tc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
		Access:   permission.WriteAccess,
	})

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      u.Tag().String(),
		Password: "gardener",
		Method:   "POST",
		URL:      s.toolsURI(""),
		Nonce:    "fake_nonce",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)
}

func (s *toolsSuite) TestUploadRequiresVersion(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadFailsWithNoTools(c *tc.C) {
	var empty bytes.Buffer
	resp := s.uploadRequest(c, s.toolsURI("?binaryVersion=1.18.0-ubuntu-amd64"), "application/x-tar-gz", &empty)
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "no agent binaries uploaded")
}

func (s *toolsSuite) TestUploadFailsWithInvalidContentType(c *tc.C) {
	var empty bytes.Buffer
	// Now try with the default Content-Type.
	resp := s.uploadRequest(c, s.toolsURI("?binaryVersion=1.18.0-ubuntu-amd64"), "application/octet-stream", &empty)
	s.assertJSONErrorResponse(
		c, resp, http.StatusBadRequest, "expected Content-Type: application/x-tar-gz, got: application/octet-stream")
}

func (s *toolsSuite) setupToolsForUpload(c *tc.C) (coretools.List, version.Binary, []byte) {
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-ubuntu-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "released", versionStrings)
	toolsFile := envtools.StorageName(vers, "released")
	toolsContent, err := os.ReadFile(filepath.Join(localStorage, toolsFile))
	c.Assert(err, tc.ErrorIsNil)
	return expectedTools, vers, toolsContent
}

func (s *toolsSuite) TestUpload(c *tc.C) {
	// Make some fake tools.
	expectedTools, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	// Now try uploading them.
	resp := s.uploadRequest(
		c, s.toolsURI("?binaryVersion="+vers),
		"application/x-tar-gz",
		bytes.NewReader(toolsContent),
	)

	// Check the response.
	expectedTools[0].URL = s.toolsURL("").String() + "/" + vers
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	metadata, uploadedData := s.getToolsFromStorage(c, s.State, vers)
	c.Assert(uploadedData, tc.DeepEquals, toolsContent)
	allMetadata := s.getToolsMetadataFromStorage(c, s.State)
	c.Assert(allMetadata, tc.DeepEquals, []binarystorage.Metadata{metadata})
}

func (s *toolsSuite) TestMigrateTools(c *tc.C) {
	// Make some fake tools.
	expectedTools, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	importedModel, err := newSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = importedModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	// Now try uploading them.
	uri := s.URL("/migrate/tools", url.Values{"binaryVersion": {vers}})
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         uri.String(),
		ContentType: "application/x-tar-gz",
		Body:        bytes.NewReader(toolsContent),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: importedModel.UUID(),
		},
	})

	// Check the response.
	expectedTools[0].URL = s.modelToolsURL(s.State.ControllerModelUUID(), "").String() + "/" + vers
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	metadata, uploadedData := s.getToolsFromStorage(c, newSt, vers)
	c.Assert(uploadedData, tc.DeepEquals, toolsContent)
	allMetadata := s.getToolsMetadataFromStorage(c, newSt)
	c.Assert(allMetadata, tc.DeepEquals, []binarystorage.Metadata{metadata})
}

func (s *toolsSuite) TestMigrateToolsNotMigrating(c *tc.C) {
	// Make some fake tools.
	_, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	uri := s.URL("/migrate/tools", url.Values{"binaryVersion": {vers}})
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         uri.String(),
		ContentType: "application/x-tar-gz",
		Body:        bytes.NewReader(toolsContent),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: newSt.ModelUUID(),
		},
	})

	// Now try uploading them.
	s.assertJSONErrorResponse(
		c, resp, http.StatusBadRequest,
		`model migration mode is "" instead of "importing"`,
	)
}

func (s *toolsSuite) TestMigrateToolsUnauth(c *tc.C) {
	// Try uploading as a non controller admin.
	url := s.URL("/migrate/tools", nil).String()
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "hunter2"})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "POST",
		URL:      url,
		Tag:      user.Tag().String(),
		Password: "hunter2",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: user .* is not a controller admin",
	)
}

func (s *toolsSuite) TestBlockUpload(c *tc.C) {
	// Make some fake tools.
	_, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	// Block all changes.
	err := s.State.SwitchBlockOn(state.ChangeBlock, "TestUpload")
	c.Assert(err, tc.ErrorIsNil)

	// Now try uploading them.
	resp := s.uploadRequest(
		c, s.toolsURI("?binaryVersion="+vers),
		"application/x-tar-gz",
		bytes.NewReader(toolsContent),
	)
	toolsResponse := s.assertResponse(c, resp, http.StatusBadRequest)
	c.Assert(toolsResponse.Error, tc.Satisfies, params.IsCodeOperationBlocked)
	c.Assert(errors.Cause(toolsResponse.Error), tc.DeepEquals, &params.Error{
		Message: "TestUpload",
		Code:    "operation is blocked",
	})

	// Check the contents.
	storage, err := s.State.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer storage.Close()
	_, _, err = storage.Open(vers)
	c.Assert(errors.IsNotFound(err), tc.IsTrue)
}

func (s *toolsSuite) TestUploadAllowsTopLevelPath(c *tc.C) {
	// Backwards compatibility check, that we can upload tools to
	// https://host:port/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.toolsURL("binaryVersion=" + vers.String())
	url.Path = "/tools"
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))
	expectedTools[0].URL = s.modelToolsURL(s.State.ControllerModelUUID(), "").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsModelUUIDPath(c *tc.C) {
	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.toolsURL("binaryVersion=" + vers.String())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))
	// Check the response.
	expectedTools[0].URL = s.toolsURL("").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsOtherModelUUIDPath(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.modelToolsURL(newSt.ModelUUID(), "binaryVersion="+vers.String())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))

	// Check the response.
	expectedTools[0].URL = s.modelToolsURL(newSt.ModelUUID(), "").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestDownloadModelUUIDPath(c *tc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, s.State.ModelUUID())
}

func (s *toolsSuite) TestDownloadOtherModelUUIDPath(c *tc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	tools := s.storeFakeTools(c, newSt, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, newSt.ModelUUID())
}

func (s *toolsSuite) TestDownloadTopLevelPath(c *tc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, "")
}

func (s *toolsSuite) TestDownloadMissingConcurrent(c *tc.C) {
	closer, testStorage, _ := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	var mut sync.Mutex
	resolutions := 0
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		// Add some delay to make sure all goroutines are waiting.
		time.Sleep(10 * time.Millisecond)
		mut.Lock()
		defer mut.Unlock()
		resolutions++
		return storage.NewStorageSimpleStreamsDataSource("test datasource", testStorage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	toolsBinaries := []version.Binary{
		version.MustParseBinary("2.9.98-ubuntu-amd64"),
		version.MustParseBinary("2.9.99-ubuntu-amd64"),
	}
	stream := "released"
	tools, err := envtesting.UploadFakeToolsVersions(testStorage, stream, stream, toolsBinaries...)
	c.Assert(err, tc.ErrorIsNil)

	var wg sync.WaitGroup
	const n = 8
	wg.Add(n)
	for i := 0; i < n; i++ {
		tool := tools[i%len(toolsBinaries)]
		go func() {
			defer wg.Done()
			s.testDownload(c, tool, s.State.ModelUUID())
		}()
	}
	wg.Wait()

	c.Assert(resolutions, tc.Equals, len(toolsBinaries))
}

type caasToolsSuite struct {
	baseToolsSuite
}

func TestCaasToolsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &caasToolsSuite{})
}

func (s *caasToolsSuite) SetUpTest(c *tc.C) {
	s.ControllerModelType = state.ModelTypeCAAS
	s.baseToolsSuite.SetUpTest(c)
}

func (s *caasToolsSuite) TestToolDownloadNotSharedCAASController(c *tc.C) {
	closer, testStorage, _ := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	const n = 8
	states := []*state.State{}
	for i := 0; i < n; i++ {
		testState := s.Factory.MakeModel(c, nil)
		defer testState.Close()
		states = append(states, testState)
	}

	var mut sync.Mutex
	resolutions := 0
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		// Add some delay to make sure all goroutines are waiting.
		time.Sleep(10 * time.Millisecond)
		mut.Lock()
		defer mut.Unlock()
		resolutions++
		return storage.NewStorageSimpleStreamsDataSource("test datasource", testStorage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	tool := version.MustParseBinary("2.9.99-ubuntu-amd64")
	stream := "released"
	tools, err := envtesting.UploadFakeToolsVersions(testStorage, stream, stream, tool)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tools, tc.HasLen, 1)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			s.testDownload(c, tools[0], states[i].ModelUUID())
		}()
	}
	wg.Wait()

	c.Assert(resolutions, tc.Equals, n)
}
