// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"os"
	"path"
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/dependency"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/deployer"
	message "github.com/juju/juju/pubsub/agent"
	jv "github.com/juju/juju/version"
)

const veryShortWait = 5 * time.Millisecond

type NestedContextSuite struct {
	BaseSuite

	config  deployer.ContextConfig
	agent   agentconf.AgentConf
	hub     *pubsub.SimpleHub
	workers *unitWorkersStub
}

func TestNestedContextSuite(t *tctesting.T) {
	tc.Run(t, &NestedContextSuite{})
}

func (s *NestedContextSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	logger := loggo.GetLogger("test.nestedcontext")
	logger.SetLogLevel(loggo.TRACE)
	s.hub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		Logger: logger,
	})
	datadir := c.MkDir()
	machine := names.NewMachineTag("42")
	config, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir:         datadir,
				LogDir:          c.MkDir(),
				MetricsSpoolDir: c.MkDir(),
			},
			Tag:                    machine,
			Password:               "sekrit",
			Nonce:                  "unused",
			Controller:             testing.ControllerTag,
			Model:                  testing.ModelTag,
			APIAddresses:           []string{"a1:123", "a2:123"},
			CACert:                 "fake CACert",
			UpgradedToVersion:      jv.Current,
			AgentLogfileMaxBackups: 7,
			AgentLogfileMaxSizeMB:  123,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(config.Write(), tc.ErrorIsNil)

	s.agent = agentconf.NewAgentConf(datadir)
	err = s.agent.ReadConfig(machine.String())
	c.Assert(err, tc.ErrorIsNil)

	s.workers = &unitWorkersStub{
		started: make(chan string, 10), // eval size later
		stopped: make(chan string, 10), // eval size later
		logger:  logger,
	}
	s.config = deployer.ContextConfig{
		Agent:  s.agent,
		Clock:  clock.WallClock,
		Hub:    s.hub,
		Logger: logger,
		UnitEngineConfig: func() dependency.EngineConfig {
			return engine.DependencyEngineConfig(dependency.DefaultMetrics())
		},
		SetupLogging: func(c *loggo.Context, _ agent.Config) {
			c.GetLogger("").SetLogLevel(loggo.DEBUG)
		},
		UnitManifolds: s.workers.Manifolds,
	}
}

func (s *NestedContextSuite) TestConfigMissingAgentConfig(c *tc.C) {
	s.config.Agent = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing Agent not valid")
}

func (s *NestedContextSuite) TestConfigMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing Clock not valid")
}

func (s *NestedContextSuite) TestConfigMissingHub(c *tc.C) {
	s.config.Hub = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing Hub not valid")
}

func (s *NestedContextSuite) TestConfigMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing Logger not valid")
}

func (s *NestedContextSuite) TestConfigMissingSetupLogging(c *tc.C) {
	s.config.SetupLogging = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing SetupLogging not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitEngineConfig(c *tc.C) {
	s.config.UnitEngineConfig = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing UnitEngineConfig not valid")
}

func (s *NestedContextSuite) TestConfigMissingUnitManifolds(c *tc.C) {
	s.config.UnitManifolds = nil
	err := s.config.Validate()
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
	c.Assert(err.Error(), tc.Equals, "missing UnitManifolds not valid")
}

func (s *NestedContextSuite) newContext(c *tc.C) deployer.Context {
	context, err := deployer.NewNestedContext(s.config)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.CleanKill(c, context) })
	s.InitializeCurrentToolsDir(c, s.agent.DataDir())
	return context
}

func (s *NestedContextSuite) TestContextStops(c *tc.C) {
	// Create a context and make sure the clean kill is good.
	ctx := s.newContext(c)
	report := ctx.Report()
	c.Assert(report, tc.DeepEquals, map[string]interface{}{
		"deployed": []string{},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{},
		},
	})
}

func (s *NestedContextSuite) TestDeployUnit(c *tc.C) {
	ctx := s.newContext(c)
	unitName := "something/0"
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, tc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Unit agent dir exists.
	unitConfig := agent.ConfigPath(s.agent.DataDir(), names.NewUnitTag(unitName))
	c.Assert(unitConfig, tc.IsNonEmptyFile)

	// Unit written into the config value as deployed units.
	c.Assert(s.agent.CurrentConfig().Value("deployed-units"), tc.Equals, unitName)
	c.Assert(s.agent.CurrentConfig().AgentLogfileMaxBackups(), tc.Equals, 7)
	c.Assert(s.agent.CurrentConfig().AgentLogfileMaxSizeMB(), tc.Equals, 123)
}

func (s *NestedContextSuite) TestRecallUnit(c *tc.C) {
	unitName := "something/0"
	tag := names.NewUnitTag(unitName)
	s.config.RebootMonitorStatePurger = &fakeRebootMonitor{c: c, tag: tag}
	ctx := s.newContext(c)
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, tc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Waiting for the unit to be indicated as started (above) is not sufficient
	// for this test.
	// The unitWorkersStub that represents the nested config for the unit
	// dependency engine indicates that the unit is started as soon as it is
	// created, but the introspection socket is created subsequently, which can
	// inhibit removal of the directory during the subsequent call to RecallUnit.
	// Waiting for the socket file to be present on disk is more robust.
	socketPath := path.Join(agent.Dir(s.agent.DataDir(), tag), addons.IntrospectionSocketName)
	err = waitForFile(socketPath)
	c.Assert(err, tc.ErrorIsNil)

	err = ctx.RecallUnit(unitName)
	c.Assert(err, tc.ErrorIsNil)

	// Unit agent dir no longer exists.
	c.Assert(agent.Dir(s.agent.DataDir(), tag), tc.DoesNotExist)

	// Unit written into the config value as deployed units.
	c.Assert(s.agent.CurrentConfig().Value("deployed-units"), tc.HasLen, 0)

	// Recall is idempotent.
	err = ctx.RecallUnit(unitName)
	c.Assert(err, tc.ErrorIsNil)
}

func waitForFile(filePath string) error {
	maxAttempts := 10
	pollInterval := 50 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		if _, err := os.Stat(filePath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		time.Sleep(pollInterval)
	}

	return errors.New("file not found after 10 attempts")
}

func (s *NestedContextSuite) TestErrTerminateAgentFromAgentWorker(c *tc.C) {
	_ = s.errTerminateAgentFromAgentWorker(c)
}

func (s *NestedContextSuite) errTerminateAgentFromAgentWorker(c *tc.C) deployer.Context {
	s.workers.workerError = jworker.ErrTerminateAgent
	ctx := s.newContext(c)
	unitName := "something/0"
	err := ctx.DeployUnit(unitName, "password")
	c.Assert(err, tc.ErrorIsNil)

	// Wait for unit to start.
	s.workers.waitForStart(c, unitName)

	// Unit is marked as stopped. There is a potential race due to the
	// number of goroutines that need to fire to get the information back
	// to the nested context.
	report := s.waitForStoppedCount(c, ctx, 1)

	c.Assert(report, tc.DeepEquals, map[string]interface{}{
		"deployed": []string{"something/0"},
		"stopped":  []string{"something/0"},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{},
		},
	})
	return ctx
}

func (s *NestedContextSuite) waitForStoppedCount(c *tc.C, ctx deployer.Context, length int) map[string]interface{} {
	report := ctx.Report()
	maxTime := time.After(testing.LongWait)
	for {
		stopped := report["stopped"]
		if stopped != nil && len(stopped.([]string)) == length {
			break
		}
		select {
		case <-time.After(veryShortWait):
			report = ctx.Report()
		case <-maxTime:
			c.Fatal("unit not stopped")
		}
	}
	return report
}

func (s *NestedContextSuite) TestStopStartUnits(c *tc.C) {
	ctx := s.newContext(c)
	s.deployThreeUnits(c, ctx)

	handledBothCalls := make(chan struct{})
	count := 0
	unsub := s.hub.Subscribe(message.StopUnitResponseTopic, func(_ string, data interface{}) {
		c.Check(data, tc.DeepEquals, message.StartStopResponse{
			"first/0":   "stopped",
			"second/0":  "stopped",
			"unknown/2": `unit "unknown/2" not found`,
		})
		count++
		if count == 2 {
			close(handledBothCalls)
		}
	})

	done := s.hub.Publish(message.StopUnitTopic, message.Units{
		Names: []string{"first/0", "second/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	// Call the stop topic again, and the results are the same.
	done = s.hub.Publish(message.StopUnitTopic, message.Units{
		Names: []string{"first/0", "second/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	s.waitForEventHandled(c, handledBothCalls)
	unsub()

	report := ctx.Report()
	c.Assert(report["stopped"], tc.DeepEquals, []string{"first/0", "second/0"})

	handledBothCalls = make(chan struct{})
	count = 0
	unsub = s.hub.Subscribe(message.StartUnitResponseTopic, func(_ string, data interface{}) {
		c.Check(data, tc.DeepEquals, message.StartStopResponse{
			"first/0":   "started",
			"unknown/2": `unit "unknown/2" not found`,
		})
		count++
		if count == 2 {
			close(handledBothCalls)
		}
	})

	// Start one back up again.
	done = s.hub.Publish(message.StartUnitTopic, message.Units{
		Names: []string{"first/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	// Called again gets the same results.
	done = s.hub.Publish(message.StartUnitTopic, message.Units{
		Names: []string{"first/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	s.waitForEventHandled(c, handledBothCalls)
	unsub()

	report = ctx.Report()
	c.Assert(report["stopped"], tc.DeepEquals, []string{"second/0"})
}

func (s *NestedContextSuite) TestStartUnitAgent(c *tc.C) {
	ctx := s.errTerminateAgentFromAgentWorker(c)
	s.workers.workerError = nil

	handledBothCalls := make(chan struct{})
	count := 0
	unsub := s.hub.Subscribe(message.StartUnitResponseTopic, func(_ string, data interface{}) {
		c.Check(data, tc.DeepEquals, message.StartStopResponse{
			"something/0": "started",
			"unknown/2":   `unit "unknown/2" not found`,
		})
		count++
		if count == 2 {
			close(handledBothCalls)
		}
	})

	// Start one back up again.
	done := s.hub.Publish(message.StartUnitTopic, message.Units{
		Names: []string{"something/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	// Wait for unit to start.
	s.workers.waitForStart(c, "something/0")

	// Called again gets the same results.
	done = s.hub.Publish(message.StartUnitTopic, message.Units{
		Names: []string{"something/0", "unknown/2"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))
	s.waitForEventHandled(c, handledBothCalls)
	unsub()

	report := ctx.Report()
	c.Assert(report["stopped"], tc.IsNil)
}

func (s *NestedContextSuite) TestUnitStatus(c *tc.C) {
	responseHandled := make(chan struct{})
	unsub := s.hub.Subscribe(message.UnitStatusResponseTopic, func(_ string, payload interface{}) {
		response := payload.(message.Status) // TODO rename to unit status
		c.Check(response, tc.DeepEquals, message.Status{
			"agent": "machine-42",
			"units": map[string]string{
				"first/0":  "running",
				"second/0": "stopped",
				"third/0":  "running",
			},
		})
		close(responseHandled)
	})
	defer unsub()

	ctx := s.newContext(c)
	s.deployThreeUnits(c, ctx)
	// And stop one.
	done := s.hub.Publish(message.StopUnitTopic, message.Units{
		Names: []string{"second/0"},
	})
	s.waitForEventHandled(c, pubsub.Wait(done))

	done = s.hub.Publish(message.UnitStatusTopic, nil)
	s.waitForEventHandled(c, pubsub.Wait(done))
	s.waitForEventHandled(c, responseHandled)
}

func (s *NestedContextSuite) waitForEventHandled(c *tc.C, handled <-chan struct{}) {
	select {
	case <-handled:
		// All good.
	case <-time.After(testing.LongWait):
		c.Fatalf("event not handled")
	}
}

func (s *NestedContextSuite) deployThreeUnits(c *tc.C, ctx deployer.Context) {
	// Units are conveniently in alphabetical order.
	for _, unitName := range []string{"first/0", "second/0", "third/0"} {
		err := ctx.DeployUnit(unitName, "password")
		c.Assert(err, tc.ErrorIsNil)
		// Wait for unit to start.
		s.workers.waitForStart(c, unitName)
	}

	report := ctx.Report()
	// There is a race condition here between the worker, which says the
	// start function was called, and the engine report itself having recorded
	// that the worker has started, and updated the engine report. In manual
	// testing locally it passed 30 odd times before failing, but on slower
	// machines it may well be more frequent, so have a loop here to test.
	maxTime := time.After(testing.LongWait)
	for {
		units := report["units"].(map[string]interface{})
		workers := units["workers"].(map[string]interface{})

		first := workers["first/0"].(map[string]interface{})
		second := workers["second/0"].(map[string]interface{})
		third := workers["third/0"].(map[string]interface{})

		if first["state"] == "started" && second["state"] == "started" && third["state"] == "started" {
			break
		}
		select {
		case <-time.After(veryShortWait):
			report = ctx.Report()
		case <-maxTime:
			c.Fatal("third unit worker did not start")
		}
	}
}

func (s *NestedContextSuite) TestReport(c *tc.C) {
	ctx := s.newContext(c)
	s.deployThreeUnits(c, ctx)

	check := tc.NewMultiChecker()
	check.AddExpr(`_["units"][_][_][_][_][_]["started"]`, tc.Ignore)
	check.AddExpr(`_["units"][_][_]["started"]`, tc.Ignore)
	// Dates are shown here as an example, but are ignored by the checker.
	c.Assert(ctx.Report(), check, map[string]interface{}{
		"deployed": []string{"first/0", "second/0", "third/0"},
		"units": map[string]interface{}{
			"workers": map[string]interface{}{
				"first/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
				"second/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
				"third/0": map[string]interface{}{
					"report": map[string]interface{}{
						"manifolds": map[string]interface{}{
							"worker": map[string]interface{}{
								"inputs":      []string{},
								"start-count": 1,
								"started":     "2020-07-24 03:01:20",
								"state":       "started",
							},
						},
						"state": "started",
					},
					"started": "2020-07-24 03:01:20",
					"state":   "started",
				},
			},
		},
	})

}

type fakeRebootMonitor struct {
	c   *tc.C
	tag names.UnitTag
}

func (m *fakeRebootMonitor) PurgeState(tag names.Tag) error {
	m.c.Assert(tag.String(), tc.Equals, m.tag.String())
	return nil
}
