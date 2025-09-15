// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type UnitResourcesHandlerSuite struct {
	testhelpers.IsolationSuite

	stub     *testhelpers.Stub
	urlStr   string
	recorder *httptest.ResponseRecorder
}

func TestUnitResourcesHandlerSuite(t *tctesting.T) {
	tc.Run(t, &UnitResourcesHandlerSuite{})
}

func (s *UnitResourcesHandlerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = new(testhelpers.Stub)

	args := url.Values{}
	args.Add(":unit", "foo/0")
	args.Add(":resource", "blob")
	s.urlStr = "https://api:17017/?" + args.Encode()

	s.recorder = httptest.NewRecorder()
}

func (s *UnitResourcesHandlerSuite) closer() bool {
	s.stub.AddCall("Close")
	return false
}

func (s *UnitResourcesHandlerSuite) TestWrongMethod(c *tc.C) {
	handler := &apiserver.UnitResourcesHandler{}

	req, err := http.NewRequest("POST", s.urlStr, nil)
	c.Assert(err, tc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	c.Assert(s.recorder.Code, tc.Equals, http.StatusMethodNotAllowed)
	s.stub.CheckNoCalls(c)
}

func (s *UnitResourcesHandlerSuite) TestOpenerCreationError(c *tc.C) {
	failure, expectedBody := apiFailure("boom", "")
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resources.Opener, state.PoolHelper, error) {
			return nil, nil, failure
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, tc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c,
		http.StatusInternalServerError,
		"application/json",
		expectedBody,
	)
}

func (s *UnitResourcesHandlerSuite) TestOpenResourceError(c *tc.C) {
	opener := &stubResourceOpener{
		Stub: s.stub,
	}
	failure, expectedBody := apiFailure("boom", "")
	s.stub.SetErrors(failure)
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resources.Opener, state.PoolHelper, error) {
			s.stub.AddCall("NewOpener", kinds)
			return opener, apiservertesting.StubPoolHelper{StubRelease: s.closer}, nil
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, tc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusInternalServerError, "application/json", expectedBody)
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{"NewOpener", []interface{}{[]string{names.UnitTagKind, names.ApplicationTagKind}}},
		{"OpenResource", []interface{}{"blob"}},
		{"Close", nil},
	})
}

func (s *UnitResourcesHandlerSuite) TestSuccess(c *tc.C) {
	const body = "some data"
	opened := resourcetesting.NewResource(c, new(testhelpers.Stub), "blob", "app", body)
	opener := &stubResourceOpener{
		Stub:               s.stub,
		ReturnOpenResource: opened,
	}
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resources.Opener, state.PoolHelper, error) {
			s.stub.AddCall("NewOpener", kinds)
			return opener, apiservertesting.StubPoolHelper{StubRelease: s.closer}, nil
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, tc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusOK, "application/octet-stream", body)
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{"NewOpener", []interface{}{[]string{names.UnitTagKind, names.ApplicationTagKind}}},
		{"OpenResource", []interface{}{"blob"}},
		{"Close", nil},
	})
}
func (s *UnitResourcesHandlerSuite) checkResp(c *tc.C, status int, ctype, body string) {
	checkHTTPResp(c, s.recorder, status, ctype, body)
}

type stubResourceOpener struct {
	*testhelpers.Stub
	ReturnOpenResource resources.Opened
}

func (s *stubResourceOpener) OpenResource(name string) (resources.Opened, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resources.Opened{}, err
	}
	return s.ReturnOpenResource, nil
}
