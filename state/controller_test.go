// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type ControllerSuite struct {
	ConnSuite
}

func TestControllerSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ControllerSuite{})
}

func (s *ControllerSuite) TestControllerAndModelConfigInitialisation(c *tc.C) {
	// Test setup has created model using a fully populated environs.Config.
	// This test ensure that the controller specific attributes have been separated out.
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, tc.ErrorIsNil)

	optional := set.NewStrings(
		controller.AgentRateLimitMax,
		controller.AgentRateLimitRate,
		controller.AllowModelAccessKey,
		controller.APIPortOpenDelay,
		controller.AuditLogExcludeMethods,
		controller.AutocertURLKey,
		controller.AutocertDNSNameKey,
		controller.CAASImageRepo,
		controller.CAASOperatorImagePath,
		controller.ControllerAPIPort,
		controller.ControllerName,
		controller.Features,
		controller.IdentityURL,
		controller.IdentityPublicKey,
		controller.LoginTokenRefreshURL,
		controller.JujuDBSnapChannel,
		controller.JujuHASpace,
		controller.JujuManagementSpace,
		controller.MaxDebugLogDuration,
		controller.MaxPruneTxnBatchSize,
		controller.MaxPruneTxnPasses,
		controller.MeteringURL,
		controller.ModelLogfileMaxBackups,
		controller.ModelLogfileMaxSize,
		controller.MongoMemoryProfile,
		controller.PruneTxnQueryCount,
		controller.PruneTxnSleepTime,
		controller.PublicDNSAddress,
		controller.MaxCharmStateSize,
		controller.MaxAgentStateSize,
		controller.MigrationMinionWaitMax,
		controller.AgentLogfileMaxBackups,
		controller.AgentLogfileMaxSize,
		controller.ControllerResourceDownloadLimit,
		controller.ApplicationResourceDownloadLimit,
		controller.QueryTracingEnabled,
		controller.QueryTracingThreshold,
		controller.JujudControllerSnapSource,
		controller.SSHMaxConcurrentConnections,
		controller.SSHServerPort,
	)
	for _, controllerAttr := range controller.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		c.Logf(controllerAttr)
		if !optional.Contains(controllerAttr) {
			c.Check(ok, tc.IsTrue)
			c.Check(v, tc.Not(tc.Equals), "")
		}
	}
}

func (s *ControllerSuite) TestNewState(c *tc.C) {
	st, err := s.Controller.GetState(s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()
	c.Check(st.ModelUUID(), tc.Equals, s.State.ModelUUID())
	c.Check(st, tc.Not(tc.Equals), s.State)
}

func (s *ControllerSuite) TestControllerConfig(c *tc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], tc.Equals, s.State.ControllerUUID())
}

func (s *ControllerSuite) TestPing(c *tc.C) {
	c.Assert(s.Controller.Ping(), tc.IsNil)
	mgotesting.MgoServer.Restart()
	c.Assert(s.Controller.Ping(), tc.NotNil)
}

func (s *ControllerSuite) TestUpdateControllerConfig(c *tc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	// Sanity check.
	c.Check(cfg.AuditingEnabled(), tc.Equals, false)
	c.Check(cfg.AuditLogCaptureArgs(), tc.Equals, true)
	c.Assert(cfg.PublicDNSAddress(), tc.Equals, "")
	c.Assert(cfg.SSHServerPort(), tc.Equals, 17022)
	c.Assert(cfg.SSHMaxConcurrentConnections(), tc.Equals, 100)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditingEnabled:             true,
		controller.AuditLogCaptureArgs:         false,
		controller.AuditLogMaxBackups:          "10",
		controller.PublicDNSAddress:            "controller.test.com:1234",
		controller.APIPortOpenDelay:            "100ms",
		controller.SSHMaxConcurrentConnections: 1025,
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	newCfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(newCfg.AuditingEnabled(), tc.Equals, true)
	c.Assert(newCfg.AuditLogCaptureArgs(), tc.Equals, false)
	c.Assert(newCfg.AuditLogMaxBackups(), tc.Equals, 10)
	c.Assert(newCfg.PublicDNSAddress(), tc.Equals, "controller.test.com:1234")
	c.Assert(newCfg.APIPortOpenDelay(), tc.Equals, 100*time.Millisecond)
	c.Assert(newCfg.SSHMaxConcurrentConnections(), tc.Equals, 1025)
}

func (s *ControllerSuite) TestUpdateControllerConfigRemoveYieldsDefaults(c *tc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditingEnabled:     true,
		controller.AuditLogCaptureArgs: true,
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(nil, []string{
		controller.AuditLogCaptureArgs,
	})
	c.Assert(err, tc.ErrorIsNil)

	newCfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(newCfg.AuditLogCaptureArgs(), tc.Equals, false)
}

func (s *ControllerSuite) TestUpdateControllerConfigRejectsDisallowedUpdates(c *tc.C) {
	// Sanity check.
	c.Assert(controller.AllowedUpdateConfigAttributes.Contains(controller.APIPort), tc.IsFalse)

	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.APIPort: 1234,
	}, nil)
	c.Assert(err, tc.ErrorMatches, `can't change "api-port" after bootstrap`)

	err = s.State.UpdateControllerConfig(nil, []string{controller.APIPort})
	c.Assert(err, tc.ErrorMatches, `can't change "api-port" after bootstrap`)
}

func (s *ControllerSuite) TestUpdateControllerConfigChecksSchema(c *tc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditLogExcludeMethods: []int{1, 2, 3},
	}, nil)
	c.Assert(err, tc.ErrorMatches, `audit-log-exclude-methods\[0\]: expected string, got int\(1\)`)
}

func (s *ControllerSuite) TestUpdateControllerConfigValidates(c *tc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditLogExcludeMethods: []string{"thing"},
	}, nil)
	c.Assert(err, tc.ErrorMatches, `invalid audit log exclude methods: should be a list of "Facade.Method" names \(or "ReadOnlyMethods"\), got "thing" at position 1`)
}

func (s *ControllerSuite) TestUpdatingUnknownName(c *tc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		"ana-ng": "majestic",
	}, nil)
	c.Assert(err, tc.ErrorMatches, `unknown controller config setting "ana-ng"`)
}

func (s *ControllerSuite) TestRemovingUnknownName(c *tc.C) {
	err := s.State.UpdateControllerConfig(nil, []string{"dr-worm"})
	c.Assert(err, tc.ErrorMatches, `unknown controller config setting "dr-worm"`)
}

func (s *ControllerSuite) TestUpdateControllerConfigAcceptEmptyStringSpace(c *tc.C) {
	sp, err := s.State.AddSpace("ha-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	addr := network.NewSpaceAddress("192.168.9.9")
	addr.SpaceID = sp.Id()

	c.Assert(m.SetProviderAddresses(addr), tc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuHASpace: "ha-space",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuHASpace: "",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestUpdateControllerConfigRejectsSpaceWithoutAddresses(c *tc.C) {
	_, err := s.State.AddSpace("mgmt-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.SetMachineAddresses(network.NewSpaceAddress("192.168.9.9")), tc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, tc.ErrorMatches,
		`invalid config "juju-mgmt-space"="mgmt-space": machines with no addresses in this space: 0`)
}

func (s *ControllerSuite) TestUpdateControllerConfigAcceptsSpaceWithAddresses(c *tc.C) {
	sp, err := s.State.AddSpace("mgmt-space", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	addr := network.NewSpaceAddress("192.168.9.9")
	addr.SpaceID = sp.Id()

	c.Assert(m.SetProviderAddresses(addr), tc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerInfo(c *tc.C) {
	info, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.CloudName, tc.Equals, "dummy")
	c.Assert(info.ModelTag, tc.Equals, s.modelTag)
	c.Assert(info.ControllerIds, tc.HasLen, 0)

	node, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	info, err = s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.ControllerIds, tc.DeepEquals, []string{node.Id()})
}

func (s *ControllerSuite) TestSetMachineAddressesControllerCharm(c *tc.C) {
	controller, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	worker, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	controllerApp := s.AddTestingApplication(c, "controller", s.AddTestingCharm(c, "juju-controller"))
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: controllerApp,
		Machine:     controller,
	})

	addresses := network.NewSpaceAddresses("10.0.0.1")
	err = controller.SetMachineAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)

	// Updating a worker machine does not affect charm config.
	addresses = network.NewSpaceAddresses("10.0.0.2")
	err = worker.SetMachineAddresses(addresses...)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := controllerApp.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg["controller-url"], tc.Equals, "wss://10.0.0.1:17777/api")
}

func (s *ControllerSuite) testOpenParams() state.OpenParams {
	return state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.modelTag,
		MongoSession:       s.Session,
	}
}

func (s *ControllerSuite) TestReopenWithNoMachines(c *tc.C) {
	expected := &state.ControllerInfo{
		CloudName: "dummy",
		ModelTag:  s.modelTag,
	}
	info, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)

	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, tc.ErrorIsNil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)

	info, err = st.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)
}

func (s *ControllerSuite) TestStateServingInfo(c *tc.C) {
	_, err := s.State.StateServingInfo()
	c.Assert(err, tc.ErrorMatches, "state serving info not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	data := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	err = s.State.SetStateServingInfo(data)
	c.Assert(err, tc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, data)
}

var setStateServingInfoWithInvalidInfoTests = []func(info *controller.StateServingInfo){
	func(info *controller.StateServingInfo) { info.APIPort = 0 },
	func(info *controller.StateServingInfo) { info.StatePort = 0 },
	func(info *controller.StateServingInfo) { info.Cert = "" },
	func(info *controller.StateServingInfo) { info.PrivateKey = "" },
}

func (s *ControllerSuite) TestSetStateServingInfoWithInvalidInfo(c *tc.C) {
	origData := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	for i, test := range setStateServingInfoWithInvalidInfoTests {
		c.Logf("test %d", i)
		data := origData
		test(&data)
		err := s.State.SetStateServingInfo(data)
		c.Assert(err, tc.ErrorMatches, "incomplete state serving info set in state")
	}
}

// SSHServerHostKey is set on state initialisation, whether that is generated
// or passed in by bootstrap --config params. So we're just testing it is
// retrievable as it will always be set.
func (s *ControllerSuite) TestSSHServerHostKey(c *tc.C) {
	key, err := s.State.SSHServerHostKey()
	c.Assert(err, tc.IsNil)

	c.Assert(key, tc.Equals, testing.SSHServerHostKey)
}
