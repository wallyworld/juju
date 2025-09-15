// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"github.com/kr/pretty"

	corepresence "github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/presence"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/pubsub/forwarder"
)

type PresenceSuite struct {
	testhelpers.IsolationSuite
	hub      *pubsub.StructuredHub
	clock    *testclock.Clock
	recorder corepresence.Recorder
	config   presence.WorkerConfig
}

func TestPresenceSuite(t *tctesting.T) {
	tc.Run(t, &PresenceSuite{})
}

func (s *PresenceSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.hub = centralhub.New(ourTag, centralhub.PubsubNoOpMetrics{})
	s.clock = testclock.NewClock(time.Time{})
	s.recorder = corepresence.New(s.clock)
	s.recorder.Enable()
	s.config = presence.WorkerConfig{
		Origin:   ourServer,
		Hub:      s.hub,
		Recorder: s.recorder,
		Logger:   loggo.GetLogger("test"),
	}
	loggo.ConfigureLoggers("<root>=trace")
}

func (s *PresenceSuite) worker(c *tc.C) worker.Worker {
	w, err := presence.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *PresenceSuite) TestWorkerConfigMissingOrigin(c *tc.C) {
	s.config.Origin = ""
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing origin not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingHub(c *tc.C) {
	s.config.Hub = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing hub not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingRecorder(c *tc.C) {
	s.config.Recorder = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing recorder not valid")
}

func (s *PresenceSuite) TestWorkerConfigMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "missing logger not valid")
}

func (s *PresenceSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	w, err := presence.NewWorker(presence.WorkerConfig{})
	c.Check(err, tc.ErrorMatches, "missing origin not valid")
	c.Check(w, tc.IsNil)
}

func (s *PresenceSuite) TestWorkerDies(c *tc.C) {
	w := s.worker(c)
	workertest.CleanKill(c, w)
}

func (s *PresenceSuite) TestReport(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	s.recorder.Connect("machine-0", "model-uuid", "agent", 1, false, "")
	s.recorder.Connect("machine-0", "model-uuid", "agent", 2, false, "")
	s.recorder.Connect("machine-0", "model-uuid", "agent", 3, false, "")
	s.recorder.Connect("machine-1", "model-uuid", "agent", 4, false, "")
	s.recorder.Connect("machine-1", "model-uuid", "agent", 5, false, "")
	s.recorder.Connect("machine-2", "model-uuid", "agent", 6, false, "")

	reporter, ok := w.(worker.Reporter)
	c.Assert(ok, tc.IsTrue)
	c.Assert(reporter.Report(), tc.DeepEquals, map[string]interface{}{
		"machine-0": 3,
		"machine-1": 2,
		"machine-2": 1,
	})
}

func (s *PresenceSuite) TestForwarderConnectToOther(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, tc.ErrorIsNil)
		c.Check(data.Target, tc.Equals, otherServer)
		c.Check(data.Origin, tc.Equals, ourServer)
		close(done)
	})
	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// When connections are established from us to them, we ask for their presence info.
	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: ourServer, Target: otherServer})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestForwarderConnectFromOther(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, tc.ErrorIsNil)
		c.Check(data.Target, tc.Equals, otherServer)
		c.Check(data.Origin, tc.Equals, ourServer)
		close(done)
	})
	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// When connections are established from them to us, we ask for their presence info.
	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: ourServer})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestForwarderConnectOtherIgnored(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	called := make(chan struct{})

	unsub, err := s.hub.Subscribe(apiserver.PresenceRequestTopic, func(topic string, data apiserver.OriginTarget, err error) {
		c.Logf("handler called for %q", topic)
		close(called)
	})
	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	_, err = s.hub.Publish(
		forwarder.ConnectedTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: "machine-8"})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertNotCalled(c, called)
}

func (s *PresenceSuite) TestForwarderDisconnectConnectFromOther(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		forwarder.DisconnectedTopic,
		apiserver.OriginTarget{Origin: ourServer, Target: otherServer})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, pubsub.Wait(done))
	s.AssertConnections(c, alive(agent1), missing(agent2))
}

func (s *PresenceSuite) TestForwarderDisconnectOthersIgnored(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		forwarder.DisconnectedTopic,
		apiserver.OriginTarget{Origin: "machine-7", Target: otherServer})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, pubsub.Wait(done))
	s.AssertConnections(c, alive(agent1), alive(agent2))
}

func (s *PresenceSuite) TestConnectTopic(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	done, err := s.hub.Publish(
		apiserver.ConnectTopic,
		apiserver.APIConnection{
			Origin:          "machine-5",
			ModelUUID:       "model-uuid",
			AgentTag:        "agent-2",
			ConnectionID:    42,
			ControllerAgent: true,
			UserData:        "test",
		})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, pubsub.Wait(done))
	s.AssertConnections(c, corepresence.Value{
		Model:           "model-uuid",
		Server:          "machine-5",
		Agent:           "agent-2",
		ConnectionID:    42,
		Status:          corepresence.Alive,
		ControllerAgent: true,
		UserData:        "test",
	})
}

func (s *PresenceSuite) TestDisconnectTopic(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2)

	done, err := s.hub.Publish(
		apiserver.DisconnectTopic,
		apiserver.APIConnection{
			Origin:       agent2.Server,
			ConnectionID: agent2.ConnectionID,
		})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, pubsub.Wait(done))
	s.AssertConnections(c, alive(agent1))
}

func (s *PresenceSuite) TestPresenceRequest(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2, agent3, agent4)

	done := make(chan struct{})
	unsub, err := s.hub.Subscribe(apiserver.PresenceResponseTopic, func(topic string, data apiserver.PresenceResponse, err error) {
		c.Logf("handler called for %q", topic)
		c.Check(err, tc.ErrorIsNil)
		c.Check(data.Origin, tc.Equals, ourServer)

		c.Check(data.Connections, tc.HasLen, 2)
		s.CheckConnection(c, data.Connections[0], agent1)
		s.CheckConnection(c, data.Connections[1], agent3)

		close(done)
	})
	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// When asked for our presence, we respond with the agents connected to us.
	_, err = s.hub.Publish(
		apiserver.PresenceRequestTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: ourServer})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, done)
}

func (s *PresenceSuite) TestPresenceRequestOtherServer(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	called := make(chan struct{})
	unsub, err := s.hub.Subscribe(apiserver.PresenceResponseTopic, func(topic string, data apiserver.PresenceResponse, err error) {
		c.Logf("handler called for %q", topic)
		close(called)
	})
	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// When presence requests come in for other servers, we ignore them.
	_, err = s.hub.Publish(
		apiserver.PresenceRequestTopic,
		apiserver.OriginTarget{Origin: otherServer, Target: "another"})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertNotCalled(c, called)
}

func (s *PresenceSuite) TestPresenceResponse(c *tc.C) {
	w := s.worker(c)
	defer workertest.CleanKill(c, w)

	connect(s.recorder, agent1, agent2, agent3, agent4)
	s.recorder.ServerDown(otherServer)

	// When connections information comes from other servers, we update our recorder.
	done, err := s.hub.Publish(
		apiserver.PresenceResponseTopic,
		apiserver.PresenceResponse{
			Origin: otherServer,
			Connections: []apiserver.APIConnection{
				apiConn(agent2), apiConn(agent4),
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertDone(c, pubsub.Wait(done))

	s.AssertConnections(c, alive(agent1), alive(agent2), alive(agent3), alive(agent4))
}

func (s *PresenceSuite) AssertDone(c *tc.C, called <-chan struct{}) {
	select {
	case <-called:
	case <-time.After(coretesting.LongWait):
		c.Fatal("event not handled")
	}
}

func (s *PresenceSuite) AssertNotCalled(c *tc.C, called <-chan struct{}) {
	select {
	case <-called:
		c.Fatal("event called unexpectedly")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *PresenceSuite) AssertConnections(c *tc.C, values ...corepresence.Value) {
	connections := s.recorder.Connections()
	c.Log(pretty.Sprint(connections))
	c.Assert(connections.Values(), tc.SameContents, values)
}

func (s *PresenceSuite) CheckConnection(c *tc.C, conn apiserver.APIConnection, agent corepresence.Value) {
	c.Check(conn.AgentTag, tc.Equals, agent.Agent)
	c.Check(conn.ControllerAgent, tc.Equals, agent.ControllerAgent)
	c.Check(conn.ModelUUID, tc.Equals, agent.Model)
	c.Check(conn.ConnectionID, tc.Equals, agent.ConnectionID)
	c.Check(conn.Origin, tc.Equals, agent.Server)
	c.Check(conn.UserData, tc.Equals, agent.UserData)
}

func apiConn(value corepresence.Value) apiserver.APIConnection {
	return apiserver.APIConnection{
		AgentTag:        value.Agent,
		ControllerAgent: value.ControllerAgent,
		ModelUUID:       value.Model,
		ConnectionID:    value.ConnectionID,
		Origin:          value.Server,
		UserData:        value.UserData,
	}
}

func alive(v corepresence.Value) corepresence.Value {
	v.Status = corepresence.Alive
	return v
}

func missing(v corepresence.Value) corepresence.Value {
	v.Status = corepresence.Missing
	return v
}

func connect(r corepresence.Recorder, values ...corepresence.Value) {
	for _, info := range values {
		r.Connect(info.Server, info.Model, info.Agent, info.ConnectionID, info.ControllerAgent, info.UserData)
	}
}

const modelUUID = "model-uuid"

var (
	ourTag      = names.NewMachineTag("1")
	ourServer   = ourTag.String()
	otherServer = "machine-2"
	agent1      = corepresence.Value{
		Model:        modelUUID,
		Server:       ourServer,
		Agent:        "machine-0",
		ConnectionID: 1237,
		UserData:     "foo",
	}
	agent2 = corepresence.Value{
		Model:        modelUUID,
		Server:       otherServer,
		Agent:        "machine-1",
		ConnectionID: 1238,
		UserData:     "bar",
	}
	agent3 = corepresence.Value{
		Model:        modelUUID,
		Server:       ourServer,
		Agent:        "unit-ubuntu-0",
		ConnectionID: 1239,
		UserData:     "baz",
	}
	agent4 = corepresence.Value{
		Model:        modelUUID,
		Server:       otherServer,
		Agent:        "unit-ubuntu-1",
		ConnectionID: 1240,
		UserData:     "splat",
	}
)
