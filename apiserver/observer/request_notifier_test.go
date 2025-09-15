// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/pubsub/apiserver"
)

type RequestObserverSuite struct {
	testhelpers.IsolationSuite
}

func TestRequestObserverSuite(t *tctesting.T) {
	tc.Run(t, &RequestObserverSuite{})
}

func (*RequestObserverSuite) makeNotifier(c *tc.C) (*observer.RequestObserver, *connectionHub) {
	hub := &connectionHub{c: c}
	return observer.NewRequestObserver(observer.RequestObserverContext{
		Clock:  testclock.NewClock(time.Now()),
		Hub:    hub,
		Logger: loggo.GetLogger("test"),
	}), hub
}

func (s *RequestObserverSuite) TestAgentConnectionPublished(c *tc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(agent, model, false, "user data")

	c.Assert(hub.called, tc.Equals, 1)
	c.Assert(hub.topic, tc.Equals, apiserver.ConnectTopic)
	c.Assert(hub.details, tc.DeepEquals, apiserver.APIConnection{
		AgentTag:     "machine-42",
		ModelUUID:    "fake-uuid",
		UserData:     "user data",
		ConnectionID: 0,
	})
}

func (s *RequestObserverSuite) assertControllerAgentConnectionPublished(c *tc.C, agent names.Tag) {
	notifier, hub := s.makeNotifier(c)

	model := names.NewModelTag("fake-uuid")
	notifier.Login(agent, model, true, "user data")

	c.Assert(hub.called, tc.Equals, 1)
	c.Assert(hub.topic, tc.Equals, apiserver.ConnectTopic)
	c.Assert(hub.details, tc.DeepEquals, apiserver.APIConnection{
		AgentTag:        agent.String(),
		ModelUUID:       "fake-uuid",
		ControllerAgent: true,
		UserData:        "user data",
		ConnectionID:    0,
	})
}

func (s *RequestObserverSuite) TestControllerMachineAgentConnectionPublished(c *tc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewMachineTag("2"))
}

func (s *RequestObserverSuite) TestControllerUnitAgentConnectionPublished(c *tc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewUnitTag("mariadb/0"))
}

func (s *RequestObserverSuite) TestControllerApplicationAgentConnectionPublished(c *tc.C) {
	s.assertControllerAgentConnectionPublished(c, names.NewApplicationTag("gitlab"))
}

func (s *RequestObserverSuite) TestUserConnectionsNotPublished(c *tc.C) {
	notifier, hub := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	notifier.Login(user, model, false, "user data")

	c.Assert(hub.called, tc.Equals, 0)
}

func (s *RequestObserverSuite) TestAgentDisconnectionPublished(c *tc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(agent, model, false, "user data")
	notifier.Leave()

	c.Assert(hub.called, tc.Equals, 2)
	c.Assert(hub.topic, tc.Equals, apiserver.DisconnectTopic)
	c.Assert(hub.details, tc.DeepEquals, apiserver.APIConnection{
		AgentTag:     "machine-42",
		ModelUUID:    "fake-uuid",
		ConnectionID: 0,
	})
}

func (s *RequestObserverSuite) TestControllerAgentDisconnectionPublished(c *tc.C) {
	notifier, hub := s.makeNotifier(c)

	agent := names.NewMachineTag("2")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(agent, model, true, "user data")
	notifier.Leave()

	c.Assert(hub.called, tc.Equals, 2)
	c.Assert(hub.topic, tc.Equals, apiserver.DisconnectTopic)
	c.Assert(hub.details, tc.DeepEquals, apiserver.APIConnection{
		AgentTag:        "machine-2",
		ModelUUID:       "fake-uuid",
		ControllerAgent: true,
		ConnectionID:    0,
	})
}

func (s *RequestObserverSuite) TestUserDisconnectionsNotPublished(c *tc.C) {
	notifier, hub := s.makeNotifier(c)

	user := names.NewUserTag("bob")
	model := names.NewModelTag("fake-uuid")
	// All details are saved from Login.
	notifier.Login(user, model, false, "user data")
	notifier.Leave()

	c.Assert(hub.called, tc.Equals, 0)
}

type connectionHub struct {
	c       *tc.C
	called  int
	topic   string
	details apiserver.APIConnection
}

func (hub *connectionHub) Publish(topic string, data interface{}) (func(), error) {
	hub.called++
	hub.topic = topic
	hub.details = data.(apiserver.APIConnection)
	return func() {}, nil
}
