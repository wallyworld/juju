// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	tctesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/gorilla/websocket"
	jujuerrors "github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/errors"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/testserver"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var fastDialOpts = api.DialOpts{}

type serverSuite struct {
	jujutesting.JujuConnSuite
}

func TestServerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &serverSuite{})
}

func (s *serverSuite) TestStop(c *tc.C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv := testserver.NewServer(c, s.StatePool, s.Controller)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// Note we can't use openAs because we're not connecting to
	info := srv.Info
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = s.Model.ModelTag()

	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()

	_, err = apimachiner.NewState(st).Machine(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)

	err = srv.Stop()
	c.Assert(err, tc.ErrorIsNil)

	_, err = apimachiner.NewState(st).Machine(machine.MachineTag())
	// The client has not necessarily seen the server shutdown yet, so there
	// are multiple possible errors. All we should care about is that there is
	// an error, not what the error actually is.
	c.Assert(err, tc.NotNil)

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestAPIServerCanListenOnBothIPv4AndIPv6(c *tc.C) {
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Start our own instance of the server listening on
	// both IPv4 and IPv6 localhost addresses and an ephemeral port.
	srv := testserver.NewServer(c, s.StatePool, s.Controller)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	info := srv.Info
	port := info.Ports()[0]
	portString := fmt.Sprintf("%d", port)

	// Now connect twice - using IPv4 and IPv6 endpoints.
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = s.Model.ModelTag()

	ipv4State, err := api.Open(info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer ipv4State.Close()
	ipv4Addr := ipv4State.Addr()
	c.Assert(ipv4Addr.String(), tc.Equals, "wss://"+net.JoinHostPort("localhost", portString))
	c.Assert(ipv4State.APIHostPorts(), tc.DeepEquals, []network.MachineHostPorts{
		network.NewMachineHostPorts(port, "localhost"),
	})

	_, err = apimachiner.NewState(ipv4State).Machine(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)

	info.Addrs = []string{net.JoinHostPort("::1", portString)}
	ipv6State, err := api.Open(info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer ipv6State.Close()
	ipv6Addr := ipv6State.Addr()
	c.Assert(ipv6Addr.String(), tc.Equals, "wss://"+net.JoinHostPort("::1", portString))
	c.Assert(ipv6State.APIHostPorts(), tc.DeepEquals, []network.MachineHostPorts{
		network.NewMachineHostPorts(port, "::1"),
	})

	_, err = apimachiner.NewState(ipv6State).Machine(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestOpenAsMachineErrors(c *tc.C) {
	assertNotProvisioned := func(err error) {
		c.Assert(err, tc.NotNil)
		c.Assert(err, tc.Satisfies, params.IsCodeNotProvisioned)
		c.Assert(err, tc.ErrorMatches, `machine \d+ not provisioned \(not provisioned\)`)
	}

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// This does almost exactly the same as OpenAPIAsMachine but checks
	// for failures instead.
	info := s.APIInfo(c)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "invalid-nonce"
	st, err := api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)

	// Try with empty nonce as well.
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)

	// Finally, with the correct one succeeds.
	info.Nonce = "fake_nonce"
	st, err = api.Open(info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.NotNil)
	st.Close()

	// Now add another machine, intentionally unprovisioned.
	stm1, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = stm1.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)

	// Try connecting, it will fail.
	info.Tag = stm1.Tag()
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)
}

func dialWebsocket(c *tc.C, addr, path string) (*websocket.Conn, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocketFromURL function.
	url := fmt.Sprintf("wss://%s%s", addr, path)
	header := make(http.Header)
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), tc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "anything"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	conn, _, err := dialer.Dial(url, header)
	return conn, err
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *tc.C) {
	// We expose the API at '/api', '/' (controller-only), and at '/ModelUUID/api'
	// for the correct location, but other paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	srv := testserver.NewServer(c, s.StatePool, s.Controller)
	defer assertStop(c, srv)

	// We have to use 'localhost' because that is what the TLS cert says.
	addr := fmt.Sprintf("localhost:%d", srv.Info.Ports()[0])

	// '/api' should be fine
	conn, err := dialWebsocket(c, addr, "/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/`' should be fine
	conn, err = dialWebsocket(c, addr, "/")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/model/MODELUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/model/deadbeef-1234-5678-0123-0123456789ab/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath")
	// Unfortunately gorilla/websocket just returns bad handshake, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, tc.ErrorMatches, `websocket: bad handshake`)
	c.Assert(conn, tc.IsNil)
}

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Stop() error {
	r.stopped = true
	return nil
}

func (s *serverSuite) bootstrapHasPermissionTest(c *tc.C) (*state.User, names.ControllerTag) {
	u, err := s.State.AddUser("foobar", "Foo Bar", "password", "read")
	c.Assert(err, tc.ErrorIsNil)
	user := u.UserTag()

	ctag, err := names.ParseControllerTag("controller-" + s.State.ControllerUUID())
	c.Assert(err, tc.ErrorIsNil)
	access, err := s.State.UserPermission(user, ctag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.LoginAccess)
	return u, ctag
}

func (s *serverSuite) TestAPIHandlerHasPermissionLogin(c *tc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool, s.State, u)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, false)
}

func (s *serverSuite) TestAPIHandlerHasPermissionSuperUser(c *tc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)
	user := u.UserTag()

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool, s.State, u)
	defer handler.Kill()

	ua, err := s.State.SetUserAccess(user, ctag, permission.SuperuserAccess)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ua.Access, tc.Equals, permission.SuperuserAccess)

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, true)
}

func (s *serverSuite) TestAPIHandlerHasPermissionLoginToken(c *tc.C) {
	user := names.NewUserTag("fred")
	token, err := apitesting.NewJWT(apitesting.JWTParams{
		Controller: coretesting.ControllerTag.Id(),
		User:       user.String(),
		Access: map[string]any{
			coretesting.ControllerTag.String(): "superuser",
			coretesting.ModelTag.String():      "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	delegator := &jwt.PermissionDelegator{token}
	handler, _ := apiserver.TestingAPIHandlerWithToken(c, s.StatePool, s.State, token, delegator)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, coretesting.ControllerTag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, coretesting.ControllerTag, true)
	apiserver.AssertHasPermission(c, handler, permission.WriteAccess, coretesting.ModelTag, true)
}

func (s *serverSuite) TestAPIHandlerMissingPermissionLoginToken(c *tc.C) {
	user := names.NewUserTag("fred")
	token, err := apitesting.NewJWT(apitesting.JWTParams{
		Controller: coretesting.ControllerTag.Id(),
		User:       user.String(),
		Access: map[string]any{
			coretesting.ControllerTag.String(): "superuser",
			coretesting.ModelTag.String():      "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	delegator := &jwt.PermissionDelegator{token}
	handler, _ := apiserver.TestingAPIHandlerWithToken(c, s.StatePool, s.State, token, delegator)
	defer handler.Kill()
	err = handler.HasPermission(permission.AdminAccess, coretesting.ModelTag)
	var reqError *errors.AccessRequiredError
	c.Assert(jujuerrors.As(err, &reqError), tc.IsTrue)
	c.Assert(reqError, tc.DeepEquals, &errors.AccessRequiredError{
		RequiredAccess: map[names.Tag]permission.Access{
			coretesting.ModelTag: permission.AdminAccess,
		},
	})
}

func (s *serverSuite) TestAPIHandlerTeardownInitialModel(c *tc.C) {
	s.checkAPIHandlerTeardown(c, s.State, s.State)
}

func (s *serverSuite) TestAPIHandlerTeardownOtherModel(c *tc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	s.checkAPIHandlerTeardown(c, s.State, otherState)
}

func (s *serverSuite) TestAPIHandlerConnectedModel(c *tc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	handler, _ := apiserver.TestingAPIHandler(c, s.StatePool, otherState)
	defer handler.Kill()
	c.Check(handler.ConnectedModel(), tc.Equals, otherState.ModelUUID())
}

func (s *serverSuite) TestClosesStateFromPool(c *tc.C) {
	cfg := testserver.DefaultServerConfig(c, nil)
	cfg.Controller = s.Controller
	server := testserver.NewServerWithConfig(c, s.StatePool, cfg)
	defer assertStop(c, server)

	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	// Ensure the model's in the pool but not referenced.
	st, err := s.StatePool.Get(otherState.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	st.Release()

	// Make a request for the model API to check it releases
	// state back into the pool once the connection is closed.
	addr := fmt.Sprintf("localhost:%d", server.Info.Ports()[0])
	conn, err := dialWebsocket(c, addr, fmt.Sprintf("/model/%s/api", st.ModelUUID()))
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// Don't make an assertion about whether the remove call returns
	// true - that's dependent on whether the server has reacted to
	// the connection being closed yet, so it's racy.
	_, err = s.StatePool.Remove(otherState.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	assertStateBecomesClosed(c, st.State)
}

func assertStateBecomesClosed(c *tc.C, st *state.State) {
	// This is gross but I can't see any other way to check for
	// closedness outside the state package.
	checkModel := func() {
		attempt := utils.AttemptStrategy{
			Total: coretesting.LongWait,
			Delay: coretesting.ShortWait,
		}
		for a := attempt.Start(); a.Next(); {
			// This will panic once the state is closed.
			_, _ = st.Model()
		}
		// If we got here then st is still open.
		st.Close()
	}
	c.Assert(checkModel, tc.PanicMatches, "Session already closed")
}

func (s *serverSuite) checkAPIHandlerTeardown(c *tc.C, srvSt, st *state.State) {
	handler, resources := apiserver.TestingAPIHandler(c, s.StatePool, st)
	resource := new(fakeResource)
	resources.Register(resource)

	c.Assert(resource.stopped, tc.IsFalse)
	handler.Kill()
	c.Assert(resource.stopped, tc.IsTrue)
}

type stopper interface {
	Stop() error
}

func assertStop(c *tc.C, stopper stopper) {
	c.Assert(stopper.Stop(), tc.IsNil)
}

type mockAuthenticator struct {
}

func (a *mockAuthenticator) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, nil
}

func (a *mockAuthenticator) AuthenticateLoginRequest(
	_ context.Context,
	_,
	_ string,
	authParams authentication.AuthParams,
) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{
		Entity: &mockEntity{tag: authParams.AuthTag},
	}, nil
}

func (a *mockAuthenticator) CreateLocalLoginMacaroon(ctx context.Context, tag names.UserTag, version bakery.Version) (*macaroon.Macaroon, error) {
	return nil, nil
}

type mockEntity struct {
	tag names.Tag
}

func (e *mockEntity) Tag() names.Tag {
	return e.tag
}
