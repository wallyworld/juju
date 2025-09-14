// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	"regexp"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	unitassignerapi "github.com/juju/juju/api/agent/unitassigner"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/application"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/testcharms"
)

type applicationSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	applicationAPI *application.APIBase
	application    *state.Application
	authorizer     *apiservertesting.FakeAuthorizer
	lastKnownRev   map[string]int
}

func TestApplicationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &applicationSuite{})
}

func (s *applicationSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*tc.C) { s.BlockHelper.Close() })

	s.application = s.Factory.MakeApplication(c, nil)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.applicationAPI = s.makeAPI(c)
	s.lastKnownRev = make(map[string]int)
}

func (s *applicationSuite) makeAPI(c *tc.C) *application.APIBase {
	resources := common.NewResources()
	c.Assert(resources.RegisterNamed("dataDir", common.StringResource(c.MkDir())), tc.ErrorIsNil)
	storageAccess, err := application.GetStorageState(s.State)
	c.Assert(err, tc.ErrorIsNil)
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(s.State)
	registry := stateenvirons.NewStorageProviderRegistry(s.Environ)
	pm := poolmanager.New(state.NewStateSettings(s.State), registry)
	api, err := application.NewAPIBase(
		application.GetState(s.State),
		storageAccess,
		s.authorizer,
		nil,
		nil,
		blockChecker,
		application.GetModel(model),
		nil, // leadership not used in these tests.
		application.CharmToStateCharm,
		application.DeployApplication,
		pm,
		registry,
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
		nil, // secret backend config getter not used in this suite.
		state.NewSecrets(s.State),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *applicationSuite) setupApplicationDeploy(c *tc.C, args string) (string, charm.Charm, constraints.Value) {
	curl, ch := s.addCharmToState(c, "ch:jammy/dummy-42", "dummy")
	cons := constraints.MustParse(args)
	return curl, ch, cons
}

func (s *applicationSuite) assertApplicationDeployPrincipal(c *tc.C, curl string, ch charm.Charm, mem4g constraints.Value) {
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, mem4g)
}

func (s *applicationSuite) assertApplicationDeployPrincipalBlocked(c *tc.C, msg string, curl string, mem4g constraints.Value) {
	_, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyApplicationDeployPrincipal(c *tc.C) {
	curl, bundle, cons := s.setupApplicationDeploy(c, "arch=amd64 mem=4G")
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockRemoveApplicationDeployPrincipal(c *tc.C) {
	curl, bundle, cons := s.setupApplicationDeploy(c, "arch=amd64 mem=4G")
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockChangesApplicationDeployPrincipal(c *tc.C) {
	curl, _, cons := s.setupApplicationDeploy(c, "mem=4G")
	s.BlockAllChanges(c, "TestBlockChangesApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipalBlocked(c, "TestBlockChangesApplicationDeployPrincipal", curl, cons)
}

func (s *applicationSuite) TestApplicationDeploySubordinate(c *tc.C) {
	curl, ch := s.addCharmToState(c, "ch:utopic/logging-47", "logging")
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	app, err := s.State.Application("application-name")
	c.Assert(err, tc.ErrorIsNil)
	charm, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsFalse)
	c.Assert(charm.URL(), tc.DeepEquals, curl)
	c.Assert(charm.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), tc.DeepEquals, ch.Config())

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 0)
}

func (s *applicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *applicationSuite) TestApplicationDeployConfig(c *tc.C) {
	curl, _ := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	app, err := s.State.Application("application-name")
	c.Assert(err, tc.ErrorIsNil)
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, s.combinedSettings(ch, charm.Settings{"username": "fred"}))
}

func (s *applicationSuite) TestApplicationDeployConfigError(c *tc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  skill-level: fred",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Application("application-name")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *applicationSuite) TestApplicationDeployToMachine(c *tc.C) {
	curl, ch := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")

	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	app, err := s.State.Application("application-name")
	c.Assert(err, tc.ErrorIsNil)
	charm, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsFalse)
	c.Assert(charm.URL(), tc.DeepEquals, curl)
	c.Assert(charm.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), tc.DeepEquals, ch.Config())

	errs, err := unitassignerapi.New(s.APIState).AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, tc.DeepEquals, []error{nil})
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithLXDProfile(c *tc.C) {
	curl, ch := s.addCharmToState(c, "ch:jammy/lxd-profile-0", "lxd-profile")

	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, tc.ErrorIsNil)
	expected, force, err := application.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsFalse)
	c.Assert(expected.URL(), tc.DeepEquals, curl)
	c.Assert(expected.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), tc.DeepEquals, ch.Config())

	expectedProfile := ch.(charm.LXDProfiler).LXDProfile()
	c.Assert(expected.LXDProfile(), tc.DeepEquals, &charm.LXDProfile{
		Description: expectedProfile.Description,
		Config:      expectedProfile.Config,
		Devices:     expectedProfile.Devices,
	})

	errs, err := unitassignerapi.New(s.APIState).AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, tc.DeepEquals, []error{nil})
	c.Assert(err, tc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithInvalidLXDProfileAndForceStillSucceeds(c *tc.C) {
	curl, ch := s.addCharmToState(c, "ch:jammy/lxd-profile-fail-0", "lxd-profile-fail")

	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	app, err := s.State.Application("application-name")
	c.Assert(err, tc.ErrorIsNil)
	expected, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsFalse)
	c.Assert(expected.URL(), tc.DeepEquals, curl)
	c.Assert(expected.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), tc.DeepEquals, ch.Config())

	expectedProfile := ch.(charm.LXDProfiler).LXDProfile()
	c.Assert(expected.LXDProfile(), tc.DeepEquals, &charm.LXDProfile{
		Description: expectedProfile.Description,
		Config:      expectedProfile.Config,
		Devices:     expectedProfile.Devices,
	})

	errs, err := unitassignerapi.New(s.APIState).AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, tc.DeepEquals, []error{nil})
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineNotFound(c *tc.C) {
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        "ch:jammy/application-name-1",
			CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}},
			ApplicationName: "application-name",
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `cannot deploy "application-name" to machine 42: machine 42 not found`)

	_, err = s.State.Application("application-name")
	c.Assert(err, tc.ErrorMatches, `application "application-name" not found`)
}

func (s *applicationSuite) TestApplicationUpdateDoesNotSetMinUnitsWithLXDProfile(c *tc.C) {
	series := "quantal"
	repo := testcharms.RepoForSeries(series)
	ch := repo.CharmDir("lxd-profile-fail")
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/%s", series, ident))
	_, err := jujutesting.PutCharm(s.State, curl, ch)
	c.Assert(err, tc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

var clientAddApplicationUnitsTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	numUnits    int
	expected    []string
	to          string
	err         string
}{
	{
		about:    "returns unit names",
		numUnits: 3,
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		numUnits: 1,
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:       "unknown application name",
		application: "unknown-application",
		numUnits:    1,
		err:         `application "unknown-application" not found`,
	},
}

func (s *applicationSuite) TestClientAddApplicationUnits(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	for i, t := range clientAddApplicationUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		args := params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        t.numUnits,
		}
		if t.to != "" {
			args.Placement = []*instance.Placement{instance.MustParsePlacement(t.to)}
		}
		result, err := s.applicationAPI.AddUnits(args)
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.Units, tc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/3")
	c.Assert(err, tc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(assignedMachine, tc.Equals, "0")
}

func (s *applicationSuite) TestAddApplicationUnitsToNewContainer(c *tc.C) {
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	machine, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        1,
		Placement:       []*instance.Placement{instance.MustParsePlacement("lxd:" + machine.Id())},
	})
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mid, tc.Equals, machine.Id()+"/lxd/0")
}

var addApplicationUnitTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	expected    []string
	machineIds  []string
	placement   []*instance.Placement
	err         string
}{
	{
		about:      "valid placement directives",
		expected:   []string{"dummy/0"},
		placement:  []*instance.Placement{{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "valid"}},
		machineIds: []string{"1"},
	}, {
		about:      "direct machine assignment placement directive",
		expected:   []string{"dummy/1", "dummy/2"},
		placement:  []*instance.Placement{{Scope: "#", Directive: "1"}, {Scope: "lxd", Directive: "1"}},
		machineIds: []string{"1", "1/lxd/0"},
	}, {
		about:     "invalid placement directive",
		err:       ".* invalid placement is invalid",
		expected:  []string{"dummy/3"},
		placement: []*instance.Placement{{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "invalid"}},
	},
}

func (s *applicationSuite) TestAddApplicationUnits(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Add a machine for the units to be placed on.
	_, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	for i, t := range addApplicationUnitTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		result, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        len(t.expected),
			Placement:       t.placement,
		})
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.Units, tc.DeepEquals, t.expected)
		for i, unitName := range result.Units {
			u, err := s.BackingState.Unit(unitName)
			c.Assert(err, tc.ErrorIsNil)
			assignedMachine, err := u.AssignedMachineId()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(assignedMachine, tc.Equals, t.machineIds[i])
		}
	}
}

func (s *applicationSuite) assertAddApplicationUnits(c *tc.C) {
	result, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Units, tc.DeepEquals, []string{"dummy/0", "dummy/1", "dummy/2"})

	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/0")
	c.Assert(err, tc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(assignedMachine, tc.Equals, "0")
}

func (s *applicationSuite) TestApplicationCharmRelations(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.applicationAPI.CharmRelations(params.ApplicationCharmRelations{ApplicationName: "blah"})
	c.Assert(err, tc.ErrorMatches, `application "blah" not found`)

	result, err := s.applicationAPI.CharmRelations(params.ApplicationCharmRelations{ApplicationName: "wordpress"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.CharmRelations, tc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *applicationSuite) assertAddApplicationUnitsBlocked(c *tc.C, msg string) {
	_, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyAddApplicationUnits(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyModel(c, "TestBlockDestroyAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockRemoveAddApplicationUnits(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockChangeAddApplicationUnits(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangeAddApplicationUnits")
	s.assertAddApplicationUnitsBlocked(c, "TestBlockChangeAddApplicationUnits")
}

func (s *applicationSuite) TestAddUnitToMachineNotFound(c *tc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	_, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
		Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
	})
	c.Assert(err, tc.ErrorMatches, `acquiring machine to host unit "dummy/0": machine 42 not found`)
}

func (s *applicationSuite) TestApplicationExpose(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = s.AddTestingApplication(c, name, charm)
		c.Assert(apps[i].IsExposed(), tc.IsFalse)
	}
	err = apps[1].MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), tc.IsTrue)

	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestApplicationExposeEndpoints(c *tc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.Assert(app.IsExposed(), tc.IsFalse)

	err := s.applicationAPI.Expose(params.ApplicationExpose{
		ApplicationName: app.Name(),
		ExposedEndpoints: map[string]params.ExposedEndpoint{
			// Exposing an endpoint with no expose options implies
			// expose to 0.0.0.0/0 and ::/0.
			"monitoring-port": {},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.State.Application(app.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.IsExposed(), tc.Equals, true)
	c.Assert(got.ExposedEndpoints(), tc.DeepEquals, map[string]state.ExposedEndpoint{
		"monitoring-port": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	})
}

func (s *applicationSuite) TestApplicationExposeEndpointsWithPre29Client(c *tc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.Assert(app.IsExposed(), tc.IsFalse)

	err := s.applicationAPI.Expose(params.ApplicationExpose{
		ApplicationName: app.Name(),
		// If no endpoint-specific expose params are provided, the call
		// will emulate the behavior of a pre 2.9 controller where all
		// ports are exposed to 0.0.0.0/0 and ::/0.
	})
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.State.Application(app.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.IsExposed(), tc.Equals, true)
	c.Assert(got.ExposedEndpoints(), tc.DeepEquals, map[string]state.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	})
}

func (s *applicationSuite) setupApplicationExpose(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = s.AddTestingApplication(c, name, charm)
		c.Assert(apps[i].IsExposed(), tc.IsFalse)
	}
	err = apps[1].MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), tc.IsTrue)
}

var applicationExposeTests = []struct {
	about                 string
	application           string
	exposedEndpointParams map[string]params.ExposedEndpoint
	//
	expExposed          bool
	expExposedEndpoints map[string]state.ExposedEndpoint
	expErr              string
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		expErr:      `application "unknown-application" not found`,
	},
	{
		about:       "expose all endpoints of an application ",
		application: "dummy-application",
		expExposed:  true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		},
	},
	{
		about:       "expose an already exposed application",
		application: "exposed-application",
		expExposed:  true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		},
	},
	{
		about:       "unknown endpoint name in expose parameters",
		application: "dummy-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"bogus": {},
		},
		expErr: `endpoint "bogus" not found`,
	},
	{
		about:       "unknown space name in expose parameters",
		application: "dummy-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"": {
				ExposeToSpaces: []string{"invaders"},
			},
		},
		expErr: `space "invaders" not found`,
	},
	{
		about:       "expose an application and provide expose parameters",
		application: "exposed-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"": {
				ExposeToSpaces: []string{network.AlphaSpaceName},
				ExposeToCIDRs:  []string{"13.37.0.0/16"},
			},
		},
		expExposed: true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToSpaceIDs: []string{network.AlphaSpaceId},
				ExposeToCIDRs:    []string{"13.37.0.0/16"},
			},
		},
	},
}

func (s *applicationSuite) assertApplicationExpose(c *tc.C) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(params.ApplicationExpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.exposedEndpointParams,
		})
		if t.expErr != "" {
			c.Assert(err, tc.ErrorMatches, t.expErr)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			app, err := s.State.Application(t.application)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(app.IsExposed(), tc.Equals, t.expExposed)
			c.Assert(app.ExposedEndpoints(), tc.DeepEquals, t.expExposedEndpoints)
		}
	}
}

func (s *applicationSuite) assertApplicationExposeBlocked(c *tc.C, msg string) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(params.ApplicationExpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.exposedEndpointParams,
		})
		s.AssertBlocked(c, err, msg)
	}
}

func (s *applicationSuite) TestBlockDestroyApplicationExpose(c *tc.C) {
	s.setupApplicationExpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockRemoveApplicationExpose(c *tc.C) {
	s.setupApplicationExpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockChangesApplicationExpose(c *tc.C) {
	s.setupApplicationExpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationExpose")
	s.assertApplicationExposeBlocked(c, "TestBlockChangesApplicationExpose")
}

var applicationUnexposeTests = []struct {
	about               string
	application         string
	err                 string
	initial             map[string]state.ExposedEndpoint
	unexposeEndpoints   []string
	expExposed          bool
	expExposedEndpoints map[string]state.ExposedEndpoint
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "unexpose a application without specifying any endpoints",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"": {},
		},
		expExposed: false,
	},
	{
		about:       "unexpose specific application endpoint",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"server":       {},
			"server-admin": {},
		},
		unexposeEndpoints: []string{"server"},
		// The server-admin (and hence the app) should remain exposed
		expExposed: true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"server-admin": {ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"}},
		},
	},
	{
		about:       "unexpose all currently exposed application endpoints",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"server":       {},
			"server-admin": {},
		},
		unexposeEndpoints: []string{"server", "server-admin"},
		// Application should now be unexposed as all its endpoints have
		// been unexposed.
		expExposed: false,
	},
	{
		about:       "unexpose an already unexposed application",
		application: "dummy-application",
		initial:     nil,
		expExposed:  false,
	},
}

func (s *applicationSuite) TestApplicationUnexpose(c *tc.C) {
	charm := s.AddTestingCharm(c, "mysql")
	for i, t := range applicationUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", charm)
		if len(t.initial) != 0 {
			err := app.MergeExposeSettings(t.initial)
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Assert(app.IsExposed(), tc.Equals, len(t.initial) != 0)
		err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.unexposeEndpoints,
		})
		if t.err == "" {
			c.Assert(err, tc.ErrorIsNil)
			app.Refresh()
			c.Assert(app.IsExposed(), tc.Equals, t.expExposed)
			c.Assert(app.ExposedEndpoints(), tc.DeepEquals, t.expExposedEndpoints)
		} else {
			c.Assert(err, tc.ErrorMatches, t.err)
		}
		err = app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *applicationSuite) setupApplicationUnexpose(c *tc.C) *state.Application {
	charm := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", charm)
	app.MergeExposeSettings(nil)
	c.Assert(app.IsExposed(), tc.Equals, true)
	return app
}

func (s *applicationSuite) assertApplicationUnexpose(c *tc.C, app *state.Application) {
	err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{ApplicationName: "dummy-application"})
	c.Assert(err, tc.ErrorIsNil)
	app.Refresh()
	c.Assert(app.IsExposed(), tc.Equals, false)
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) assertApplicationUnexposeBlocked(c *tc.C, app *state.Application, msg string) {
	err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{ApplicationName: "dummy-application"})
	s.AssertBlocked(c, err, msg)
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestBlockDestroyApplicationUnexpose(c *tc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockRemoveApplicationUnexpose(c *tc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockChangesApplicationUnexpose(c *tc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationUnexpose")
	s.assertApplicationUnexposeBlocked(c, app, "TestBlockChangesApplicationUnexpose")
}

var applicationDestroyTests = []struct {
	about       string
	application string
	err         string
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "destroy an application",
		application: "dummy-application",
	},
	{
		about:       "destroy an already destroyed application",
		application: "dummy-application",
		err:         `application "dummy-application" not found`,
	},
}

func (s *applicationSuite) apiv16() *application.APIv16 {
	return &application.APIv16{
		APIv17: &application.APIv17{
			APIv18: &application.APIv18{
				APIv19: &application.APIv19{
					APIv20: &application.APIv20{
						APIv21: &application.APIv21{
							APIBase: s.applicationAPI,
						},
					},
				},
			},
		},
	}
}

func (s *applicationSuite) TestApplicationDestroy(c *tc.C) {
	apiv16 := s.apiv16()
	s.AddTestingApplication(c, "dummy-application", s.AddTestingCharm(c, "dummy"))
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-application",
		SourceModel: s.Model.ModelTag(),
		Token:       "t0",
	})
	c.Assert(err, tc.ErrorIsNil)

	for i, t := range applicationDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := apiv16.Destroy(params.ApplicationDestroy{ApplicationName: t.application})
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	// Now do Destroy on an application with units. Destroy will
	// cause the application to be not-Alive, but will not remove its
	// document.
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	applicationName := "wordpress"
	app, err := s.State.Application(applicationName)
	c.Assert(err, tc.ErrorIsNil)
	err = apiv16.Destroy(params.ApplicationDestroy{ApplicationName: applicationName})
	c.Assert(err, tc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func assertLife(c *tc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Life(), tc.Equals, life)
}

func (s *applicationSuite) TestBlockApplicationDestroy(c *tc.C) {
	apiv16 := s.apiv16()
	s.AddTestingApplication(c, "dummy-application", s.AddTestingCharm(c, "dummy"))

	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockApplicationDestroy")
	err := apiv16.Destroy(params.ApplicationDestroy{ApplicationName: "dummy-application"})
	s.AssertBlocked(c, err, "TestBlockApplicationDestroy")
	// Tests may have invalid application names.
	app, err := s.State.Application("dummy-application")
	if err == nil {
		// For valid application names, check that application is alive :-)
		assertLife(c, app, state.Alive)
	}
}

func (s *applicationSuite) TestDestroyControllerApplicationNotAllowed(c *tc.C) {
	apiv16 := s.apiv16()
	s.AddTestingApplication(c, "controller-application", s.AddTestingCharm(c, "juju-controller"))

	err := apiv16.Destroy(params.ApplicationDestroy{"controller-application"})
	c.Assert(err, tc.ErrorMatches, "removing the controller application not supported")
}

func (s *applicationSuite) TestDestroyPrincipalUnits(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		unit.AssignToNewMachine()
		c.Assert(err, tc.ErrorIsNil)
		now := time.Now()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = unit.SetAgentStatus(sInfo)
		c.Assert(err, tc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDestroyPrincipalUnits(c, units)
}

func (s *applicationSuite) TestDestroySubordinateUnits(c *tc.C) {
	apiv16 := s.apiv16()
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, tc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate, .*`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *applicationSuite) assertDestroyPrincipalUnits(c *tc.C, units []*state.Unit) {
	apiv16 := s.apiv16()
	// Destroy 2 of them; check they become Dying.
	err := apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/2", "wordpress/0"},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, units[2], state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"boojum/123", "wordpress/3"},
	})
	c.Assert(err, tc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	assertLife(c, units[3], state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, tc.ErrorIsNil)
	err = wp0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/4"},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, units[0], state.Dead)
	assertLife(c, units[4], state.Dying)
}

func (s *applicationSuite) setupDestroyPrincipalUnits(c *tc.C) []*state.Unit {
	units := make([]*state.Unit, 5)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for i := range units {
		unit, err := wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = unit.AssignToNewMachine()
		c.Assert(err, tc.ErrorIsNil)
		now := time.Now()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = unit.SetAgentStatus(sInfo)
		c.Assert(err, tc.ErrorIsNil)
		units[i] = unit
	}
	return units
}

func (s *applicationSuite) assertBlockedErrorAndLiveliness(
	c *tc.C,
	err error,
	msg string,
	living1 state.Living,
	living2 state.Living,
	living3 state.Living,
	living4 state.Living,
) {
	s.AssertBlocked(c, err, msg)
	assertLife(c, living1, state.Alive)
	assertLife(c, living2, state.Alive)
	assertLife(c, living3, state.Alive)
	assertLife(c, living4, state.Alive)
}

func (s *applicationSuite) TestBlockChangesDestroyPrincipalUnits(c *tc.C) {
	apiv16 := s.apiv16()
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyPrincipalUnits")
	err := apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *applicationSuite) TestBlockRemoveDestroyPrincipalUnits(c *tc.C) {
	apiv16 := s.apiv16()
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyPrincipalUnits")
	err := apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *applicationSuite) TestBlockDestroyDestroyPrincipalUnits(c *tc.C) {
	apiv16 := s.apiv16()
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockDestroyModel(c, "TestBlockDestroyDestroyPrincipalUnits")
	err := apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)
}

func (s *applicationSuite) assertDestroySubordinateUnits(c *tc.C, wordpress0, logging0 *state.Unit) {
	apiv16 := s.apiv16()
	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be responsible for destroying the subordinate.)
	err := apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	c.Assert(err, tc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate, .*`)
	assertLife(c, wordpress0, state.Dying)
	assertLife(c, logging0, state.Alive)
}

func (s *applicationSuite) TestBlockRemoveDestroySubordinateUnits(c *tc.C) {
	apiv16 := s.apiv16()
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	s.BlockRemoveObject(c, "TestBlockRemoveDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *applicationSuite) TestBlockChangesDestroySubordinateUnits(c *tc.C) {
	apiv16 := s.apiv16()
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	s.BlockAllChanges(c, "TestBlockChangesDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *applicationSuite) TestBlockDestroyDestroySubordinateUnits(c *tc.C) {
	apiv16 := s.apiv16()
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = apiv16.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, tc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate, .*`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *applicationSuite) TestClientSetApplicationConstraints(c *tc.C) {
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)
	err = s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := app.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, cons)
}

func (s *applicationSuite) setupSetApplicationConstraints(c *tc.C) (*state.Application, constraints.Value) {
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)
	return app, cons
}

func (s *applicationSuite) assertSetApplicationConstraints(c *tc.C, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, tc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := application.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, cons)
}

func (s *applicationSuite) assertSetApplicationConstraintsBlocked(c *tc.C, msg string, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroySetApplicationConstraints(c *tc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockDestroyModel(c, "TestBlockDestroySetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockRemoveSetApplicationConstraints(c *tc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockRemoveObject(c, "TestBlockRemoveSetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockChangesSetApplicationConstraints(c *tc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockAllChanges(c, "TestBlockChangesSetApplicationConstraints")
	s.assertSetApplicationConstraintsBlocked(c, "TestBlockChangesSetApplicationConstraints", app, cons)
}

func (s *applicationSuite) TestClientGetApplicationConstraints(c *tc.C) {
	fooConstraints := constraints.MustParse("arch=amd64", "mem=4G")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "foo",
		Constraints: fooConstraints,
	})
	barConstraints := constraints.MustParse("arch=amd64", "mem=128G", "cores=64")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "bar",
		Constraints: barConstraints,
	})

	results, err := s.applicationAPI.GetConstraints(params.Entities{
		Entities: []params.Entity{
			{Tag: "wat"}, {Tag: "machine-0"}, {Tag: "user-foo"},
			{Tag: "application-foo"}, {Tag: "application-bar"}, {Tag: "application-wat"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ApplicationGetConstraintsResults{
		Results: []params.ApplicationConstraint{
			{
				Error: &params.Error{Message: `"wat" is not a valid tag`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got machine`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got user`},
			}, {
				Constraints: fooConstraints,
			}, {
				Constraints: barConstraints,
			}, {
				Error: &params.Error{Message: `application "wat" not found`, Code: "not found"},
			},
		}})
}

func (s *applicationSuite) checkEndpoints(c *tc.C, mysqlAppName string, endpoints map[string]params.CharmRelation) {
	c.Assert(endpoints["wordpress"], tc.DeepEquals, params.CharmRelation{
		Name:      "db",
		Role:      "requirer",
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     "global",
	})
	ep := params.CharmRelation{
		Name:      "server",
		Role:      "provider",
		Interface: "mysql",
		Scope:     "global",
	}
	// Remote applications don't use scope.
	if mysqlAppName == "hosted-mysql" {
		ep.Scope = ""
	}
	c.Assert(endpoints[mysqlAppName], tc.DeepEquals, ep)
}

func (s *applicationSuite) setupRelationScenario(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) assertAddRelation(c *tc.C, endpoints, viaCIDRs []string) {
	s.setupRelationScenario(c)

	res, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs})
	c.Assert(err, tc.ErrorIsNil)
	// Show that the relation was added.
	wpApp, err := s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rels, err := wpApp.Relations()
	c.Assert(err, tc.ErrorIsNil)
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), tc.Equals, 2)

	// We may be related to a local application or a remote offer
	// or an application in another model.
	var mySqlApplication state.ApplicationEntity
	mySqlApplication, err = s.State.RemoteApplication("hosted-mysql")
	if errors.IsNotFound(err) {
		mySqlApplication, err = s.State.RemoteApplication("othermysql")
		if errors.IsNotFound(err) {
			mySqlApplication, err = s.State.Application("mysql")
			c.Assert(err, tc.ErrorIsNil)
			s.checkEndpoints(c, "mysql", res.Endpoints)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			s.checkEndpoints(c, "othermysql", res.Endpoints)
		}
	} else {
		c.Assert(err, tc.ErrorIsNil)
		s.checkEndpoints(c, "hosted-mysql", res.Endpoints)
	}
	c.Assert(err, tc.ErrorIsNil)
	rels, err = mySqlApplication.Relations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rels), tc.Equals, 1)
}

func (s *applicationSuite) TestSuccessfullyAddRelation(c *tc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestBlockDestroyAddRelation(c *tc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}
func (s *applicationSuite) TestBlockRemoveAddRelation(c *tc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}

func (s *applicationSuite) TestBlockChangesAddRelation(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.BlockAllChanges(c, "TestBlockChangesAddRelation")
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: []string{"wordpress", "mysql"}})
	s.AssertBlocked(c, err, "TestBlockChangesAddRelation")
}

func (s *applicationSuite) TestSuccessfullyAddRelationSwapped(c *tc.C) {
	// Show that the order of the applications listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the application
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestCallWithOnlyOneEndpoint(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestCallWithOneEndpointTooMany(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *applicationSuite) TestAddAlreadyAddedRelation(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Add a relation between wordpress and mysql.
	endpoints := []string{"wordpress", "mysql"}
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	// And try to add it again.
	_, err = s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server`)
}

func (s *applicationSuite) setupRemoteApplication(c *tc.C) {
	results, err := s.applicationAPI.Consume(params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{
			{ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         testing.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "A pretty popular database",
				Endpoints: []params.RemoteEndpoint{
					{Name: "server", Interface: "mysql", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.OneError(), tc.IsNil)
}

func (s *applicationSuite) TestAddRemoteRelation(c *tc.C) {
	s.setupRemoteApplication(c)
	// There's already a wordpress in the scenario this assertion sets up.
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationWithRelName(c *tc.C) {
	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationVia(c *tc.C) {
	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, []string{"192.168.0.0/16"})

	rel, err := s.State.KeyRelation("wordpress:db hosted-mysql:server")
	c.Assert(err, tc.ErrorIsNil)
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange("192.168.0.0/16")
	wc.AssertNoChange()
}

func (s *applicationSuite) TestAddRemoteRelationOnlyOneEndpoint(c *tc.C) {
	s.setupRemoteApplication(c)
	endpoints := []string{"hosted-mysql"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestAlreadyAddedRemoteRelation(c *tc.C) {
	s.setupRemoteApplication(c)
	endpoints := []string{"wordpress", "hosted-mysql"}
	s.assertAddRelation(c, endpoints, nil)

	// And try to add it again.
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`cannot add relation "wordpress:db hosted-mysql:server": relation wordpress:db hosted-mysql:server`))
}

func (s *applicationSuite) TestRemoteRelationInvalidEndpoint(c *tc.C) {
	s.setupRemoteApplication(c)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, `saas application "hosted-mysql" has no "nope" relation`)
}

func (s *applicationSuite) TestRemoteRelationNoMatchingEndpoint(c *tc.C) {
	results, err := s.applicationAPI.Consume(params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{
			{ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag: testing.ModelTag.String(),
				OfferName:      "hosted-db2",
				OfferUUID:      "hosted-db2-uuid",
				Endpoints: []params.RemoteEndpoint{
					{Name: "database", Interface: "db2", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.OneError(), tc.IsNil)

	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress", "hosted-db2"}
	_, err = s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestRemoteRelationApplicationNotFound(c *tc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress", "unknown"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, tc.ErrorMatches, `application "unknown" not found`)
}

// addCharmToState emulates the side-effects of an AddCharm call so that the
// deploy tests in the suite can still work even though the AddCharmX calls
// have been updated to return NotSupported errors for Juju 3.
func (s *applicationSuite) addCharmToState(c *tc.C, charmURL string, name string) (string, charm.Charm) {
	curl := charm.MustParseURL(charmURL)

	if curl.Revision < 0 {
		base := curl.String()

		if rev, found := s.lastKnownRev[base]; found {
			curl.Revision = rev + 1
		} else {
			curl.Revision = 0
		}

		s.lastKnownRev[base] = curl.Revision
	}

	_, err := s.State.PrepareCharmUpload(charmURL)
	c.Assert(err, tc.ErrorIsNil)

	ch, err := charm.ReadCharmArchive(
		testcharms.RepoWithSeries("quantal").CharmArchivePath(c.MkDir(), name))
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.UpdateUploadedCharm(state.CharmInfo{
		Charm:       ch,
		ID:          charmURL,
		StoragePath: fmt.Sprintf("charms/%s", name),
		SHA256:      "1234",
		Version:     ch.Version(),
	})
	c.Assert(err, tc.ErrorIsNil)

	return charmURL, ch
}

func (s *applicationSuite) TestValidateSecretConfig(c *tc.C) {
	chCfg := &charm.Config{
		Options: map[string]charm.Option{
			"foo": {Type: "secret"},
		},
	}
	cfg := charm.Settings{
		"foo": "bar",
	}
	err := application.ValidateSecretConfig(chCfg, cfg)
	c.Assert(err, tc.ErrorMatches, `invalid secret URI for option "foo": secret URI "bar" not valid`)

	cfg = charm.Settings{"foo": "secret:cj4v5vm78ohs79o84r4g"}
	err = application.ValidateSecretConfig(chCfg, cfg)
	c.Assert(err, tc.ErrorIsNil)
}
