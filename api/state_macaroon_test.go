// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"
	"net/url"
	tctesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/permission"
)

func TestMacaroonLoginSuite(t *tctesting.T) {
	tc.Run(t, &macaroonLoginSuite{})
}

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	client   api.Connection
	macSlice []macaroon.Slice
}

const testUserName = "testuser@somewhere"

func (s *macaroonLoginSuite) SetUpTest(c *tc.C) {
	s.MacaroonSuite.SetUpTest(c)
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	s.macSlice = []macaroon.Slice{{mac}}
	s.AddModelUser(c, testUserName)
	s.AddControllerUser(c, testUserName, permission.LoginAccess)
	info := s.APIInfo(c)
	info.Macaroons = nil
	info.SkipLogin = true
	s.client = s.OpenAPI(c, info, nil)
}

func (s *macaroonLoginSuite) TearDownTest(c *tc.C) {
	s.client.Close()
	s.MacaroonSuite.TearDownTest(c)
}

func (s *macaroonLoginSuite) TestSuccessfulLogin(c *tc.C) {
	s.DischargerLogin = func() string { return testUserName }
	err := s.client.Login(nil, "", "", s.macSlice)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestFailedToObtainDischargeLogin(c *tc.C) {
	err := s.client.Login(nil, "", "", s.macSlice)
	c.Assert(err, tc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
}

func (s *macaroonLoginSuite) TestConnectStream(c *tc.C) {
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	dischargeCount := 0
	s.DischargerLogin = func() string {
		dischargeCount++
		return testUserName
	}
	// First log into the regular API.
	err := s.client.Login(nil, "", "", s.macSlice)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dischargeCount, tc.Equals, 1)

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	conn, err := s.client.ConnectStream("/path", nil)
	c.Assert(err, tc.IsNil)
	defer conn.Close()
	connectURL, err := url.Parse(catcher.Location())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(connectURL.Path, tc.Equals, "/model/"+s.Model.ModelTag().Id()+"/path")
	c.Assert(dischargeCount, tc.Equals, 1)
}

func (s *macaroonLoginSuite) TestConnectStreamWithoutLogin(c *tc.C) {
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	conn, err := s.client.ConnectStream("/path", nil)
	c.Assert(err, tc.ErrorMatches, `cannot use ConnectStream without logging in`)
	c.Assert(conn, tc.Equals, nil)
}

func (s *macaroonLoginSuite) TestConnectStreamFailedDischarge(c *tc.C) {
	// This is really a test for ConnectStream, but to test ConnectStream's
	// discharge failing logic, we need an actual endpoint to test against,
	// and the debug-log endpoint makes a convenient example.

	var dischargeError bool
	s.DischargerLogin = func() string {
		if dischargeError {
			return ""
		}
		return testUserName
	}

	// Make an API connection that uses a cookie jar
	// that allows us to remove all cookies.
	jar := apitesting.NewClearableCookieJar()
	client := s.OpenAPI(c, nil, jar)

	// Ensure that the discharger won't discharge and try
	// logging in again. We should succeed in getting past
	// authorization because we have the cookies (but
	// the actual debug-log endpoint will return an error).
	dischargeError = true
	logArgs := url.Values{"noTail": []string{"true"}}
	conn, err := client.ConnectStream("/log", logArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)
	conn.Close()

	// Then delete all the cookies by deleting the cookie jar
	// and try again. The login should fail.
	jar.Clear()

	conn, err = client.ConnectStream("/log", logArgs)
	c.Assert(err, tc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(conn, tc.IsNil)
}

func (s *macaroonLoginSuite) TestConnectStreamWithDischargedMacaroons(c *tc.C) {
	// If the connection was created with already-discharged macaroons
	// (rather than acquiring them through the discharge dance), they
	// wouldn't get attached to the websocket request.
	// https://bugs.launchpad.net/juju/+bug/1650451
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	mac, err := macaroon.New([]byte("abc-123"), []byte("aurora gone"), "shankil butchers", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)

	s.DischargerLogin = func() string {
		return testUserName
	}

	info := s.APIInfo(c)
	info.Macaroons = []macaroon.Slice{{mac}}
	client := s.OpenAPI(c, info, nil)

	dischargedMacaroons, err := api.ExtractMacaroons(client)
	c.Assert(err, tc.IsNil)
	c.Assert(len(dischargedMacaroons), tc.Equals, 1)

	// Mirror the situation in migration logtransfer - the macaroon is
	// now stored in the auth service (so no further discharge is
	// needed), but we use a different client to connect to the log
	// stream, so the macaroon isn't in the cookie jar despite being
	// in the connection info.

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	s.DischargerLogin = nil

	info2 := s.APIInfo(c)
	info2.Macaroons = dischargedMacaroons

	client2 := s.OpenAPI(c, info2, nil)
	conn, err := client2.ConnectStream("/path", nil)
	c.Assert(err, tc.IsNil)
	defer conn.Close()

	headers := catcher.Headers()
	c.Assert(headers.Get(httpbakery.BakeryProtocolHeader), tc.Equals, "3")
	c.Assert(headers.Get("Cookie"), tc.HasPrefix, "macaroon-")
	assertHeaderMatchesMacaroon(c, headers, dischargedMacaroons[0])
}

func assertHeaderMatchesMacaroon(c *tc.C, header http.Header, macaroon macaroon.Slice) {
	req := http.Request{Header: header}
	actualCookie := req.Cookies()[0]
	expectedCookie, err := httpbakery.NewCookie(nil, macaroon)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actualCookie.Name, tc.Equals, expectedCookie.Name)
	c.Assert(actualCookie.Value, tc.Equals, expectedCookie.Value)
}
