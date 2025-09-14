// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	tctesting "testing"
	"time"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/mocks"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/resources"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type resourcesUploadSuite struct {
	apiserverBaseSuite
	appName        string
	unit           *state.Unit
	importingState *state.State
	importingModel *state.Model
}

func TestResourcesUploadSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &resourcesUploadSuite{})
}

func (s *resourcesUploadSuite) SetUpTest(c *tc.C) {
	s.apiserverBaseSuite.SetUpTest(c)

	// Create an importing model to work with.
	var err error
	s.importingState = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { s.importingState.Close() })
	s.importingModel, err = s.importingState.Model()
	c.Assert(err, tc.ErrorIsNil)

	newFactory := factory.NewFactory(s.importingState, s.StatePool)
	app := newFactory.MakeApplication(c, nil)
	s.appName = app.Name()

	s.unit = newFactory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	err = s.importingModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourcesUploadSuite) sendHTTPRequest(c *tc.C, p apitesting.HTTPRequestParams) *http.Response {
	p.ExtraHeaders = map[string]string{
		params.MigrationModelHTTPHeader: s.importingModel.UUID(),
	}
	return s.apiserverBaseSuite.sendHTTPRequest(c, p)
}

func (s *resourcesUploadSuite) TestServedSecurely(c *tc.C) {
	url := s.resourcesURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *resourcesUploadSuite) TestGETUnsupported(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *resourcesUploadSuite) TestPUTUnsupported(c *tc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *resourcesUploadSuite) TestPOSTRequiresAuth(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.resourcesURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authentication failed: no credentials provided\n")
}

func (s *resourcesUploadSuite) TestPOSTRequiresUserAuth(c *tc.C) {
	// Add a machine and try to login.
	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "noncy",
	})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         machine.Tag().String(),
		Password:    password,
		Method:      "POST",
		URL:         s.resourcesURI(""),
		Nonce:       "noncy",
		ContentType: "foo/bar",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), tc.Equals, "authorization failed: machine 0 is not a user\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "missing application/unit")
}

func (s *resourcesUploadSuite) TestRejectsInvalidModel(c *tc.C) {
	params := apitesting.HTTPRequestParams{
		Method: "POST",
		URL:    s.resourcesURI(""),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: "dead-beef-123456",
		},
	}
	resp := s.apiserverBaseSuite.sendHTTPRequest(c, params)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `.*unknown model: "dead-beef-123456"`)
}

const content = "stuff"

func (s *resourcesUploadSuite) makeUploadArgs(c *tc.C) url.Values {
	return s.makeResourceUploadArgs(c, "file")
}

func (s *resourcesUploadSuite) makeDockerUploadArgs(c *tc.C) url.Values {
	result := s.makeResourceUploadArgs(c, "oci-image")
	result.Del("path")
	return result
}

func (s *resourcesUploadSuite) makeResourceUploadArgs(c *tc.C, resType string) url.Values {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)
	q := make(url.Values)
	q.Add("application", s.appName)
	q.Add("user", "napoleon")
	q.Add("name", "bin")
	q.Add("path", "blob.zip")
	q.Add("description", "hmm")
	q.Add("type", resType)
	q.Add("origin", "store")
	q.Add("revision", "3")
	q.Add("size", fmt.Sprint(len(content)))
	q.Add("fingerprint", fp.Hex())
	q.Add("timestamp", fmt.Sprint(time.Now().UnixNano()))
	return q
}

func (s *resourcesUploadSuite) TestUpload(c *tc.C) {
	outResp := s.uploadAppResource(c, nil)
	c.Check(outResp.ID, tc.Not(tc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), tc.IsFalse)

	rSt := s.importingState.Resources()
	res, reader, err := rSt.OpenResource(s.appName, "bin")
	c.Assert(err, tc.ErrorIsNil)
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(readContent), tc.Equals, content)
	c.Assert(res.ID, tc.Equals, outResp.ID)
}

func (s *resourcesUploadSuite) TestUnitUpload(c *tc.C) {
	// Upload application resource first. A unit resource can't be
	// uploaded without the application resource being there first.
	s.uploadAppResource(c, nil)

	q := s.makeUploadArgs(c)
	q.Del("application")
	q.Set("unit", s.unit.Name())
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(q.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	outResp := s.assertResponse(c, resp, http.StatusOK)
	c.Check(outResp.ID, tc.Not(tc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), tc.IsFalse)
}

func (s *resourcesUploadSuite) TestPlaceholder(c *tc.C) {
	query := s.makeUploadArgs(c)
	query.Del("timestamp") // No timestamp means placeholder
	outResp := s.uploadAppResource(c, &query)
	c.Check(outResp.ID, tc.Not(tc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), tc.IsTrue)

	rSt := s.importingState.Resources()
	res, err := rSt.GetResource(s.appName, "bin")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.IsPlaceholder(), tc.IsTrue)
	c.Check(res.ApplicationID, tc.Equals, s.appName)
	c.Check(res.Name, tc.Equals, "bin")
	c.Check(res.Size, tc.Equals, int64(len(content)))
}

func (s *resourcesUploadSuite) uploadAppResource(c *tc.C, query *url.Values) params.ResourceUploadResult {
	if query == nil {
		q := s.makeUploadArgs(c)
		query = &q
	}
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(query.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	return s.assertResponse(c, resp, http.StatusOK)
}

func (s *resourcesUploadSuite) TestArgValidation(c *tc.C) {
	checkBadRequest := func(q url.Values, expected string) {
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method: "POST",
			URL:    s.resourcesURI(q.Encode()),
		})
		s.assertErrorResponse(c, resp, http.StatusBadRequest, expected)
	}

	q := s.makeUploadArgs(c)
	q.Del("application")
	checkBadRequest(q, "missing application/unit")

	q = s.makeUploadArgs(c)
	q.Set("unit", "some/0")
	checkBadRequest(q, "application and unit can't be set at the same time")

	q = s.makeUploadArgs(c)
	q.Del("name")
	checkBadRequest(q, "missing name")

	q = s.makeUploadArgs(c)
	q.Del("path")
	checkBadRequest(q, "missing path")

	q = s.makeUploadArgs(c)
	q.Set("type", "fooo")
	checkBadRequest(q, "invalid type")

	q = s.makeUploadArgs(c)
	q.Set("origin", "fooo")
	checkBadRequest(q, "invalid origin")

	q = s.makeUploadArgs(c)
	q.Set("revision", "fooo")
	checkBadRequest(q, "invalid revision")

	q = s.makeUploadArgs(c)
	q.Set("size", "fooo")
	checkBadRequest(q, "invalid size")

	q = s.makeUploadArgs(c)
	q.Set("fingerprint", "zzz")
	checkBadRequest(q, "invalid fingerprint")
}

func (s *resourcesUploadSuite) TestArgValidationCAASModel(c *tc.C) {
	content := `{"ImageName": "image-name", "Username": "fred", "Password":"secret"}`
	checkRequest := func(q url.Values) {
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method: "POST",
			URL:    s.resourcesURI(q.Encode()),
			Body:   bytes.NewReader([]byte(content)),
		})
		s.assertResponse(c, resp, http.StatusOK)
	}

	q := s.makeDockerUploadArgs(c)
	checkRequest(q)
}

func (s *resourcesUploadSuite) TestFailsWhenModelNotImporting(c *tc.C) {
	err := s.importingModel.SetMigrationMode(state.MigrationModeNone)
	c.Assert(err, tc.ErrorIsNil)

	q := s.makeUploadArgs(c)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(q.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	s.assertResponse(c, resp, http.StatusBadRequest)
}

func (s *resourcesUploadSuite) resourcesURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.resourcesURL(query).String()
}

func (s *resourcesUploadSuite) resourcesURL(query string) *url.URL {
	url := s.URL("/migrate/resources", nil)
	url.RawQuery = query
	return url
}

func (s *resourcesUploadSuite) assertErrorResponse(c *tc.C, resp *http.Response, expStatus int, expError string) {
	outResp := s.assertResponse(c, resp, expStatus)
	err := outResp.Error
	c.Assert(err, tc.NotNil)
	c.Check(err.Message, tc.Matches, expError)
}

func (s *resourcesUploadSuite) assertResponse(c *tc.C, resp *http.Response, expStatus int) params.ResourceUploadResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var outResp params.ResourceUploadResult
	err := json.Unmarshal(body, &outResp)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("Body: %s", body))
	return outResp
}

func (s *resourcesUploadSuite) TestSetResource(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stResources := mocks.NewMockResources(ctrl)
	gomock.InOrder(
		stResources.EXPECT().SetUnitResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(resources.Resource{}, nil),
		stResources.EXPECT().SetResource(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), state.DoNotIncrementCharmModifiedVersion).Return(resources.Resource{}, nil),
	)
	apiserver.SetResource(true, "", "", charmresource.Resource{}, nil, stResources)
	apiserver.SetResource(false, "", "", charmresource.Resource{}, nil, stResources)
}
