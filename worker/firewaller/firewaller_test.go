// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"reflect"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodelrelations"
	apifirewaller "github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/firewaller"
)

// firewallerBaseSuite implements common functionality for embedding
// into each of the other per-mode suites.
type firewallerBaseSuite struct {
	jujutesting.JujuConnSuite
	testing.OsEnvSuite
	op                 <-chan dummy.Operation
	charm              *state.Charm
	controllerMachine  *state.Machine
	controllerPassword string

	st                   api.Connection
	firewaller           *apifirewaller.State
	remoteRelations      *remoterelations.Client
	crossmodelFirewaller *crossmodelrelations.Client
	mockClock            *mockClock
}

func (s *firewallerBaseSuite) SetUpSuite(c *gc.C) {
	s.OsEnvSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *firewallerBaseSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
	s.OsEnvSuite.TearDownSuite(c)
}

func (s *firewallerBaseSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.OsEnvSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *firewallerBaseSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
	s.OsEnvSuite.TearDownTest(c)
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

func (s *firewallerBaseSuite) setUpTest(c *gc.C, firewallMode string) {
	add := map[string]interface{}{"firewall-mode": firewallMode}
	s.DummyConfig = dummy.SampleConfig().Merge(add).Delete("admin-secret")

	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")

	// Create a manager machine and login to the API.
	var err error
	s.controllerMachine, err = s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	s.controllerPassword, err = utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.controllerMachine.SetPassword(s.controllerPassword)
	c.Assert(err, jc.ErrorIsNil)
	err = s.controllerMachine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.controllerMachine.Tag(), s.controllerPassword, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the API facades.
	s.firewaller = apifirewaller.NewState(s.st)
	c.Assert(s.firewaller, gc.NotNil)
	s.remoteRelations = remoterelations.NewClient(s.st)
	c.Assert(s.remoteRelations, gc.NotNil)
}

// assertPorts retrieves the open ports of the instance and compares them
// to the expected.
func (s *firewallerBaseSuite) assertPorts(c *gc.C, inst instance.Instance, machineId string, expected []network.IngressRule) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := inst.IngressRules(machineId)
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortIngressRules(got)
		network.SortIngressRules(expected)
		if reflect.DeepEqual(got, expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertEnvironPorts retrieves the open ports of environment and compares them
// to the expected.
func (s *firewallerBaseSuite) assertEnvironPorts(c *gc.C, expected []network.IngressRule) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := s.Environ.IngressRules()
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortIngressRules(got)
		network.SortIngressRules(expected)
		if reflect.DeepEqual(got, expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(coretesting.ShortWait)
	}
}

func (s *firewallerBaseSuite) addUnit(c *gc.C, app *state.Application) (*state.Unit, *state.Machine) {
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	return u, m
}

// startInstance starts a new instance for the given machine.
func (s *firewallerBaseSuite) startInstance(c *gc.C, m *state.Machine) instance.Instance {
	inst, hc := jujutesting.AssertStartInstance(c, s.Environ, s.ControllerConfig.ControllerUUID(), m.Id())
	err := m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	return inst
}

type InstanceModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&InstanceModeSuite{})

func (s *InstanceModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwInstance)
}

func (s *InstanceModeSuite) TearDownTest(c *gc.C) {
	s.firewallerBaseSuite.JujuConnSuite.TearDownTest(c)
}

// mockClock will panic if anything but After is called
type mockClock struct {
	clock.Clock
	wait time.Duration
	c    *gc.C
}

func (m *mockClock) After(duration time.Duration) <-chan time.Time {
	m.wait = duration
	return time.After(time.Millisecond)
}

func (s *InstanceModeSuite) newFirewaller(c *gc.C) worker.Worker {
	s.mockClock = &mockClock{c: c}
	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwInstance,
		EnvironFirewaller:  s.Environ,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(modelUUID string) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock: s.mockClock,
	}
	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *InstanceModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *InstanceModeSuite) TestNotExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err := u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)

	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	err = u.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestMultipleExposedApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "mysql", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "0.0.0.0/0"),
	})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestMachineWithoutInstanceId(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, app)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestMultipleUnits(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestStartWithState(c *gc.C) {
	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithPartialState(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Expose service.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestSetClearExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err := u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	// Not exposed service, so no open port.
	s.assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// ClearExposed closes the ports again.
	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveUnit(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestRemoveApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove application.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMultipleApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "mysql", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "0.0.0.0/0"),
	})

	// Remove applications.
	err = u2.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u2.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app2.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app1.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestDeadMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove unit and application, also tested without. Has no effect.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Kill machine.
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove unit.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Remove machine. Nothing bad should happen, but can't
	// assert port state since the machine must have been
	// destroyed and we lost its reference.
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithStateOpenPortsBroken(c *gc.C) {
	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)
	dummy.SetInstanceBroken(inst, "OpenPorts")

	// Starting the firewaller should attempt to open the ports,
	// and fail due to the method being broken.
	fw := s.newFirewaller(c)

	errc := make(chan error, 1)
	go func() { errc <- fw.Wait() }()
	s.BackingState.StartSync()
	select {
	case err := <-errc:
		c.Assert(err, gc.ErrorMatches,
			`cannot respond to units changes for "machine-1": dummyInstance.OpenPorts is broken`)
	case <-time.After(coretesting.LongWait):
		fw.Kill()
		fw.Wait()
		c.Fatal("timed out waiting for firewaller to stop")
	}
}

func (s *InstanceModeSuite) TestRemoteRelationRequirerRole(c *gc.C) {
	// Set up the consuming model - create the local app.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Set up the consuming model - create the remote app.
	offeringModelTag := names.NewModelTag(utils.MustNewUUID().String())
	appToken := utils.MustNewUUID().String()
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: offeringModelTag,
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Create the external controller info.
	ec := state.NewExternalControllers(s.State)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"1.2.3.4:1234"},
		CACert:        coretesting.CACert}, offeringModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	published := make(chan bool)
	var relToken string
	var ingressRequired bool
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishIngressNetworkChanges")
		expected := params.IngressNetworksChanges{
			Changes: []params.IngressNetworksChangeEvent{{
				RelationId:      params.RemoteEntityId{ModelUUID: s.State.ModelUUID(), Token: relToken},
				ApplicationId:   params.RemoteEntityId{ModelUUID: offeringModelTag.Id(), Token: appToken},
				Networks:        []string{"10.0.0.4/32"},
				IngressRequired: ingressRequired,
			}},
		}
		expected.Changes[0].IngressRequired = ingressRequired
		if !ingressRequired {
			expected.Changes[0].Networks = []string{}
		}
		c.Check(arg, gc.DeepEquals, expected)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		published <- true
		return nil
	})

	s.crossmodelFirewaller = crossmodelrelations.NewClient(apiCaller)
	c.Assert(s.crossmodelFirewaller, gc.NotNil)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	relToken, err = re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(offeringModelTag, app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)
	ingressRequired = true

	// We should not have published any ingress events yet - no unit has entered scope.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-published:
		c.Fatal("unexpected ingress change to be published")
	}

	// Add a public address to the consuming unit so the firewaller can use it.
	wpm := s.Factory.MakeMachine(c, &factory.MachineParams{
		Addresses: []network.Address{network.NewAddress("10.0.0.4")},
	})
	u, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(wpm)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	err = ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Check the relation ready poll time is as expected.
	c.Assert(s.mockClock.wait, gc.Equals, 3*time.Second)

	// Change should be sent when unit leaves scope.
	ingressRequired = false
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on leave scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRole(c *gc.C) {
	// Set up the offering model - create the local app.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, m := s.addUnit(c, mysql)
	inst := s.startInstance(c, m)
	err := u.OpenPort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	// Set up the offering model - create the remote app.
	consumingModelTag := names.NewModelTag(utils.MustNewUUID().String())
	relToken := utils.MustNewUUID().String()
	appToken := utils.MustNewUUID().String()
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "wordpress", SourceModel: consumingModelTag,
		Endpoints: []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Create the firewaller facade on the offering model.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	err = re.ImportRemoteEntity(consumingModelTag, rel.Tag(), relToken)
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(consumingModelTag, app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)

	// No port changes yet.
	s.assertPorts(c, inst, m.Id(), nil)

	// Save a new ingress network against the relation.
	rin := state.NewRelationIngressNetworks(s.State)
	_, err = rin.Save(rel.Tag().Id(), []string{"10.0.0.4/16"})
	c.Assert(err, jc.ErrorIsNil)

	//Ports opened.
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "10.0.0.4/16"),
	})

	// Check the relation ready poll time is as expected.
	c.Assert(s.mockClock.wait, gc.Equals, 3*time.Second)

	// Change should be sent when ingress networks disappear.
	_, err = rin.Save(rel.Tag().Id(), nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)

	_, err = rin.Save(rel.Tag().Id(), []string{"10.0.0.4/16"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "10.0.0.4/16"),
	})

	// And again when relation is destroyed.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

type GlobalModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&GlobalModeSuite{})

func (s *GlobalModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwGlobal)
}

func (s *GlobalModeSuite) TearDownTest(c *gc.C) {
	s.firewallerBaseSuite.JujuConnSuite.TearDownTest(c)
}

func (s *GlobalModeSuite) newFirewaller(c *gc.C) worker.Worker {
	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwGlobal,
		EnvironFirewaller:  s.Environ,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(modelUUID string) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
	}
	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *GlobalModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *GlobalModeSuite) TestGlobalMode(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	err = u1.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "moinmoin", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	err = u2.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
	})

	// Closing the last port also modifies the environment.
	err = u2.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)

	// Expose application.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *GlobalModeSuite) TestRestart(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	err = u.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and close one and open a different port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	err = u.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8888)
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8888, 8888, "0.0.0.0/0"),
	})
}

func (s *GlobalModeSuite) TestRestartUnexposedApplication(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and clear exposed flag on application.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestRestartPortCount(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and add another application using the port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "moinmoin", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, nil)
}

type NoneModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&NoneModeSuite{})

func (s *NoneModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwNone)
}

func (s *NoneModeSuite) TestStopImmediately(c *gc.C) {
	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwNone,
		EnvironFirewaller:  s.Environ,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(modelUUID string) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
	}
	_, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, gc.ErrorMatches, `invalid firewall-mode "none"`)
}
