// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/introspection"
	"github.com/juju/juju/pubsub/agent"
	_ "github.com/juju/juju/state"
)

type suite struct {
	testhelpers.IsolationSuite
}

func TestSuite(t *tctesting.T) {
	tc.Run(t, &suite{})
}

func (s *suite) TestConfigValidation(c *tc.C) {
	w, err := introspection.NewWorker(introspection.Config{})
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "empty SocketName not valid")
	w, err = introspection.NewWorker(introspection.Config{
		SocketName: "socket",
	})
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "nil PrometheusGatherer not valid")
	w, err = introspection.NewWorker(introspection.Config{
		SocketName:         "socket",
		PrometheusGatherer: newPrometheusGatherer(),
		LocalHub:           pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{}),
	})
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "nil Clock not valid")
}

func (s *suite) TestStartStop(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	socketName := path.Join(c.MkDir(), "introspection-test")
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         socketName,
		PrometheusGatherer: prometheus.NewRegistry(),
	})
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckKill(c, w)
}

type introspectionSuite struct {
	testhelpers.IsolationSuite

	name       string
	worker     worker.Worker
	reporter   introspection.DepEngineReporter
	gatherer   prometheus.Gatherer
	recorder   presence.Recorder
	localHub   *pubsub.SimpleHub
	centralHub introspection.StructuredHub
	clock      *testclock.Clock
}

func TestIntrospectionSuite(t *tctesting.T) {
	tc.Run(t, &introspectionSuite{})
}

func (s *introspectionSuite) SetUpTest(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}
	s.IsolationSuite.SetUpTest(c)
	s.reporter = nil
	s.worker = nil
	s.recorder = nil
	s.gatherer = newPrometheusGatherer()
	s.localHub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{Logger: loggo.GetLogger("test.localhub")})
	s.centralHub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{Logger: loggo.GetLogger("test.centralhub")})
	s.clock = testclock.NewClock(time.Now())
	s.startWorker(c)
}

func (s *introspectionSuite) startWorker(c *tc.C) {
	s.name = path.Join(c.MkDir(), fmt.Sprintf("introspection-test-%d", os.Getpid()))
	w, err := introspection.NewWorker(introspection.Config{
		SocketName:         s.name,
		DepEngine:          s.reporter,
		PrometheusGatherer: s.gatherer,
		Presence:           s.recorder,
		Clock:              s.clock,
		LocalHub:           s.localHub,
		CentralHub:         s.centralHub,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *tc.C) {
		workertest.CleanKill(c, w)
	})
}

func (s *introspectionSuite) call(c *tc.C, path string) *http.Response {
	client := unixSocketHTTPClient(s.name)
	c.Assert(strings.HasPrefix(path, "/"), tc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, tc.ErrorIsNil)

	resp, err := client.Get(targetURL.String())
	c.Assert(err, tc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) post(c *tc.C, path string, values url.Values) *http.Response {
	client := unixSocketHTTPClient(s.name)
	c.Assert(strings.HasPrefix(path, "/"), tc.IsTrue)
	targetURL, err := url.Parse("http://unix.socket" + path)
	c.Assert(err, tc.ErrorIsNil)

	resp, err := client.PostForm(targetURL.String(), values)
	c.Assert(err, tc.ErrorIsNil)
	return resp
}

func (s *introspectionSuite) body(c *tc.C, r *http.Response) string {
	response, err := io.ReadAll(r.Body)
	c.Assert(err, tc.ErrorIsNil)
	return string(response)
}

func (s *introspectionSuite) assertBody(c *tc.C, response *http.Response, value string) {
	body := s.body(c, response)
	c.Assert(body, tc.Equals, value+"\n")
}

func (s *introspectionSuite) assertContains(c *tc.C, value, expected string) {
	c.Assert(strings.Contains(value, expected), tc.IsTrue,
		tc.Commentf("missing %q in %v", expected, value))
}

func (s *introspectionSuite) assertBodyContains(c *tc.C, response *http.Response, value string) {
	body := s.body(c, response)
	s.assertContains(c, body, value)
}

func (s *introspectionSuite) TestCmdLine(c *tc.C) {
	response := s.call(c, "/debug/pprof/cmdline")
	s.assertBodyContains(c, response, "/introspection.test")
}

func (s *introspectionSuite) TestGoroutineProfile(c *tc.C) {
	response := s.call(c, "/debug/pprof/goroutine?debug=1")
	body := s.body(c, response)
	c.Check(body, tc.Matches, `(?s)^goroutine profile: total \d+.*`)
}

func (s *introspectionSuite) TestTrace(c *tc.C) {
	response := s.call(c, "/debug/pprof/trace?seconds=1")
	c.Assert(response.Header.Get("Content-Type"), tc.Equals, "application/octet-stream")
}

func (s *introspectionSuite) TestMissingDepEngineReporter(c *tc.C) {
	response := s.call(c, "/depengine")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing dependency engine reporter")
}

func (s *introspectionSuite) TestMissingStatePoolReporter(c *tc.C) {
	response := s.call(c, "/statepool")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, `"State Pool" introspection not supported`)
}

func (s *introspectionSuite) TestMissingPubSubReporter(c *tc.C) {
	response := s.call(c, "/pubsub")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, `"PubSub Report" introspection not supported`)
}

func (s *introspectionSuite) TestMissingMachineLock(c *tc.C) {
	response := s.call(c, "/machinelock")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "missing machine lock reporter")
}

func (s *introspectionSuite) TestStateTrackerReporter(c *tc.C) {
	response := s.call(c, "/debug/pprof/juju/state/tracker?debug=1")
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	s.assertBodyContains(c, response, "juju/state/tracker profile: total")
}

func (s *introspectionSuite) TestEngineReporter(c *tc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.reporter = &reporter{
		values: map[string]interface{}{
			"working": true,
		},
	}
	s.startWorker(c)
	response := s.call(c, "/depengine")
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	// TODO: perhaps make the output of the dependency engine YAML parseable.
	// This could be done by having the first line start with a '#'.
	s.assertBody(c, response, `
Dependency Engine Report

working: true`[1:])
}

func (s *introspectionSuite) TestMissingPresenceReporter(c *tc.C) {
	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, `"Presence" introspection not supported`)
}

func (s *introspectionSuite) TestDisabledPresenceReporter(c *tc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.startWorker(c)

	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, tc.Equals, http.StatusNotFound)
	s.assertBody(c, response, "agent is not an apiserver")
}

func (s *introspectionSuite) TestEnabledPresenceReporter(c *tc.C) {
	// We need to make sure the existing worker is shut down
	// so we can connect to the socket.
	workertest.CheckKill(c, s.worker)
	s.recorder = presence.New(testclock.NewClock(time.Now()))
	s.recorder.Enable()
	s.recorder.Connect("server", "model-uuid", "agent-1", 42, false, "")
	s.startWorker(c)

	response := s.call(c, "/presence")
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	s.assertBody(c, response, `
[model-uuid]

AGENT    SERVER  CONN ID  STATUS
agent-1  server  42       alive
`[1:])
}

func (s *introspectionSuite) TestPrometheusMetrics(c *tc.C) {
	response := s.call(c, "/metrics")
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	body := s.body(c, response)
	s.assertContains(c, body, "# HELP tau Tau")
	s.assertContains(c, body, "# TYPE tau counter")
	s.assertContains(c, body, "tau 6.283185")
}

func (s *introspectionSuite) TestUnitMissingAction(c *tc.C) {
	response := s.call(c, "/units")
	c.Assert(response.StatusCode, tc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing action")
}

func (s *introspectionSuite) TestUnitUnknownAction(c *tc.C) {
	response := s.post(c, "/units", url.Values{"action": {"foo"}})
	c.Assert(response.StatusCode, tc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, `unknown action: "foo"`)
}

func (s *introspectionSuite) TestUnitStartWithGet(c *tc.C) {
	response := s.call(c, "/units?action=start")
	c.Assert(response.StatusCode, tc.Equals, http.StatusMethodNotAllowed)
	s.assertBody(c, response, `start requires a POST request, got "GET"`)
}

func (s *introspectionSuite) TestUnitStartMissingUnits(c *tc.C) {
	response := s.post(c, "/units", url.Values{"action": {"start"}})
	c.Assert(response.StatusCode, tc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing unit")
}

func (s *introspectionSuite) TestUnitStartUnits(c *tc.C) {
	unsub := s.localHub.Subscribe(agent.StartUnitTopic, func(topic string, data interface{}) {
		_, ok := data.(agent.Units)
		if !ok {
			c.Fatalf("bad data type: %T", data)
			return
		}
		s.localHub.Publish(agent.StartUnitResponseTopic, agent.StartStopResponse{
			"one": "started",
			"two": "not found",
		})
	})
	defer unsub()

	response := s.post(c, "/units", url.Values{"action": {"start"}, "unit": {"one", "two"}})
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	s.assertBody(c, response, "one: started\ntwo: not found")
}

func (s *introspectionSuite) TestUnitStopWithGet(c *tc.C) {
	response := s.call(c, "/units?action=stop")
	c.Assert(response.StatusCode, tc.Equals, http.StatusMethodNotAllowed)
	s.assertBody(c, response, `stop requires a POST request, got "GET"`)
}

func (s *introspectionSuite) TestUnitStopMissingUnits(c *tc.C) {
	response := s.post(c, "/units", url.Values{"action": {"stop"}})
	c.Assert(response.StatusCode, tc.Equals, http.StatusBadRequest)
	s.assertBody(c, response, "missing unit")
}

func (s *introspectionSuite) TestUnitStopUnits(c *tc.C) {
	unsub := s.localHub.Subscribe(agent.StopUnitTopic, func(topic string, data interface{}) {
		_, ok := data.(agent.Units)
		if !ok {
			c.Fatalf("bad data type: %T", data)
			return
		}
		s.localHub.Publish(agent.StopUnitResponseTopic, agent.StartStopResponse{
			"one": "stopped",
			"two": "not found",
		})
	})
	defer unsub()

	response := s.post(c, "/units", url.Values{"action": {"stop"}, "unit": {"one", "two"}})
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	s.assertBody(c, response, "one: stopped\ntwo: not found")
}

func (s *introspectionSuite) TestUnitStatus(c *tc.C) {
	unsub := s.localHub.Subscribe(agent.UnitStatusTopic, func(string, interface{}) {
		s.localHub.Publish(agent.UnitStatusResponseTopic, agent.Status{
			"one": "running",
			"two": "stopped",
		})
	})
	defer unsub()

	response := s.call(c, "/units?action=status")
	c.Assert(response.StatusCode, tc.Equals, http.StatusOK)
	s.assertBody(c, response, `
one: running
two: stopped`[1:])
}

func (s *introspectionSuite) TestUnitStatusTimeout(c *tc.C) {
	unsub := s.localHub.Subscribe(agent.UnitStatusTopic, func(string, interface{}) {
		s.clock.WaitAdvance(10*time.Second, time.Second, 1)
	})
	defer unsub()

	response := s.call(c, "/units?action=status")
	c.Assert(response.StatusCode, tc.Equals, http.StatusInternalServerError)
	s.assertBody(c, response, "response timed out")
}

type reporter struct {
	values map[string]interface{}
}

func (r *reporter) Report() map[string]interface{} {
	return r.values
}

func newPrometheusGatherer() prometheus.Gatherer {
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "tau", Help: "Tau."})
	counter.Add(6.283185)
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(counter)
	return r
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: unixSocketHTTPTransport(socketPath),
		Timeout:   15 * time.Second,
	}
}

func unixSocketHTTPTransport(socketPath string) *http.Transport {
	return &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
}
