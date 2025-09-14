// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	tctesting "testing"

	"github.com/gorilla/websocket"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

type dispatchSuite struct {
	testing.BaseSuite

	server     *httptest.Server
	serverAddr string
	ready      chan struct{}
}

func TestDispatchSuite(t *tctesting.T) {
	tc.Run(t, &dispatchSuite{})
}

func (s *dispatchSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	rpcServer := func(ws *websocket.Conn) {
		codec := jsoncodec.NewWebsocket(ws)
		conn := rpc.NewConn(codec, nil)

		conn.Serve(&DispatchRoot{}, nil, nil)
		conn.Start(c.Context())

		<-conn.Dead()
	}
	http.Handle("/rpc", websocketHandler(rpcServer))
	s.server = httptest.NewServer(nil)
	s.serverAddr = s.server.Listener.Addr().String()
	s.ready = make(chan struct{}, 1)
	s.AddCleanup(func(*tc.C) {
		s.server.Close()
	})
}

var wsUpgrader = &websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func websocketHandler(f func(*websocket.Conn)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := wsUpgrader.Upgrade(w, req, nil)
		if err == nil {
			f(c)
		}
	})
}

func (s *dispatchSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	loggo.GetLogger("juju.rpc").SetLogLevel(loggo.TRACE)
}

func (s *dispatchSuite) TestWSWithoutParamsV0(c *tc.C) {
	resp := s.request(c, `{"RequestId":1,"Type": "DispatchDummy","Id": "without","Request":"DoSomething"}`)
	s.assertResponse(c, resp, `{"RequestId":1,"Response":{}}`)
}

func (s *dispatchSuite) TestWSWithParamsV0(c *tc.C) {
	resp := s.request(c, `{"RequestId":2,"Type": "DispatchDummy","Id": "with","Request":"DoSomething", "Params": {}}`)
	s.assertResponse(c, resp, `{"RequestId":2,"Response":{}}`)
}

func (s *dispatchSuite) TestWSWithoutParamsV1(c *tc.C) {
	resp := s.request(c, `{"request-id":1,"type": "DispatchDummy","id": "without","request":"DoSomething"}`)
	s.assertResponse(c, resp, `{"request-id":1,"response":{}}`)
}

func (s *dispatchSuite) TestWSWithParamsV1(c *tc.C) {
	resp := s.request(c, `{"request-id":2,"type": "DispatchDummy","id": "with","request":"DoSomething", "params": {}}`)
	s.assertResponse(c, resp, `{"request-id":2,"response":{}}`)
}

func (s *dispatchSuite) assertResponse(c *tc.C, obtained, expected string) {
	c.Assert(obtained, tc.Equals, expected+"\n")
}

// request performs one request to the test server via websockets.
func (s *dispatchSuite) request(c *tc.C, req string) string {
	url := fmt.Sprintf("ws://%s/rpc", s.serverAddr)
	ws, _, err := websocket.DefaultDialer.Dial(url, http.Header{
		"Origin": {"http://localhost"},
	})
	c.Assert(err, tc.ErrorIsNil)

	reqdata := []byte(req)
	err = ws.WriteMessage(websocket.TextMessage, reqdata)
	c.Assert(err, tc.ErrorIsNil)

	_, resp, err := ws.ReadMessage()
	c.Assert(err, tc.ErrorIsNil)

	err = ws.Close()
	c.Assert(err, tc.ErrorIsNil)

	return string(resp)
}

// DispatchRoot simulates the root for the test.
type DispatchRoot struct{}

func (*DispatchRoot) DispatchDummy(id string) (*DispatchDummy, error) {
	return &DispatchDummy{}, nil
}

// DispatchDummy is the type to whish the request is dispatched.
type DispatchDummy struct{}

func (d *DispatchDummy) DoSomething() {}
