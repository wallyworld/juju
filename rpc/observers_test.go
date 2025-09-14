// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc"
)

type multiplexerSuite struct {
	testhelpers.IsolationSuite
}

func TestMultiplexerSuite(t *tctesting.T) {
	tc.Run(t, &multiplexerSuite{})
}

func (*multiplexerSuite) TestServerReply_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.RPCInstance{
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
	}

	o := rpc.NewObserverMultiplexer(observers[0], observers[1])
	var (
		req  rpc.Request
		hdr  rpc.Header
		body string
	)
	o.ServerReply(req, &hdr, body)

	for _, f := range observers {
		f.CheckCall(c, 0, "ServerReply", req, &hdr, body)
	}
}

func (*multiplexerSuite) TestServerRequest_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.RPCInstance{
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
	}

	o := rpc.NewObserverMultiplexer(observers[0], observers[1])
	var (
		hdr  rpc.Header
		body string
	)
	o.ServerRequest(&hdr, body)

	for _, f := range observers {
		f.CheckCall(c, 0, "ServerRequest", &hdr, body)
	}
}
