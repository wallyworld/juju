// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/rpc/params"
)

type AgentConfigUpdaterSuite struct {
	testing.BaseSuite
	manifold dependency.Manifold
	hub      *pubsub.StructuredHub
}

func TestAgentConfigUpdaterSuite(t *tctesting.T) {
	tc.Run(t, &AgentConfigUpdaterSuite{})
}

func (s *AgentConfigUpdaterSuite) SetUpTest(c *tc.C) {
	logger := loggo.GetLogger("test")
	s.manifold = agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
		AgentName:      "agent",
		APICallerName:  "api-caller",
		CentralHubName: "central-hub",
		Logger:         logger,
	})
	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Logger: logger,
	})
}

func (s *AgentConfigUpdaterSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{
		"agent",
		"api-caller",
		"central-hub",
	})
}

func (s *AgentConfigUpdaterSuite) TestStartAgentMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestStartAPICallerMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":      &mockAgent{},
		"api-caller": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestNotMachine(c *tc.C) {
	a := &mockAgent{
		conf: mockConfig{tag: names.NewUnitTag("foo/0")},
	}
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": a,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "agent's tag is not a machine or controller agent tag")
}

func (s *AgentConfigUpdaterSuite) TestEntityLookupFailure(c *tc.C) {
	// Set up a fake Agent and APICaller
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Error: &params.Error{Message: "boom"},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	// Call the manifold's start func with a fake resource getter that
	// returns the fake Agent and APICaller
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
	})
	w, err := s.manifold.Start(context)
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "checking controller status: boom")
}

func (s *AgentConfigUpdaterSuite) TestCentralHubMissing(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    1234,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{
						"mongo-memory-profile":    "default",
						"juju-db-snap-channel":    controller.DefaultJujuDBSnapChannel,
						"query-tracing-enabled":   controller.DefaultQueryTracingEnabled,
						"query-tracing-threshold": controller.DefaultQueryTracingThreshold,
					},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &mockAgent{},
		"api-caller":  apiCaller,
		"central-hub": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *AgentConfigUpdaterSuite) TestCentralHubMissingFirstPass(c *tc.C) {
	agent := &mockAgent{}
	agent.conf.profile = "not-set"
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    1234,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{
						"mongo-memory-profile": "default",
					},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       agent,
		"api-caller":  apiCaller,
		"central-hub": dependency.ErrMissing,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, jworker.ErrRestartAgent)
}

func (s *AgentConfigUpdaterSuite) startManifold(c *tc.C, a agent.Agent, mockAPIPort int) (worker.Worker, error) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{model.JobManageModel},
				}}
			case "StateServingInfo":
				result := response.(*params.StateServingInfo)
				*result = params.StateServingInfo{
					Cert:       "cert",
					PrivateKey: "key",
					APIPort:    mockAPIPort,
				}
			case "ControllerConfig":
				result := response.(*params.ControllerConfigResult)
				*result = params.ControllerConfigResult{
					Config: map[string]interface{}{
						"mongo-memory-profile":    "default",
						"juju-db-snap-channel":    controller.DefaultJujuDBSnapChannel,
						"query-tracing-enabled":   controller.DefaultQueryTracingEnabled,
						"query-tracing-threshold": controller.DefaultQueryTracingThreshold,
					},
				}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
	})
	return s.manifold.Start(context)
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnviron(c *tc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	c.Assert(a.conf.profileSet, tc.IsFalse)
	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, tc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, tc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, tc.Equals, "cert")
	c.Assert(a.conf.ssi.PrivateKey, tc.Equals, "key")
}

func (s *AgentConfigUpdaterSuite) TestProfileDifferenceRestarts(c *tc.C) {
	const mockAPIPort = 1234

	a := &mockAgent{}
	a.conf.profile = "other"
	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.Equals, jworker.ErrRestartAgent)

	c.Assert(a.conf.profileSet, tc.IsTrue)
}

func (s *AgentConfigUpdaterSuite) TestJobManageEnvironNotOverwriteCert(c *tc.C) {
	// State serving info should be set for machines with JobManageEnviron.
	const mockAPIPort = 1234

	a := &mockAgent{}
	existingCert := "some cert set by certupdater"
	existingKey := "some key set by certupdater"
	a.conf.SetStateServingInfo(controller.StateServingInfo{
		Cert:       existingCert,
		PrivateKey: existingKey,
	})

	w, err := s.startManifold(c, a, mockAPIPort)
	c.Assert(w, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	// Verify that the state serving info was actually set.
	c.Assert(a.conf.ssiSet, tc.IsTrue)
	c.Assert(a.conf.ssi.APIPort, tc.Equals, mockAPIPort)
	c.Assert(a.conf.ssi.Cert, tc.Equals, existingCert)
	c.Assert(a.conf.ssi.PrivateKey, tc.Equals, existingKey)
}

func (s *AgentConfigUpdaterSuite) TestJobHostUnits(c *tc.C) {
	// State serving info should not be set for JobHostUnits.
	s.checkNotController(c, model.JobHostUnits)
}

func (s *AgentConfigUpdaterSuite) checkNotController(c *tc.C, job model.MachineJob) {
	a := &mockAgent{}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, response interface{}) error {
			c.Assert(objType, tc.Equals, "Agent")
			switch request {
			case "GetEntities":
				c.Assert(args.(params.Entities).Entities, tc.HasLen, 1)
				result := response.(*params.AgentGetEntitiesResults)
				result.Entities = []params.AgentGetEntitiesResult{{
					Jobs: []model.MachineJob{job},
				}}
			default:
				c.Fatalf("not sure how to handle: %q", request)
			}
			return nil
		},
	)
	w, err := s.manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":       a,
		"api-caller":  apiCaller,
		"central-hub": s.hub,
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.Equals, dependency.ErrUninstall)

	// State serving info shouldn't have been set for this job type.
	c.Assert(a.conf.ssiSet, tc.IsFalse)
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag    names.Tag
	ssiSet bool
	ssi    controller.StateServingInfo

	profile    string
	profileSet bool

	snapChannel    string
	snapChannelSet bool

	queryTracingEnabled    bool
	queryTracingEnabledSet bool

	queryTracingThreshold    time.Duration
	queryTracingThresholdSet bool
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) MongoMemoryProfile() mongo.MemoryProfile {
	if mc.profile == "" {
		return controller.DefaultMongoMemoryProfile
	}
	return mongo.MemoryProfile(mc.profile)
}

func (mc *mockConfig) SetMongoMemoryProfile(profile mongo.MemoryProfile) {
	mc.profile = string(profile)
	mc.profileSet = true
}

func (mc *mockConfig) JujuDBSnapChannel() string {
	if mc.snapChannel == "" {
		return controller.DefaultJujuDBSnapChannel
	}
	return mc.snapChannel
}

func (mc *mockConfig) SetJujuDBSnapChannel(snapChannel string) {
	mc.snapChannel = snapChannel
	mc.snapChannelSet = true
}

func (mc *mockConfig) QueryTracingEnabled() bool {
	return mc.queryTracingEnabled
}

func (mc *mockConfig) SetQueryTracingEnabled(enabled bool) {
	mc.queryTracingEnabled = enabled
	mc.queryTracingEnabledSet = true
}

func (mc *mockConfig) QueryTracingThreshold() time.Duration {
	if mc.queryTracingThreshold == 0 {
		return controller.DefaultQueryTracingThreshold
	}
	return mc.queryTracingThreshold
}

func (mc *mockConfig) SetQueryTracingThreshold(threshold time.Duration) {
	mc.queryTracingThreshold = threshold
	mc.queryTracingThresholdSet = true
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}
