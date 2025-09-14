// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployApplication demands that a charm already exists in state,
// and that is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.JujuConnSuite
	charm *state.Charm
}

func TestDeployLocalSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &DeployLocalSuite{})
}

func (s *DeployLocalSuite) SetUpSuite(c *tc.C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *DeployLocalSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	curl := charm.MustParseURL("local:quantal/dummy")
	ch := testcharms.RepoForSeries("quantal").CharmDir("dummy")
	charm, err := testing.PutCharm(s.State, curl, ch)
	c.Assert(err, tc.ErrorIsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployControllerNotAllowed(c *tc.C) {
	ch := s.AddTestingCharm(c, "juju-controller")
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "my-controller",
			Charm:           ch,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorMatches, "manual deploy of the controller charm not supported")
}

func (s *DeployLocalSuite) TestDeployMinimal(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	s.assertCharm(c, app, s.charm.URL())
	s.assertSettings(c, app, charm.Settings{})
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes{})
	s.assertConstraints(c, app, constraints.MustParse("arch=amd64"))
	s.assertMachines(c, app, constraints.Value{})
}

func (s *DeployLocalSuite) TestDeployChannel(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.CharmOrigin, tc.DeepEquals, &state.CharmOrigin{
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}})
}

func (s *DeployLocalSuite) TestDeployWithImplicitBindings(c *tc.C) {
	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName:  "bob",
			Charm:            wordpressCharm,
			EndpointBindings: nil,
			CharmOrigin:      corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"": network.AlphaSpaceId,
		// relation names
		"url":             network.AlphaSpaceId,
		"logging-dir":     network.AlphaSpaceId,
		"monitoring-port": network.AlphaSpaceId,
		"db":              network.AlphaSpaceId,
		"cache":           network.AlphaSpaceId,
		"cluster":         network.AlphaSpaceId,
		// extra-bindings names
		"db-client": network.AlphaSpaceId,
		"admin-api": network.AlphaSpaceId,
		"foo-bar":   network.AlphaSpaceId,
	})
}

func (s *DeployLocalSuite) addWordpressCharm(c *tc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmWithExtraBindings(c *tc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress-extra-bindings")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmFromURL(c *tc.C, charmURL *charm.URL) *state.Charm {
	ch := testcharms.RepoForSeries("quantal").CharmDir(charmURL.Name)
	wordpressCharm, err := testing.PutCharm(s.State, charmURL, ch)
	c.Assert(err, tc.ErrorIsNil)
	return wordpressCharm
}

func (s *DeployLocalSuite) assertBindings(c *tc.C, app application.Application, expected map[string]string) {
	type withEndpointBindings interface {
		EndpointBindings() (application.Bindings, error)
	}
	bindings, err := app.(withEndpointBindings).EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings.Map(), tc.DeepEquals, expected)
}

func (s *DeployLocalSuite) TestDeployWithSomeSpecifiedBindings(c *tc.C) {
	wordpressCharm := s.addWordpressCharm(c)
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	publicSpace, err := s.State.AddSpace("public", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   publicSpace.Id(),
				"db": dbSpace.Id(),
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		// default binding
		"": publicSpace.Id(),
		// relation names
		"url":             publicSpace.Id(),
		"logging-dir":     publicSpace.Id(),
		"monitoring-port": publicSpace.Id(),
		"db":              dbSpace.Id(),
		"cache":           publicSpace.Id(),
		// extra-bindings names
		"db-client": publicSpace.Id(),
		"admin-api": publicSpace.Id(),
		"foo-bar":   publicSpace.Id(),
	})
}

func (s *DeployLocalSuite) TestDeployWithBoundRelationNamesAndExtraBindingsNames(c *tc.C) {
	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	publicSpace, err := s.State.AddSpace("public", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	internalSpace, err := s.State.AddSpace("internal", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":          publicSpace.Id(),
				"db":        dbSpace.Id(),
				"db-client": dbSpace.Id(),
				"admin-api": internalSpace.Id(),
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"":                publicSpace.Id(),
		"url":             publicSpace.Id(),
		"logging-dir":     publicSpace.Id(),
		"monitoring-port": publicSpace.Id(),
		"db":              dbSpace.Id(),
		"cache":           publicSpace.Id(),
		"db-client":       dbSpace.Id(),
		"admin-api":       internalSpace.Id(),
		"cluster":         publicSpace.Id(),
		"foo-bar":         publicSpace.Id(), // like for relations, uses the application-default.
	})

}

func (s *DeployLocalSuite) TestDeployWithInvalidSpace(c *tc.C) {
	wordpressCharm := s.addWordpressCharm(c)
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	publicSpace, err := s.State.AddSpace("public", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   publicSpace.Id(),
				"db": "42", //unknown space id
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorMatches, `cannot add application "bob": space not found`)
	c.Check(app, tc.IsNil)
	// The application should not have been added
	_, err = s.State.Application("bob")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *DeployLocalSuite) TestDeployResources(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			EndpointBindings: map[string]string{
				"": "public",
			},
			Resources:   map[string]string{"foo": "bar"},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Resources, tc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *DeployLocalSuite) TestDeploySettings(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"title":       "banana cupcakes",
				"skill-level": 9901,
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	s.assertSettings(c, app, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"skill-level": 99.01,
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorMatches, `option "skill-level" expected int, got 99.01`)
	_, err = s.State.Application("bob")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func sampleApplicationConfigSchema() environschema.Fields {
	schema := environschema.Fields{
		"title":       environschema.Attr{Type: environschema.Tstring},
		"outlook":     environschema.Attr{Type: environschema.Tstring},
		"username":    environschema.Attr{Type: environschema.Tstring},
		"skill-level": environschema.Attr{Type: environschema.Tint},
	}
	return schema
}

func (s *DeployLocalSuite) TestDeployWithApplicationConfig(c *tc.C) {
	cfg, err := coreconfig.NewConfig(map[string]interface{}{
		"outlook":     "good",
		"skill-level": 1,
	}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName:   "bob",
			Charm:             s.charm,
			ApplicationConfig: cfg,
			CharmOrigin:       corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes{
		"outlook":     "good",
		"skill-level": 1,
	})
}

func (s *DeployLocalSuite) TestDeployConstraints(c *tc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, tc.ErrorIsNil)
	applicationCons := constraints.MustParse("cores=2")

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	s.assertConstraints(c, app, constraints.MustParse("cores=2 arch=amd64"))
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	applicationCons := constraints.MustParse("cores=2")
	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        2,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, tc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, tc.Equals, 2)
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	applicationCons := constraints.MustParse("cores=2")
	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, tc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, tc.Equals, 1)
	c.Assert(f.args.Placement, tc.HasLen, 1)
	c.Assert(*f.args.Placement[0], tc.Equals, instance.Placement{Scope: instance.MachineScope, Directive: "0"})
}

func (s *DeployLocalSuite) TestDeployForceMachineIdWithContainer(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	applicationCons := constraints.MustParse("cores=2")
	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement(fmt.Sprintf("%s:0", instance.LXD))},
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, tc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, tc.Equals, 1)
	c.Assert(f.args.Placement, tc.HasLen, 1)
	c.Assert(*f.args.Placement[0], tc.Equals, instance.Placement{Scope: string(instance.LXD), Directive: "0"})
}

func (s *DeployLocalSuite) TestDeploy(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{
		{Scope: s.State.ModelUUID(), Directive: "valid"},
		{Scope: "#", Directive: "0"},
		{Scope: "lxd", Directive: "1"},
		{Scope: "lxd", Directive: ""},
	}
	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        4,
			Placement:       placement,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, tc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, tc.Equals, 4)
	c.Assert(f.args.Placement, tc.DeepEquals, placement)
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirements(c *tc.C) {
	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	charm, err := testing.PutCharm(s.State, curl, ch)
	c.Assert(err, tc.ErrorIsNil)

	var f = fakeDeployer{}

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorMatches, `(?s)Charm cannot be deployed because:
  - charm requires Juju version >= 42.0.0.*`)
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirementsAndForce(c *tc.C) {
	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	charm, err := testing.PutCharm(s.State, curl, ch)
	c.Assert(err, tc.ErrorIsNil)

	var f = fakeDeployer{}

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			Force:           true, // bypass assumes checks
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DeployLocalSuite) TestDeployWithFewerPlacement(c *tc.C) {
	var f fakeDeployer

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{{Scope: s.State.ModelUUID(), Directive: "valid"}}
	_, err = application.DeployApplication(&f,
		model,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        3,
			Placement:       placement,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.args.Name, tc.Equals, "bob")
	c.Assert(f.args.Charm, tc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, tc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, tc.Equals, 3)
	c.Assert(f.args.Placement, tc.DeepEquals, placement)
}

func (s *DeployLocalSuite) assertCharm(c *tc.C, app application.Application, expect string) {
	curl, force := app.CharmURL()
	c.Assert(*curl, tc.Equals, expect)
	c.Assert(force, tc.IsFalse)
}

func (s *DeployLocalSuite) assertSettings(c *tc.C, app application.Application, _ charm.Settings) {
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	expected := s.charm.Config().DefaultSettings()
	for name, value := range settings {
		expected[name] = value
	}
	c.Assert(settings, tc.DeepEquals, expected)
}

func (s *DeployLocalSuite) assertApplicationConfig(c *tc.C, app application.Application, wantCfg coreconfig.ConfigAttributes) {
	cfg, err := app.ApplicationConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, wantCfg)
}

func (s *DeployLocalSuite) assertConstraints(c *tc.C, app application.Application, expect constraints.Value) {
	cons, err := app.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertMachines(c *tc.C, app application.Application, expectCons constraints.Value, expectIds ...string) {
	type withAssignedMachineId interface {
		AssignedMachineId() (string, error)
	}

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, len(expectIds))
	// first manually tell state to assign all the units
	for _, unit := range units {
		id := unit.UnitTag().Id()
		res, err := s.State.AssignStagedUnits([]string{id})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(res[0].Error, tc.ErrorIsNil)
		c.Assert(res[0].Unit, tc.Equals, id)
	}

	// refresh the list of units from state
	units, err = app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, len(expectIds))
	unseenIds := set.NewStrings(expectIds...)
	for _, unit := range units {
		id, err := unit.(withAssignedMachineId).AssignedMachineId()
		c.Assert(err, tc.ErrorIsNil)
		unseenIds.Remove(id)
		machine, err := s.State.Machine(id)
		c.Assert(err, tc.ErrorIsNil)
		cons, err := machine.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(cons, tc.DeepEquals, expectCons)
	}
	c.Assert(unseenIds, tc.DeepEquals, set.NewStrings())
}

type stateDeployer struct {
	*state.State
}

func (d stateDeployer) AddApplication(args state.AddApplicationArgs) (application.Application, error) {
	app, err := d.State.AddApplication(args)
	if err != nil {
		return nil, err
	}
	return application.NewStateApplication(d.State, app), nil
}

type fakeDeployer struct {
	args          state.AddApplicationArgs
	controllerCfg *controller.Config
}

func (f *fakeDeployer) ControllerConfig() (controller.Config, error) {
	if f.controllerCfg != nil {
		return *f.controllerCfg, nil
	}
	return controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, map[string]interface{}{})
}

func (f *fakeDeployer) AddApplication(args state.AddApplicationArgs) (application.Application, error) {
	f.args = args
	return nil, nil
}
