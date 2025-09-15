// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"net/http"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/internal/testhelpers"
)

type multiplexerSuite struct {
	testhelpers.IsolationSuite
}

func TestMultiplexerSuite(t *tctesting.T) {
	tc.Run(t, &multiplexerSuite{})
}

func (*multiplexerSuite) TestObserverFactoryMultiplexer_CallsAllFactories(c *tc.C) {
	callCount := 0
	factories := []observer.ObserverFactory{
		func() observer.Observer { callCount++; return nil },
		func() observer.Observer { callCount++; return nil },
	}

	newMultiplexObserver := observer.ObserverFactoryMultiplexer(factories...)
	c.Assert(callCount, tc.Equals, 0)

	multiplexedObserver := newMultiplexObserver()
	c.Check(multiplexedObserver, tc.NotNil)
	c.Check(callCount, tc.Equals, 2)
}

func (*multiplexerSuite) TestJoin_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	var req http.Request
	o.Join(&req, 1234)

	for _, f := range observers {
		f.CheckCall(c, 0, "Join", &req, uint64(1234))
	}
}

func (*multiplexerSuite) TestLeave_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	o.Leave()

	for _, f := range observers {
		f.CheckCall(c, 0, "Leave")
	}
}

func (*multiplexerSuite) TestRPCObserver_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	o.RPCObserver()

	for _, f := range observers {
		f.CheckCallNames(c, "RPCObserver")
	}
}

func (*multiplexerSuite) TestLogin_CallsAllObservers(c *tc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	entity := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	fromController := false
	userData := "foo"
	o.Login(entity, model, fromController, userData)

	for _, f := range observers {
		f.CheckCall(c, 0, "Login", entity, model, fromController, userData)
	}
}
