// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/arch"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testcharms"
	jujuversion "github.com/juju/juju/version"
)

type ApplicationSuite struct {
	ConnSuite

	charm *state.Charm
	mysql *state.Application
}

func TestApplicationSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ApplicationSuite{})
}

func (s *ApplicationSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.charm = s.AddTestingCharm(c, "mysql")
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)

	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ApplicationSuite) assertNeedsCleanup(c *tc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dirty, tc.IsTrue)
}

func (s *ApplicationSuite) assertNoCleanup(c *tc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dirty, tc.IsFalse)
}

func (s *ApplicationSuite) TestSetCharm(c *tc.C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, s.charm.URL())
	c.Assert(force, tc.IsFalse)
	url, force := s.mysql.CharmURL()
	c.Assert(*url, tc.DeepEquals, s.charm.URL())
	c.Assert(force, tc.IsFalse)

	// Add a compatible charm and force it.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	ch, force, err = s.mysql.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
	url, force = s.mysql.CharmURL()
	c.Assert(*url, tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
}

func (s *ApplicationSuite) TestSetCharmCharmOrigin(c *tc.C) {
	// Add a compatible charm.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)
	rev := sch.Revision()
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: &rev,
		Channel:  &state.Channel{Risk: "stable"},
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		},
	}
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: origin,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	obtainedOrigin := s.mysql.CharmOrigin()
	c.Assert(obtainedOrigin, tc.DeepEquals, origin)
}

func (s *ApplicationSuite) TestSetCharmUpdateChannelURLNoChange(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	origin := defaultCharmOrigin(sch.URL())
	// This is a workaround, AddCharm creates a local
	// charm, which cannot have a channel in the CharmOrigin.
	// However, we need to test changing the channel only.
	origin.Source = "charm-hub"
	origin.Channel = &state.Channel{
		Risk: "stable",
	}
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: origin,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	mOrigin := s.mysql.CharmOrigin()
	c.Assert(mOrigin.Channel, tc.NotNil)
	c.Assert(mOrigin.Channel.Risk, tc.DeepEquals, "stable")

	cfg.CharmOrigin.Channel.Risk = "candidate"
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().Channel.Risk, tc.DeepEquals, "candidate")
}

func (s *ApplicationSuite) TestLXDProfileSetCharm(c *tc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile")
	app := s.AddTestingApplication(c, "lxd-profile", charm)

	c.Assert(charm.LXDProfile(), tc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)
	c.Assert(charm.LXDProfile(), tc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	ch, force, err = app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
	url, force = app.CharmURL()
	c.Assert(*url, tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
	c.Assert(charm.LXDProfile(), tc.DeepEquals, ch.LXDProfile())
}

func (s *ApplicationSuite) TestLXDProfileFailSetCharm(c *tc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile-fail")
	app := s.AddTestingApplication(c, "lxd-profile-fail", charm)

	c.Assert(charm.LXDProfile(), tc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)
	c.Assert(charm.LXDProfile(), tc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile-fail", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, ".*validating lxd profile: invalid lxd-profile\\.yaml.*")
}

func (s *ApplicationSuite) TestLXDProfileFailWithForceSetCharm(c *tc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile-fail")
	app := s.AddTestingApplication(c, "lxd-profile-fail", charm)

	c.Assert(charm.LXDProfile(), tc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)
	c.Assert(charm.LXDProfile(), tc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, tc.DeepEquals, charm.URL())
	c.Assert(force, tc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile-fail", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		Force:       true,
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	ch, force, err = app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
	url, force = app.CharmURL()
	c.Assert(*url, tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
	c.Assert(charm.LXDProfile(), tc.DeepEquals, ch.LXDProfile())
}

func (s *ApplicationSuite) TestCAASSetCharm(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch})

	// Add a compatible charm and force it.
	sch := state.AddCustomCharm(c, st, "mysql", "metadata.yaml", metaBaseCAAS, "kubernetes", 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	ch, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, sch.URL())
	c.Assert(force, tc.IsTrue)
}

func (s *ApplicationSuite) TestCAASSetCharmRequireNoUnits(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch, DesiredScale: 1})

	// Add a compatible charm and force it.
	sch := state.AddCustomCharm(c, st, "mysql", "metadata.yaml", metaBaseCAAS, "kubernetes", 2)

	cfg := state.SetCharmConfig{
		Charm:          sch,
		CharmOrigin:    defaultCharmOrigin(ch.URL()),
		ForceUnits:     true,
		RequireNoUnits: true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `.*application should not have units`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentFails(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: gitlab
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  type: stateful
  service: loadbalancer
`[1:]
	newCh := state.AddCustomCharm(c, st, "gitlab", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "gitlab" to charm "local:kubernetes/kubernetes-gitlab-2": cannot change a charm's deployment info`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentTypeFails(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "elastic-operator", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: elastic-operator
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  type: stateful
  service: loadbalancer
`[1:]
	newCh := state.AddCustomCharm(c, st, "elastic-operator", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "elastic-operator" to charm "local:kubernetes/kubernetes-elastic-operator-2": cannot change a charm's deployment type`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentModeFails(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "elastic-operator", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: elastic-operator
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  mode: workload
`[1:]
	newCh := state.AddCustomCharm(c, st, "elastic-operator", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "elastic-operator" to charm "local:kubernetes/kubernetes-elastic-operator-2": cannot change a charm's deployment mode`)
}

func (s *ApplicationSuite) TestSetCharmWithNewBindings(c *tc.C) {
	sp := s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")
	sch := s.AddMetaCharm(c, "mysql", metaBaseWithNewEndpoint, 2)

	// Assign new charm endpoint to "isolated" space
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
		EndpointBindings: map[string]string{
			"events": sp.Name(),
		},
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	expBindings := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		"events":  sp.Id(),
	}

	updatedBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), tc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestMergeBindings(c *tc.C) {
	s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")

	expBindings := map[string]string{
		"":               network.AlphaSpaceName,
		"metrics-client": network.AlphaSpaceName,
		"server":         network.AlphaSpaceName,
		"server-admin":   network.AlphaSpaceName,
		"db-router":      network.AlphaSpaceName,
	}
	b, err := s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)

	allSpaceInfosLookup, err := s.State.AllSpaceInfos()
	c.Assert(err, tc.ErrorIsNil)

	curBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curBindings, tc.DeepEquals, expBindings)

	// Use MergeBindings to bind "server" -> "isolated"
	b, err = state.NewBindings(s.State, map[string]string{
		"server": "isolated",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.MergeBindings(b, false)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the bindings have been updated
	expBindings["server"] = "isolated"
	b, err = s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	updatedBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updatedBindings, tc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestMergeBindingsWithForce(c *tc.C) {
	s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")

	sn, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.99.99.0/24"})
	c.Assert(err, tc.IsNil)
	_, err = s.State.AddSpace("far", "", []string{sn.ID()}, false)
	c.Assert(err, tc.IsNil)

	expBindings := map[string]string{
		"":               network.AlphaSpaceName,
		"metrics-client": network.AlphaSpaceName,
		"server":         network.AlphaSpaceName,
		"server-admin":   network.AlphaSpaceName,
		"db-router":      network.AlphaSpaceName,
	}
	b, err := s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)

	allSpaceInfosLookup, err := s.State.AllSpaceInfos()
	c.Assert(err, tc.ErrorIsNil)

	curBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curBindings, tc.DeepEquals, expBindings)

	// Use MergeBindings to force-bind "server" -> "far"
	b, err = state.NewBindings(s.State, map[string]string{
		"server": "far",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.MergeBindings(b, true)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the bindings have been updated
	expBindings["server"] = "far"
	b, err = s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	updatedBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updatedBindings, tc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestSetCharmWithNewBindingsAssigneToDefaultSpace(c *tc.C) {
	_ = s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")
	sch := s.AddMetaCharm(c, "mysql", metaBaseWithNewEndpoint, 2)

	// New charm endpoint should be auto-assigned to default space if not
	// explicitly bound by the operator.
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	expBindings := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		"events":  network.AlphaSpaceId,
	}

	updatedBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), tc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) assignUnitOnMachineWithSpaceToApplication(c *tc.C, a *state.Application, spaceName string) *state.Space {
	sn1, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.0.254.0/24"})
	c.Assert(err, tc.IsNil)

	sp, err := s.State.AddSpace(spaceName, "", []string{sn1.ID()}, false)
	c.Assert(err, tc.IsNil)

	m1, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: constraints.MustParse("spaces=isolated"),
	})
	c.Assert(err, tc.IsNil)
	err = m1.SetLinkLayerDevices(state.LinkLayerDeviceArgs{
		Name: "enp5s0",
		Type: network.EthernetDevice,
	})
	c.Assert(err, tc.IsNil)
	err = m1.SetDevicesAddresses(state.LinkLayerDeviceAddress{
		DeviceName:   "enp5s0",
		CIDRAddress:  "10.0.254.42/24",
		ConfigMethod: network.ConfigStatic,
	})
	c.Assert(err, tc.IsNil)

	u1, err := a.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, tc.IsNil)

	return sp
}

func (s *ApplicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *ApplicationSuite) TestSetCharmCharmSettings(c *tc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": "value"},
	})
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, s.combinedSettings(newCh, charm.Settings{"key": "value"}))

	newCh = s.AddConfigCharm(c, "mysql", newStringConfig, 3)
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"other": "one"},
	})
	c.Assert(err, tc.ErrorIsNil)

	cfg, err = s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key":   "value",
		"other": "one",
	}))
}

func (s *ApplicationSuite) TestSetCharmCharmSettingsForBranch(c *tc.C) {
	c.Assert(s.State.AddBranch("new-branch", "branch-user"), tc.ErrorIsNil)

	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": "value"},
	})
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)

	// Update the next generation settings.
	cfg["key"] = "next-gen-value"
	c.Assert(s.mysql.UpdateCharmConfig("new-branch", cfg), tc.ErrorIsNil)

	// Settings for the next generation reflect the change.
	cfg, err = s.mysql.CharmConfig("new-branch")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key": "next-gen-value",
	}))

	// Settings for the current generation are as set with charm.
	cfg, err = s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key": "value",
	}))
}

func (s *ApplicationSuite) TestSetCharmCharmSettingsInvalid(c *tc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": 123.45},
	})
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-mysql-2": validating config settings: option "key" expected string, got 123.45`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeries(c *tc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "application" to charm "ch:multi-series2-8": base "ubuntu@12.04" not supported by charm, the charm supported bases are: ubuntu@14.04, ubuntu@15.10`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeriesForce(c *tc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
		ForceBase:   true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	app, err = s.State.Application("application")
	c.Assert(err, tc.ErrorIsNil)
	ch, _, err = app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.Equals, "ch:multi-series2-8")
}

func (s *ApplicationSuite) TestClientApplicationSetCharmWrongOS(c *tc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-centos")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
		ForceBase:   true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "application" to charm "ch:multi-series-centos-1": OS "ubuntu" not supported by charm.*`)
}

func (s *ApplicationSuite) TestSetCharmPreconditions(c *tc.C) {
	logging := s.AddTestingCharm(c, "logging")
	cfg := state.SetCharmConfig{
		Charm:       logging,
		CharmOrigin: defaultCharmOrigin(logging.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-logging-1": cannot change an application's subordinacy`)
}

func (s *ApplicationSuite) TestSetCharmUpdatesBindings(c *tc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	clientSpace, err := s.State.AddSpace("client", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	oldCharm := s.AddMetaCharm(c, "mysql", metaBase, 44)

	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: oldCharm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
		EndpointBindings: map[string]string{
			"":       dbSpace.Id(),
			"server": dbSpace.Id(),
			"client": clientSpace.Id(),
		}})
	c.Assert(err, tc.ErrorIsNil)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)
	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err = application.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	updatedBindings, err := application.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), tc.DeepEquals, map[string]string{
		// Existing bindings are preserved.
		"":        dbSpace.Id(),
		"server":  dbSpace.Id(),
		"client":  clientSpace.Id(),
		"cluster": dbSpace.Id(), // inherited from defaults in AddApplication.
		// New endpoints use defaults.
		"foo":  dbSpace.Id(),
		"baz":  dbSpace.Id(),
		"just": dbSpace.Id(),
	})
}

var metaRelationConsumer = `
name: sqlvampire
summary: "connects to an sql server"
description: "lorem ipsum"
requires:
  server: mysql
`
var metaBase = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaBaseCAAS = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaBaseWithNewEndpoint = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
extra-bindings:
  events:
`
var metaDifferentProvider = `
name: mysql
description: none
summary: none
provides:
  server: mysql
  kludge: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaDifferentRequirer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  kludge: mysql
peers:
  cluster: mysql
`
var metaDifferentPeer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  client: mysql
peers:
  kludge: mysql
`
var metaRemoveNonPeerRelation = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaExtraEndpoints = `
name: mysql
description: none
summary: none
provides:
  server: mysql
  foo: bar
requires:
  client: mysql
  baz: woot
peers:
  cluster: mysql
  just: me
`
var lxdProfileMetaBase = `
name: lxd-profile
summary: "Fake LXDProfile"
description: "Fake description"
`

var setCharmEndpointsTests = []struct {
	summary string
	meta    string
	err     string
}{{
	summary: "different provider (but no relation yet)",
	meta:    metaDifferentProvider,
}, {
	summary: "different requirer (but no relation yet)",
	meta:    metaDifferentRequirer,
}, {
	summary: "different peer",
	meta:    metaDifferentPeer,
}, {
	summary: "attempt to break existing non-peer relations",
	meta:    metaRemoveNonPeerRelation,
	err:     `.*would break relation "fakeother:server fakemysql:server"`,
}, {
	summary: "same relations ok",
	meta:    metaBase,
}, {
	summary: "extra endpoints ok",
	meta:    metaExtraEndpoints,
}}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithoutRelations(c *tc.C) {
	revno := 2
	ms := s.AddMetaCharm(c, "mysql", metaBase, revno)
	app := s.AddTestingApplication(c, "fakemysql", ms)
	appServerEP, err := app.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)

	otherCharm := s.AddMetaCharm(c, "dummy", metaRelationConsumer, 42)
	otherApp := s.AddTestingApplication(c, "fakeother", otherCharm)
	otherServerEP, err := otherApp.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)

	// Add two mysql units so that peer relations get established and we
	// can check that we are allowed to break them when we upgrade.
	_, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Add a unit for the other application and establish a relation.
	_, err = otherApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(appServerEP, otherServerEP)
	c.Assert(err, tc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       ms,
		CharmOrigin: defaultCharmOrigin(ms.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	for i, t := range setCharmEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)

		newCh := s.AddMetaCharm(c, "mysql", t.meta, revno+i+1)
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithRelations(c *tc.C) {
	revno := 2
	providerCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, revno)
	providerApp := s.AddTestingApplication(c, "myprovider", providerCharm)

	cfg := state.SetCharmConfig{
		Charm:       providerCharm,
		CharmOrigin: defaultCharmOrigin(providerCharm.URL()),
	}
	err := providerApp.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	revno++
	requirerCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	requirerApp := s.AddTestingApplication(c, "myrequirer", requirerCharm)
	cfg = state.SetCharmConfig{Charm: requirerCharm, CharmOrigin: defaultCharmOrigin(requirerCharm.URL())}
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("myprovider:kludge", "myrequirer:kludge")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	revno++
	baseCharm := s.AddMetaCharm(c, "mysql", metaBase, revno)
	cfg = state.SetCharmConfig{
		Charm:       baseCharm,
		CharmOrigin: defaultCharmOrigin(baseCharm.URL()),
	}
	err = providerApp.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "myprovider" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "myrequirer" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
}

var stringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
`
var emptyConfig = `
options: {}
`
var floatConfig = `
options:
  key: {default: 0.42, description: Float key, type: float}
`
var newStringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
  other: {default: None, description: My Other, type: string}
`

var sortableConfig = `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
  alphabetic:
    type: int
    description: Something early in the alphabet.
  zygomatic:
    type: int
    description: Something late in the alphabet.
`

var wordpressConfig = `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
`

var setCharmConfigTests = []struct {
	summary     string
	startconfig string
	startvalues charm.Settings
	endconfig   string
	endvalues   charm.Settings
	err         string
}{{
	summary:     "add float key to empty config",
	startconfig: emptyConfig,
	endconfig:   floatConfig,
}, {
	summary:     "add string key to empty config",
	startconfig: emptyConfig,
	endconfig:   stringConfig,
}, {
	summary:     "add string key and preserve existing values",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "foo"},
	endconfig:   newStringConfig,
	endvalues:   charm.Settings{"key": "foo"},
}, {
	summary:     "remove string key",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "value"},
	endconfig:   emptyConfig,
}, {
	summary:     "remove float key",
	startconfig: floatConfig,
	startvalues: charm.Settings{"key": 123.45},
	endconfig:   emptyConfig,
}, {
	summary:     "change key type without values",
	startconfig: stringConfig,
	endconfig:   floatConfig,
}, {
	summary:     "change key type with values",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "value"},
	endconfig:   floatConfig,
}}

func (s *ApplicationSuite) TestSetCharmConfig(c *tc.C) {
	charms := map[string]*state.Charm{
		stringConfig:    s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:     s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:     s.AddConfigCharm(c, "wordpress", floatConfig, 3),
		newStringConfig: s.AddConfigCharm(c, "wordpress", newStringConfig, 4),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		app := s.AddTestingApplication(c, "wordpress", origCh)
		err := app.UpdateCharmConfig(model.GenerationMaster, t.startvalues)
		c.Assert(err, tc.ErrorIsNil)

		newCh := charms[t.endconfig]
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		var expectVals charm.Settings
		var expectCh *state.Charm
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
			expectCh = origCh
			expectVals = t.startvalues
		} else {
			c.Assert(err, tc.ErrorIsNil)
			expectCh = newCh
			expectVals = t.endvalues
		}

		sch, _, err := app.Charm()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(sch.URL(), tc.DeepEquals, expectCh.URL())

		chConfig, err := app.CharmConfig(model.GenerationMaster)
		c.Assert(err, tc.ErrorIsNil)
		expected := s.combinedSettings(sch, expectVals)
		if len(expected) == 0 {
			c.Assert(chConfig, tc.HasLen, 0)
		} else {
			c.Assert(chConfig, tc.DeepEquals, expected)
		}

		err = app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestSetCharmWithDyingApplication(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSequenceUnitIdsAfterDestroy(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/0")
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestAssignUnitsRemovedAfterAppDestroy(c *tc.C) {
	mariadb := s.AddTestingApplicationWithNumUnits(c, 1, "mariadb", s.charm)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	units, err := mariadb.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	unit := units[0]
	c.Assert(unit.Name(), tc.Equals, "mariadb/0")
	unitAssignments, err := s.State.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 1)

	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = mariadb.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, mariadb)

	unitAssignments, err = s.State.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 0)
}

func (s *ApplicationSuite) TestSequenceUnitIdsAfterDestroyForSidecarApplication(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	s.AddCleanup(func(*tc.C) { _ = st.Close() })
	f := factory.NewFactory(st, s.StatePool)
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "cockroachdb/0")
	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	assertCleanupCount(c, st, 2)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 0)

	ch = state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	app = f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
	unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "cockroachdb/0")
}

func (s *ApplicationSuite) TestSequenceUnitIds(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/0")
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestExplicitUnitName(c *tc.C) {
	name1 := "mysql/100"
	unit, err := s.mysql.AddUnit(state.AddUnitParams{
		UnitName: &name1,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, name1)
	name0 := "mysql/0"
	unit, err = s.mysql.AddUnit(state.AddUnitParams{
		UnitName: &name0,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, name0)
}

func (s *ApplicationSuite) TestSetCharmWhenDead(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, tc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)

		// Change the application life to Dead manually, as there's no
		// direct way of doing that otherwise.
		ops := []txn.Op{{
			C:      state.ApplicationsC,
			Id:     state.DocID(s.State, s.mysql.Name()),
			Update: bson.D{{"$set", bson.D{{"life", state.Dead}}}},
		}}

		state.RunTransaction(c, s.State, ops)
		assertLife(c, s.mysql, state.Dead)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(errors.Cause(err), tc.Equals, stateerrors.ErrDead)
}

func (s *ApplicationSuite) TestSetCharmWithRemovedApplication(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenRemoved(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.mysql.Destroy()
		c.Assert(err, tc.ErrorIsNil)
		assertRemoved(c, s.mysql)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenDyingIsOK(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, tc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
}

func (s *ApplicationSuite) TestSetCharmRetriesWithSameCharmURL(c *tc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsFalse)
				c.Assert(currentCh.URL(), tc.DeepEquals, s.charm.URL())

				cfg := state.SetCharmConfig{
					Charm:       sch,
					CharmOrigin: defaultCharmOrigin(sch.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
			},
			After: func() {
				// Verify the before hook worked.
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsFalse)
				c.Assert(currentCh.URL(), tc.DeepEquals, sch.URL())
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a retry.
			After: func() {
				// Verify it worked after the retry.
				err := s.mysql.Refresh()
				c.Assert(err, tc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsTrue)
				c.Assert(currentCh.URL(), tc.DeepEquals, sch.URL())
			},
		},
	).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldSettingsChanged(c *tc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)
	cfg := state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State,
		func() {
			err := s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value"})
			c.Assert(err, tc.ErrorIsNil)
		},
		nil, // Ensure there will be a retry.
	).Check()

	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenBothOldAndNewSettingsChanged(c *tc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add two units, which will keep the refcount of oldCh
				// and newCh settings greater than 0, while the application's
				// charm URLs change between oldCh and newCh. Ensure
				// refcounts change as expected.
				unit1, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, tc.ErrorIsNil)
				unit2, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, tc.ErrorIsNil)
				cfg := state.SetCharmConfig{
					Charm:       newCh,
					CharmOrigin: defaultCharmOrigin(newCh.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				err = unit1.SetCharmURL(newCh.URL())
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				// Update newCh settings, switch to oldCh and update its
				// settings as well.
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value1"})
				c.Assert(err, tc.ErrorIsNil)
				cfg = state.SetCharmConfig{
					Charm:       oldCh,
					CharmOrigin: defaultCharmOrigin(oldCh.URL()),
				}

				err = s.mysql.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = unit2.SetCharmURL(oldCh.URL())
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value2"})
				c.Assert(err, tc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the second attempt.
				err := s.mysql.Refresh()
				c.Assert(err, tc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsFalse)
				c.Assert(currentCh.URL(), tc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: func() {
				// SetCharm has refreshed its cached settings for oldCh
				// and newCh. Change them again to trigger another
				// attempt.
				cfg := state.SetCharmConfig{
					Charm:       newCh,
					CharmOrigin: defaultCharmOrigin(newCh.URL()),
				}

				err := s.mysql.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value3"})
				c.Assert(err, tc.ErrorIsNil)

				cfg = state.SetCharmConfig{
					Charm:       oldCh,
					CharmOrigin: defaultCharmOrigin(oldCh.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value4"})
				c.Assert(err, tc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, tc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsFalse)
				c.Assert(currentCh.URL(), tc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify the charm and refcounts after the final third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, tc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(force, tc.IsTrue)
				c.Assert(currentCh.URL(), tc.DeepEquals, newCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
			},
		},
	).Check()

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldBindingsChanged(c *tc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	mysqlKey := state.ApplicationGlobalKey(s.mysql.Name())
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, revno+1)

	cfg := state.SetCharmConfig{
		Charm:       oldCharm,
		CharmOrigin: defaultCharmOrigin(oldCharm.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	oldBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(oldBindings.Map(), tc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"kludge":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
	})
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	adminSpace, err := s.State.AddSpace("admin", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	updateBindings := func(updatesMap bson.M) {
		ops := []txn.Op{{
			C:      state.EndpointBindingsC,
			Id:     mysqlKey,
			Update: bson.D{{"$set", updatesMap}},
		}}
		state.RunTransaction(c, s.State, ops)
	}

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// First change.
				updateBindings(bson.M{
					"bindings.server": dbSpace.Id(),
					"bindings.kludge": adminSpace.Id(), // will be removed before newCharm is set.
				})
			},
			After: func() {
				// Second change.
				updateBindings(bson.M{
					"bindings.cluster": adminSpace.Id(),
				})
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify final bindings.
				newBindings, err := s.mysql.EndpointBindings()
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(newBindings.Map(), tc.DeepEquals, map[string]string{
					"":        network.AlphaSpaceId,
					"server":  dbSpace.Id(), // from the first change.
					"foo":     network.AlphaSpaceId,
					"client":  network.AlphaSpaceId,
					"baz":     network.AlphaSpaceId,
					"cluster": adminSpace.Id(), // from the second change.
					"just":    network.AlphaSpaceId,
				})
			},
		},
	).Check()

	cfg = state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmViolatesMaxRelationCount(c *tc.C) {
	wp0Charm := `
name: wordpress
description: foo
summary: foo
requires:
  db:
    interface: mysql
    limit: 99
`

	wp1Charm := `
name: wordpress
description: bar
summary: foo
requires:
  db:
    interface: mysql
    limit: 1
`

	// wp0Charm allows up to 99 relations for the db endpoint
	wpApp := s.AddTestingApplication(c, "wordpress", s.AddMetaCharm(c, "wordpress", wp0Charm, 1))

	// Establish 2 relations (note: mysql is already added by the suite setup code)
	s.AddTestingApplication(c, "some-mariadb", s.AddTestingCharm(c, "mariadb"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	eps, err = s.State.InferEndpoints("wordpress", "some-mariadb")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Try to update wordpress to a new version with max 1 relation for the db endpoint
	wpCharmWithRelLimit := s.AddMetaCharm(c, "wordpress", wp1Charm, 2)
	cfg := state.SetCharmConfig{
		Charm:       wpCharmWithRelLimit,
		CharmOrigin: defaultCharmOrigin(wpCharmWithRelLimit.URL()),
	}
	err = wpApp.SetCharm(cfg)
	c.Assert(err, tc.Satisfies, errors.IsQuotaLimitExceeded, tc.Commentf("expected quota limit error due to max relation mismatch"))

	// Try again with --force
	cfg.Force = true
	err = wpApp.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHash(c *tc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source: "charm-hub",
	})
	err := s.mysql.SetDownloadedIDAndHash("testing-ID", "testing-hash")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().ID, tc.Equals, "testing-ID")
	c.Assert(s.mysql.CharmOrigin().Hash, tc.Equals, "testing-hash")
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashFailEmptyStrings(c *tc.C) {
	err := s.mysql.SetDownloadedIDAndHash("", "")
	c.Assert(err, tc.Satisfies, errors.IsBadRequest)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashFailChangeID(c *tc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source:   "charm-hub",
		ID:       "testing-ID",
		Hash:     "testing-hash",
		Platform: &state.Platform{},
	})
	err := s.mysql.SetDownloadedIDAndHash("change-ID", "testing-hash")
	c.Assert(err, tc.Satisfies, errors.IsBadRequest)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashReplaceHash(c *tc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source: "charm-hub",
		ID:     "testing-ID",
		Hash:   "testing-hash",
	})
	err := s.mysql.SetDownloadedIDAndHash("", "new-testing-hash")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().Hash, tc.Equals, "new-testing-hash")
}

func (s *ApplicationSuite) setupSetDownloadedIDAndHash(c *tc.C, origin *state.CharmOrigin) {
	origin.Platform = &state.Platform{}
	chInfoOne := s.dummyCharm(c, "ch:testing-3")
	chOne, err := s.State.AddCharm(chInfoOne)
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       chOne,
		CharmOrigin: origin,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
}

var applicationUpdateCharmConfigTests = []struct {
	about   string
	initial charm.Settings
	update  charm.Settings
	expect  charm.Settings
	err     string
}{{
	about:  "unknown option",
	update: charm.Settings{"foo": "bar"},
	err:    `unknown option "foo"`,
}, {
	about:  "bad type",
	update: charm.Settings{"skill-level": "profound"},
	err:    `option "skill-level" expected int, got "profound"`,
}, {
	about:  "set string",
	update: charm.Settings{"outlook": "positive"},
	expect: charm.Settings{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": nil, "title": "sir"},
	expect:  charm.Settings{"title": "sir"},
}, {
	about:  "unset missing string",
	update: charm.Settings{"outlook": nil},
}, {
	about:   `empty strings are valid`,
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": "", "title": ""},
	expect:  charm.Settings{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: charm.Settings{"title": "sir"},
	update:  charm.Settings{"username": "admin001"},
	expect:  charm.Settings{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: charm.Settings{"username": "admin001", "title": "sir"},
	update:  charm.Settings{"username": nil, "title": "My Title"},
	expect:  charm.Settings{"title": "My Title"},
}, {
	about:  "non-string type",
	update: charm.Settings{"skill-level": 303},
	expect: charm.Settings{"skill-level": int64(303)},
}, {
	about:   "unset non-string type",
	initial: charm.Settings{"skill-level": 303},
	update:  charm.Settings{"skill-level": nil},
}}

func (s *ApplicationSuite) TestUpdateCharmConfig(c *tc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range applicationUpdateCharmConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateCharmConfig(model.GenerationMaster, t.initial)
			c.Assert(err, tc.ErrorIsNil)
		}
		err := app.UpdateCharmConfig(model.GenerationMaster, t.update)
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			cfg, err := app.CharmConfig(model.GenerationMaster)
			c.Assert(err, tc.ErrorIsNil)
			appConfig := t.expect
			expected := s.combinedSettings(sch, appConfig)
			c.Assert(cfg, tc.DeepEquals, expected)
		}
		err = app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) setupCharmForTestUpdateApplicationBase(c *tc.C, name string) *state.Application {
	ch := state.AddTestingCharmMultiSeries(c, s.State, name)
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("20.04"), name, ch)

	rev := ch.Revision()
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: &rev,
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		},
	}
	cfg := state.SetCharmConfig{
		Charm:       ch,
		CharmOrigin: origin,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	return app
}

func (s *ApplicationSuite) TestUpdateApplicationBase(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesToStart(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	err := app.UpdateApplicationBase(state.UbuntuBase("20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesAfterStart(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, tc.ErrorIsNil)
				err = unit.AssignToNewMachine()
				c.Assert(err, tc.ErrorIsNil)

				ops := []txn.Op{{
					C:  state.ApplicationsC,
					Id: state.DocID(s.State, "multi-series"),
					Update: bson.D{{"$set", bson.D{{
						"charm-origin.platform.channel", "22.04/stable"}}}},
				}}
				state.RunTransaction(c, s.State, ops)
			},
			After: func() {
				assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
			},
		},
	).Check()

	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesFail(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				cfg := state.SetCharmConfig{
					Charm:       v2,
					CharmOrigin: defaultCharmOrigin(v2.URL()),
				}
				err := app.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
			},
		},
	).Check()

	// Trusty is listed in only version 1 of the charm.
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorMatches,
		"updating application base: base \"ubuntu@22.04\" not supported by charm, "+
			"the charm supported bases are: ubuntu@20.04, ubuntu@18.04")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesPass(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				origin := defaultCharmOrigin(v2.URL())
				origin.Platform.OS = "ubuntu"
				origin.Platform.Channel = "18.04/stable"
				cfg := state.SetCharmConfig{
					Charm:       v2,
					CharmOrigin: origin,
				}
				err := app.SetCharm(cfg)
				c.Assert(err, tc.ErrorIsNil)
			},
		},
	).Check()

	// bionic is listed in both revisions of the charm.
	err := app.UpdateApplicationBase(state.UbuntuBase("18.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("18.04"))
}

func (s *ApplicationSuite) setupMultiSeriesUnitSubordinate(c *tc.C, app *state.Application, name string) *state.Application {
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)
	return s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, name)
}

func (s *ApplicationSuite) setupMultiSeriesUnitSubordinateGivenUnit(c *tc.C, app *state.Application, unit *state.Unit, name string) *state.Application {
	subApp := s.setupCharmForTestUpdateApplicationBase(c, name)

	eps, err := s.State.InferEndpoints(app.Name(), name)
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	ru, err := rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = subApp.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	return subApp
}

func assertApplicationBaseUpdate(c *tc.C, a *state.Application, base state.Base) {
	err := a.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	stBase, err := corebase.ParseBase(base.OS, base.Channel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a.Base().String(), tc.Equals, stBase.String())
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinate(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateFail(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("16.04"), false)
	c.Assert(err, tc.ErrorMatches, `updating application base: base "ubuntu@16.04" not supported by charm.*`)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("20.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateForce(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("16.04"), true)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("16.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("16.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesUnitCountChange(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 0)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add a subordinate and unit
				_ = s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))

	units, err = app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	subApp, err := s.State.Application("multi-series-subordinate")
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinate(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), tc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				_ = s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, "multi-series-subordinate2")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp2, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinateIncompatible(c *tc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), tc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				_ = s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, "multi-series-subordinate2")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("18.04"), false)
	c.Assert(err, tc.ErrorMatches, `updating application base: base "ubuntu@18.04" not supported by charm.*`)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("20.04"))

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, tc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp2, state.UbuntuBase("20.04"))
}

func assertNoSettingsRef(c *tc.C, st *state.State, appName string, sch *state.Charm) {
	cURL := sch.URL()
	_, err := state.ApplicationSettingsRefCount(st, appName, &cURL)
	c.Assert(errors.Cause(err), tc.Satisfies, errors.IsNotFound)
}

func assertSettingsRef(c *tc.C, st *state.State, appName string, sch *state.Charm, refcount int) {
	cURL := sch.URL()
	rc, err := state.ApplicationSettingsRefCount(st, appName, &cURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc, tc.Equals, refcount)
}

func (s *ApplicationSuite) TestSettingsRefCountWorks(c *tc.C) {
	// This test ensures the application settings per charm URL are
	// properly reference counted.
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	// Both refcounts are zero initially.
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// app is using oldCh, so its settings refcount is incremented.
	app := s.AddTestingApplication(c, appName, oldCh)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing to the same charm does not change the refcount.
	cfg := state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing from oldCh to newCh causes the refcount of oldCh's
	// settings to be decremented, while newCh's settings is
	// incremented. Consequently, because oldCh's refcount is 0, the
	// settings doc will be removed.
	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertSettingsRef(c, s.State, appName, newCh, 1)

	// The same but newCh swapped with oldCh.
	cfg = state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Adding a unit without a charm URL set does not affect the
	// refcount.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	charmURL := u.CharmURL()
	c.Assert(charmURL, tc.IsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Setting oldCh as the units charm URL increments oldCh, which is
	// used by app as well, hence 2.
	err = u.SetCharmURL(oldCh.URL())
	c.Assert(err, tc.ErrorIsNil)
	charmURL = u.CharmURL()
	c.Assert(charmURL, tc.NotNil)
	c.Assert(*charmURL, tc.Equals, oldCh.URL())
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// A dead unit does not decrement the refcount.
	err = u.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Once the unit is removed, refcount is decremented.
	err = u.Remove()
	c.Assert(err, tc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Finally, after the application is destroyed and removed (since the
	// last unit's gone), the refcount is again decremented.
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Having studiously avoided triggering cleanups throughout,
	// invoke them now and check that the charms are cleaned up
	// correctly -- and that a storm of cleanups for the same
	// charm are not a problem.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	err = oldCh.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = newCh.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSettingsRefCreateRace(c *tc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// just before setting the unit charm url, switch the application
	// away from the original charm, causing the attempt to fail
	// (because the settings have gone away; it's the unit's job to
	// fail out and handle the new charm when it comes back up
	// again).
	dropSettings := func() {
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		c.Assert(err, tc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, dropSettings).Check()

	err = unit.SetCharmURL(oldCh.URL())
	c.Check(err, tc.ErrorMatches, "settings reference: does not exist")
}

func (s *ApplicationSuite) TestSettingsRefRemoveRace(c *tc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// just before updating the app charm url, set that charm url on
	// a unit to block the removal.
	grabReference := func() {
		err := unit.SetCharmURL(oldCh.URL())
		c.Assert(err, tc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, grabReference).Check()

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	// check refs to both settings exist
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertSettingsRef(c, s.State, appName, newCh, 1)
}

func assertNoOffersRef(c *tc.C, st *state.State, appName string) {
	_, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(errors.Cause(err), tc.Satisfies, errors.IsNotFound)
}

func assertOffersRef(c *tc.C, st *state.State, appName string, refcount int) {
	rc, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc, tc.Equals, refcount)
}

func (s *ApplicationSuite) TestOffersRefCountWorks(c *tc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	// Once the offer is removed, refcount is decremented.
	err = ao.Remove("hosted-mysql", false)
	c.Assert(err, tc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	// Trying to destroy the app while there is an offer
	// succeeds when that offer has no connections
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *ApplicationSuite) TestDestroyApplicationRemoveOffers(c *tc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")

	offers, err := ao.AllApplicationOffers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(offers, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestOffersRefRace(c *tc.C) {
	addOffer := func() {
		ao := state.NewApplicationOffers(s.State)
		_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.Owner.Id(),
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()

	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *ApplicationSuite) TestOffersRefRaceWithForce(c *tc.C) {
	addOffer := func() {
		ao := state.NewApplicationOffers(s.State)
		_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.Owner.Id(),
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()

	op := s.mysql.DestroyOperation()
	op.Force = true
	err := s.State.ApplyOperation(op)
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

const mysqlBaseMeta = `
name: mysql
summary: "Database engine"
description: "A pretty popular database"
provides:
  server: mysql
`
const onePeerMeta = `
peers:
  cluster: mysql
`
const onePeerAltMeta = `
peers:
  minion: helper
`
const twoPeersMeta = `
peers:
  cluster: mysql
  loadbalancer: phony
`

func (s *ApplicationSuite) assertApplicationRelations(c *tc.C, app *state.Application, expectedKeys ...string) []*state.Relation {
	rels, err := app.Relations()
	c.Assert(err, tc.ErrorIsNil)
	if len(rels) == 0 {
		return nil
	}
	relKeys := make([]string, len(expectedKeys))
	for i, rel := range rels {
		relKeys[i] = rel.String()
	}
	sort.Strings(relKeys)
	c.Assert(relKeys, tc.DeepEquals, expectedKeys)
	return rels
}

func (s *ApplicationSuite) TestNewPeerRelationsAddedOnUpgrade(c *tc.C) {
	// Original mysql charm has no peer relations.
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoPeersMeta, 3)

	// No relations joined yet.
	s.assertApplicationRelations(c, s.mysql)

	cfg := state.SetCharmConfig{Charm: oldCh, CharmOrigin: defaultCharmOrigin(oldCh.URL())}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:cluster")

	cfg = state.SetCharmConfig{Charm: newCh, CharmOrigin: defaultCharmOrigin(newCh.URL())}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	rels := s.assertApplicationRelations(c, s.mysql, "mysql:cluster", "mysql:loadbalancer")

	// Check state consistency by attempting to destroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}
}

func (s *ApplicationSuite) TestStalePeerRelationsRemovedOnUpgrade(c *tc.C) {
	// Original mysql charm has no peer relations.
	// oldCh is mysql + the peer relation "mysql:cluster"
	// newCh is mysql + the peer relation "mysql:minion"
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerAltMeta, 42)

	// No relations joined yet.
	s.assertApplicationRelations(c, s.mysql)

	cfg := state.SetCharmConfig{Charm: oldCh, CharmOrigin: defaultCharmOrigin(oldCh.URL())}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:cluster")

	// Since the two charms have different URLs, the following SetCharm call
	// emulates a "juju refresh --switch" request. We expect that any prior
	// peer relations that are not referenced by the new charm metadata
	// to be dropped and any new peer relations to be created.
	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	rels := s.assertApplicationRelations(c, s.mysql, "mysql:minion")

	// Check state consistency by attempting to destroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}
}

func jujuInfoEp(applicationname string) state.Endpoint {
	return state.Endpoint{
		ApplicationName: applicationname,
		Relation: charm.Relation{
			Interface: "juju-info",
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
}

func (s *ApplicationSuite) TestTag(c *tc.C) {
	c.Assert(s.mysql.Tag().String(), tc.Equals, "application-mysql")
}

func (s *ApplicationSuite) TestMysqlEndpoints(c *tc.C) {
	_, err := s.mysql.Endpoint("mysql")
	c.Assert(err, tc.ErrorMatches, `application "mysql" has no "mysql" relation`)

	jiEP, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jiEP, tc.DeepEquals, jujuInfoEp("mysql"))

	serverEP, err := s.mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	serverAdminEP, err := s.mysql.Endpoint("server-admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverAdminEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "server-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	dbRouterEP, err := s.mysql.Endpoint("db-router")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dbRouterEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "db-router",
			Name:      "db-router",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	monitoringEP, err := s.mysql.Endpoint("metrics-client")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(monitoringEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "metrics",
			Name:      "metrics-client",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := s.mysql.Endpoints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.SameContents, []state.Endpoint{jiEP, serverEP, serverAdminEP, dbRouterEP, monitoringEP})
}

func (s *ApplicationSuite) TestRiakEndpoints(c *tc.C) {
	riak := s.AddTestingApplication(c, "myriak", s.AddTestingCharm(c, "riak"))

	_, err := riak.Endpoint("garble")
	c.Assert(err, tc.ErrorMatches, `application "myriak" has no "garble" relation`)

	jiEP, err := riak.Endpoint("juju-info")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jiEP, tc.DeepEquals, jujuInfoEp("myriak"))

	ringEP, err := riak.Endpoint("ring")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ringEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "riak",
			Name:      "ring",
			Role:      charm.RolePeer,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEP, err := riak.Endpoint("admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(adminEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	endpointEP, err := riak.Endpoint("endpoint")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endpointEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "endpoint",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := riak.Endpoints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.DeepEquals, []state.Endpoint{adminEP, endpointEP, jiEP, ringEP})
}

func (s *ApplicationSuite) TestWordpressEndpoints(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	_, err := wordpress.Endpoint("nonsense")
	c.Assert(err, tc.ErrorMatches, `application "wordpress" has no "nonsense" relation`)

	jiEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jiEP, tc.DeepEquals, jujuInfoEp("wordpress"))

	urlEP, err := wordpress.Endpoint("url")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(urlEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "url",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	ldEP, err := wordpress.Endpoint("logging-dir")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ldEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-dir",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	mpEP, err := wordpress.Endpoint("monitoring-port")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mpEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "monitoring",
			Name:      "monitoring-port",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	dbEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dbEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     1,
		},
	})

	cacheEP, err := wordpress.Endpoint("cache")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cacheEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "varnish",
			Name:      "cache",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     2,
			Optional:  true,
		},
	})

	eps, err := wordpress.Endpoints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.DeepEquals, []state.Endpoint{cacheEP, dbEP, jiEP, ldEP, mpEP, urlEP})
}

func (s *ApplicationSuite) TestApplicationRefresh(c *tc.C) {
	s1, err := s.State.Application(s.mysql.Name())
	c.Assert(err, tc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       s.charm,
		CharmOrigin: defaultCharmOrigin(s.charm.URL()),
		ForceUnits:  true,
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	testch, force, err := s1.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsFalse)
	c.Assert(testch.URL(), tc.DeepEquals, s.charm.URL())

	err = s1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	testch, force, err = s1.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.IsTrue)
	c.Assert(testch.URL(), tc.DeepEquals, s.charm.URL())

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
}

func (s *ApplicationSuite) TestSetPassword(c *tc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Application(s.mysql.Name())
	})
}

func (s *ApplicationSuite) TestApplicationExposed(c *tc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	// Check that setting and clearing the exposed flag works correctly.
	err := s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), tc.IsTrue)
	err = s.mysql.ClearExposed()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	// Check that setting and clearing the exposed flag repeatedly does not fail.
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), tc.IsTrue)

	// Make the application Dying and check that ClearExposed and MergeExposeSettings fail.
	// TODO(fwereade): maybe application destruction should always unexpose?
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.ClearExposed()
	c.Assert(err, tc.ErrorMatches, notAliveErr)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorMatches, notAliveErr)

	// Remove the application and check that both fail.
	err = u.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorMatches, notAliveErr)
	err = s.mysql.ClearExposed()
	c.Assert(err, tc.ErrorMatches, notAliveErr)
}

func (s *ApplicationSuite) TestApplicationExposeEndpoints(c *tc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	// Check argument validation
	err := s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"":               {},
		"bogus-endpoint": {},
	})
	c.Assert(err, tc.ErrorMatches, `.*endpoint "bogus-endpoint" not found`)
	err = s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"server": {ExposeToSpaceIDs: []string{"bogus-space-id"}},
	})
	c.Assert(err, tc.ErrorMatches, `.*space with ID "bogus-space-id" not found`)
	err = s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"server": {ExposeToCIDRs: []string{"not-a-cidr"}},
	})
	c.Assert(err, tc.ErrorMatches, `.*unable to parse "not-a-cidr" as a CIDR.*`)

	// Check that the expose parameters are properly persisted
	exp := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err = s.mysql.MergeExposeSettings(exp)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, exp)

	// Refresh model and ensure that we get the same parameters
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, exp)
}

func (s *ApplicationSuite) TestApplicationExposeEndpointMergeLogic(c *tc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	// Set initial value
	initial := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err := s.mysql.MergeExposeSettings(initial)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, initial)

	// The merge call should overwrite the "server" value and append the
	// entry for "server-admin"
	updated := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToCIDRs: []string{"0.0.0.0/0"},
		},
		"server-admin": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err = s.mysql.MergeExposeSettings(updated)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, updated)
}

func (s *ApplicationSuite) TestApplicationExposeWithoutSpaceAndCIDR(c *tc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	err := s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		// If the expose params are empty, an implicit 0.0.0.0/0 will
		// be assumed (equivalent to: juju expose --endpoints server)
		"server": {},
	})
	c.Assert(err, tc.ErrorIsNil)

	exp := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	}
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, exp, tc.Commentf("expected the implicit 0.0.0.0/0 and ::/0 CIDRs to be added when an empty ExposedEndpoint value is provided to MergeExposeSettings"))
}

func (s *ApplicationSuite) TestApplicationUnsetExposeEndpoints(c *tc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), tc.IsFalse)

	// Set initial value
	initial := map[string]state.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"13.37.0.0/16"},
		},
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err := s.mysql.MergeExposeSettings(initial)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, initial)

	// Check argument validation
	err = s.mysql.UnsetExposeSettings([]string{"bogus-endpoint"})
	c.Assert(err, tc.ErrorMatches, `.*endpoint "bogus-endpoint" not found`)
	err = s.mysql.UnsetExposeSettings([]string{"server-admin"})
	c.Assert(err, tc.ErrorMatches, `.*endpoint "server-admin" is not exposed`)

	// Check unexpose logic
	err = s.mysql.UnsetExposeSettings([]string{""})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.DeepEquals, map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}, tc.Commentf("expected the entry of the wildcard endpoint to be removed"))
	c.Assert(s.mysql.IsExposed(), tc.IsTrue, tc.Commentf("expected application to remain exposed"))

	err = s.mysql.UnsetExposeSettings([]string{"server"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), tc.HasLen, 0)
	c.Assert(s.mysql.IsExposed(), tc.IsFalse, tc.Commentf("expected exposed flag to be cleared when last expose setting gets removed"))
}

func (s *ApplicationSuite) TestAddUnit(c *tc.C) {
	// Check that principal units can be added on their own.
	c.Assert(s.mysql.UnitCount(), tc.Equals, 0)
	unitZero, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.UnitCount(), tc.Equals, 1)
	c.Assert(unitZero.Name(), tc.Equals, "mysql/0")
	c.Assert(unitZero.IsPrincipal(), tc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), tc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), tc.Equals, s.State.ModelUUID())

	unitOne, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitOne.Name(), tc.Equals, "mysql/1")
	c.Assert(unitOne.IsPrincipal(), tc.IsTrue)
	c.Assert(unitOne.SubordinateNames(), tc.HasLen, 0)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, tc.ErrorIsNil)

	// Add a subordinate application and check that units cannot be added directly.
	// to add a subordinate unit.
	subCharm := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", subCharm)
	_, err = logging.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorMatches, `cannot add unit to application "logging": application is a subordinate`)

	// Indirectly create a subordinate unit by adding a relation and entering
	// scope as a principal.
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(unitZero)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	subZero, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	// Check that once it's refreshed unitZero has subordinates.
	err = unitZero.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitZero.SubordinateNames(), tc.DeepEquals, []string{"logging/0"})

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, m.Id())
}

func (s *ApplicationSuite) TestAddUnitWhenNotAlive(c *tc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorMatches, `cannot add unit to application "mysql": application is not found or not alive`)
	c.Assert(u.EnsureDead(), tc.ErrorIsNil)
	c.Assert(u.Remove(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorMatches, `cannot add unit to application "mysql": application "mysql" not found`)
}

func (s *ApplicationSuite) TestAddCAASUnit(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	unitZero, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitZero.Name(), tc.Equals, "gitlab/0")
	c.Assert(unitZero.IsPrincipal(), tc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), tc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), tc.Equals, st.ModelUUID())

	err = unitZero.SetWorkloadVersion("3.combined")
	c.Assert(err, tc.ErrorIsNil)
	version, err := unitZero.WorkloadVersion()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, "3.combined")

	// But they do have status.
	us, err := unitZero.Status()
	c.Assert(err, tc.ErrorIsNil)
	us.Since = nil
	c.Assert(us, tc.DeepEquals, status.StatusInfo{
		Status:  status.Waiting,
		Message: status.MessageInstallingAgent,
		Data:    map[string]interface{}{},
	})
	as, err := unitZero.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(as.Since, tc.NotNil)
	as.Since = nil
	c.Assert(as, tc.DeepEquals, status.StatusInfo{
		Status: status.Allocating,
		Data:   map[string]interface{}{},
	})
}

func (s *ApplicationSuite) TestAgentTools(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	agentTools := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: "ubuntu",
	}

	tools, err := app.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tools.Version, tc.DeepEquals, agentTools)
}

func (s *ApplicationSuite) TestSetAgentVersion(c *tc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	agentVersion := version.MustParseBinary("2.0.1-ubuntu-and64")
	err := app.SetAgentVersion(agentVersion)
	c.Assert(err, tc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	tools, err := app.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tools.Version, tc.DeepEquals, agentVersion)
}

func (s *ApplicationSuite) TestAddUnitWithProviderIdNonCAASModel(c *tc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, tc.ErrorIsNil)
	_, err = u.ContainerInfo()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestReadUnit(c *tc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Check that retrieving a unit from state works correctly.
	unit, err := s.State.Unit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.State.Unit("mysql")
	c.Assert(err, tc.ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.State.Unit("mysql/0/0")
	c.Assert(err, tc.ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.State.Unit("pressword/0")
	c.Assert(err, tc.ErrorMatches, `unit "pressword/0" not found`)

	units, err := s.mysql.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sortedUnitNames(units), tc.DeepEquals, []string{"mysql/0", "mysql/1"})
}

func (s *ApplicationSuite) TestReadUnitWhenDying(c *tc.C) {
	// Test that we can still read units when the application is Dying...
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	_, err = s.mysql.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	// ...and when those units are Dying or Dead...
	testWhenDying(c, unit, noErr, noErr, func() error {
		_, err := s.mysql.AllUnits()
		return err
	}, func() error {
		_, err := s.State.Unit("mysql/0")
		return err
	})

	// ...and even, in a very limited way, when the application itself is removed.
	removeAllUnits(c, s.mysql)
	_, err = s.mysql.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroySimple(c *tc.C) {
	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyRemovesStatusHistory(c *tc.C) {
	err := s.mysql.SetStatus(status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, tc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 100}
	agentInfo, err := s.mysql.StatusHistory(filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(agentInfo), tc.Equals, 2)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	agentInfo, err = s.mysql.StatusHistory(filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agentInfo, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestDestroyStillHasUnits(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)

	c.Assert(unit.EnsureDead(), tc.ErrorIsNil)
	c.Assert(s.mysql.Refresh(), tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)

	c.Assert(unit.Remove(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyOnceHadUnits(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyStaleNonZeroUnitCount(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyStaleZeroUnitCount(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)

	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.Life(), tc.Equals, state.Dying)

	c.Assert(unit.Remove(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyWithRemovableRelation(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Destroy a application with no units in relation scope; check application and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyWithRemovableApplicationOpenedPortRanges(c *tc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 0)

	unit0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	portRangesUnit0, err := unit0.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	portRangesUnit0.Open(allEndpoints, network.MustParsePortRange("3000/tcp"))
	portRangesUnit0.Open(allEndpoints, network.MustParsePortRange("3001/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit0.Changes()), tc.ErrorIsNil)

	portRangesUnit0, err = unit0.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(portRangesUnit0.UniquePortRanges(), tc.HasLen, 2)

	unit1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	portRangesUnit1, err := unit1.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3001/tcp"))
	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3002/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), tc.ErrorIsNil)

	portRangesUnit1, err = unit1.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(portRangesUnit1.UniquePortRanges(), tc.HasLen, 2)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 3)

	portRangesUnit1.Close(allEndpoints, network.MustParsePortRange("3002/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), tc.ErrorIsNil)

	portRangesUnit1, err = unit1.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(portRangesUnit1.UniquePortRanges(), tc.HasLen, 1)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 2)

	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3003/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), tc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 3)

	err = unit1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit1.Remove()
	c.Assert(err, tc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 2)

	// Remove all units, all opened ports should be removed.
	err = unit0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = unit1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit1.Remove()
	c.Assert(err, tc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 0)

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestOpenedPortRanges(c *tc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	portRanges, err := unit.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)

	flush := func(expectedErr string) {
		if len(expectedErr) == 0 {
			c.Assert(st.ApplyOperation(portRanges.Changes()), tc.ErrorIsNil)
		} else {
			c.Assert(st.ApplyOperation(portRanges.Changes()), tc.ErrorMatches, expectedErr)
		}
		portRanges, err = unit.OpenedPortRanges()
		c.Assert(err, tc.ErrorIsNil)
	}

	c.Assert(portRanges.UniquePortRanges(), tc.HasLen, 0)
	portRanges.Open(allEndpoints, network.MustParsePortRange("3000/tcp"))
	portRanges.Open("data-port", network.MustParsePortRange("2000/udp"))
	// All good.
	flush(``)
	c.Assert(portRanges.UnitName(), tc.DeepEquals, `cockroachdb/0`)
	c.Assert(portRanges.UniquePortRanges(), tc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("3000/tcp"),
		network.MustParsePortRange("2000/udp"),
	})
	c.Assert(portRanges.ByEndpoint(), tc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// Errors for unknown endpoint.
	portRanges.Open("bad-endpoint", network.MustParsePortRange("2000/udp"))
	flush(`cannot open/close ports: open port range: endpoint "bad-endpoint" for application "cockroachdb" not found`)
	c.Assert(portRanges.ByEndpoint(), tc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// No ops for duplicated Open.
	portRanges.Open("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), tc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// Close one port.
	portRanges.Close("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), tc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
	})

	// No ops for Close non existing port.
	portRanges.Close("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), tc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
	})

	// Destroy the application; check application and
	// openedApplicationportRanges removed.
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), tc.HasLen, 0)

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelation(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *ApplicationSuite) assertDestroyWithReferencedRelation(c *tc.C, refresh bool) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err = s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.mysql.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	c.Assert(s.mysql.Destroy(), tc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel0.Life(), tc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are are both removed.
	c.Assert(ru.LeaveScope(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyQueuesUnitCleanup(c *tc.C) {
	// Add 5 units; block quick-remove of mysql/1 and mysql/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			preventUnitDestroyRemove(c, unit)
		}
	}

	s.assertNoCleanup(c)

	// Destroy mysql, and check units are not touched.
	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	s.assertNeedsCleanup(c)

	// Run the cleanup and check the units.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	for i, unit := range units {
		if i%2 != 0 {
			assertLife(c, unit, state.Dying)
		} else {
			assertRemoved(c, unit)
		}
	}

	// Check for queued unit cleanups, and run them.
	s.assertNeedsCleanup(c)
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestRemoveApplicationMachine(c *tc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), tc.IsNil)

	c.Assert(s.mysql.Destroy(), tc.IsNil)
	assertLife(c, s.mysql, state.Dying)

	// Application.Destroy adds units to cleanup, make it happen now.
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.IsNil)

	c.Assert(unit.Refresh(), tc.Satisfies, errors.IsNotFound)
	assertLife(c, machine, state.Dying)
}

func (s *ApplicationSuite) TestDestroyRemoveAlsoDeletesSecretPermissions(c *tc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	// Make a relation for the access scope.
	endpoint1, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, tc.ErrorIsNil)
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "logging",
		}),
	})
	endpoint2, err := application2.Endpoint("info")
	c.Assert(err, tc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint1, endpoint2},
	})

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       rel.Tag(),
		Subject:     s.mysql.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, s.mysql.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleView)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.SecretAccess(uri, s.mysql.Tag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyRemoveAlsoDeletesOwnedSecrets(c *tc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	_, err = store.GetSecret(uri)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Create again, no label clash.
	s.AddTestingApplication(c, "mysql", s.charm)
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyNoRemoveKeepsOwnedSecrets(c *tc.C) {
	// Create a relation so destroy does not remove.
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wp := s.AddTestingApplication(c, "wordpress", wpch)
	wpep, err := wp.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(mysqlep, wpep)
	c.Assert(err, tc.ErrorIsNil)

	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	_, err = store.GetSecret(uri)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestApplicationCleanupRemovesStorageConstraints(c *tc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u.SetCharmURL(ch.URL())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(app.Destroy(), tc.IsNil)
	assertLife(c, app, state.Dying)
	assertCleanupCount(c, s.State, 2)

	// These next API calls are normally done by the uniter.
	c.Assert(u.EnsureDead(), tc.ErrorIsNil)
	c.Assert(u.Remove(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)

	// Ensure storage constraints and settings are now gone.
	_, err = state.AppStorageConstraints(app)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	cfg := state.GetApplicationCharmConfig(s.State, app)
	err = cfg.Read()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestApplicationCleanupRemovesAppFromActiveBranches(c *tc.C) {
	s.assertNoCleanup(c)

	// setup branch, tracking and app with config changes.
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(s.Model.AddBranch("apple", "testuser"), tc.ErrorIsNil)
	branch, err := s.Model.Branch("apple")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(branch.AssignApplication(app.Name()), tc.ErrorIsNil)
	c.Assert(branch.AssignApplication(s.mysql.Name()), tc.ErrorIsNil)
	newCfg := map[string]interface{}{"outlook": "testing"}
	c.Assert(app.UpdateCharmConfig(branch.BranchName(), newCfg), tc.ErrorIsNil)

	// verify the branch setup
	c.Assert(branch.Refresh(), tc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), tc.DeepEquals, map[string][]string{
		app.Name():     {},
		s.mysql.Name(): {},
	})
	branchCfg := branch.Config()
	_, ok := branchCfg[app.Name()]
	c.Assert(ok, tc.IsTrue)

	// destroy the app
	c.Assert(app.Destroy(), tc.IsNil)
	assertRemoved(c, app)

	// Check the branch
	c.Assert(branch.Refresh(), tc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), tc.DeepEquals, map[string][]string{
		s.mysql.Name(): {},
	})
	c.Assert(branch.Config(), tc.HasLen, 0)
}

func (s *ApplicationSuite) TestRemoveQueuesLocalCharmCleanup(c *tc.C) {
	s.assertNoCleanup(c)

	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check a cleanup doc was added.
	s.assertNeedsCleanup(c)

	// Run the cleanup
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)

	// Check charm removed
	err = s.charm.Refresh()
	c.Check(err, tc.Satisfies, errors.IsNotFound)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestDestroyQueuesResourcesCleanup(c *tc.C) {
	s.assertNoCleanup(c)

	// Add a resource to the application, ensuring it is stored.
	rSt := s.State.Resources()
	const content = "abc"
	res := resourcetesting.NewCharmResource(c, "blob", content)
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res, strings.NewReader(content), state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)
	storagePath := state.ResourceStoragePath(c, s.State, outRes.ID)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), tc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Cleanup should be registered but not yet run.
	s.assertNeedsCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), tc.IsTrue)

	// Run the cleanup.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), tc.IsFalse)
}

func (s *ApplicationSuite) TestDestroyWithPlaceholderResources(c *tc.C) {
	s.assertNoCleanup(c)

	// Add a placeholder resource to the application.
	rSt := s.State.Resources()
	res := resourcetesting.NewPlaceholderResource(c, "blob", s.mysql.Name())
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res.Resource, nil, state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(outRes.IsPlaceholder(), tc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// No cleanup required for placeholder resources.
	state.AssertNoCleanupsWithKind(c, s.State, "resourceBlob")
}

func (s *ApplicationSuite) TestReadUnitWithChangingState(c *tc.C) {
	// Check that reading a unit after removing the application
	// fails nicely.
	err := s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, tc.ErrorMatches, `unit "mysql/0" not found`)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *ApplicationSuite) TestConstraints(c *tc.C) {
	// Constraints are initially empty (for now).
	cons, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&cons, tc.Not(tc.Satisfies), constraints.IsEmpty)
	c.Assert(cons, tc.DeepEquals, constraints.MustParse("arch=amd64"))

	// Constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(4096)}
	err = s.mysql.SetConstraints(cons2)
	c.Assert(err, tc.ErrorIsNil)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons3, tc.DeepEquals, cons2)

	// Constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(750)}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, tc.ErrorIsNil)
	cons5, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons5, tc.DeepEquals, cons4)

	// Destroy the existing application; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// ...but we can check that old constraints do not affect new applications
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&cons6, tc.Not(tc.Satisfies), constraints.IsEmpty)
	c.Assert(cons6, tc.DeepEquals, constraints.MustParse("arch=amd64"))
}

func (s *ApplicationSuite) TestArchConstraints(c *tc.C) {
	amdArch := "amd64"
	armArch := "arm64"

	cons2 := constraints.Value{Arch: &amdArch}
	err := s.mysql.SetConstraints(cons2)
	c.Assert(err, tc.ErrorIsNil)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons3, tc.DeepEquals, cons2)

	// Constraints error out if it's already set.
	cons4 := constraints.Value{Arch: &armArch}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, tc.ErrorMatches, "changing architecture \\(amd64\\) not supported")

	// Destroy the existing application; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// ...but we can check that old constraints do not affect new applications
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(constraints.IsEmpty(&cons6), tc.IsFalse)
	c.Assert(cons6, tc.DeepEquals, cons2)
}

func (s *ApplicationSuite) TestSetInvalidConstraints(c *tc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, tc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *ApplicationSuite) TestSetUnsupportedConstraintsWarning(c *tc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw), tc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{{
		Level:   loggo.WARNING,
		Message: `setting constraints on application "mysql": unsupported constraints: cpu-power`,
	}})
	scons, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scons, tc.DeepEquals, cons)
}

func (s *ApplicationSuite) TestConstraintsLifecycle(c *tc.C) {
	// Dying.
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)

	cons1 := constraints.MustParse("mem=1G")
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, tc.ErrorMatches, `cannot set constraints: application is not found or not alive`)

	scons, err := s.mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&scons, tc.Not(tc.Satisfies), constraints.IsEmpty)
	c.Assert(scons, tc.DeepEquals, constraints.MustParse("arch=amd64"))

	// Removed (== Dead, for a application).
	c.Assert(unit.EnsureDead(), tc.ErrorIsNil)
	c.Assert(unit.Remove(), tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, tc.ErrorMatches, `cannot set constraints: application is not found or not alive`)
	_, err = s.mysql.Constraints()
	c.Assert(err, tc.ErrorMatches, `constraints not found`)
}

func (s *ApplicationSuite) TestSubordinateConstraints(c *tc.C) {
	loggingCh := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", loggingCh)

	_, err := logging.Constraints()
	c.Assert(err, tc.Equals, state.ErrSubordinateConstraints)

	err = logging.SetConstraints(constraints.Value{})
	c.Assert(err, tc.Equals, state.ErrSubordinateConstraints)
}

func (s *ApplicationSuite) TestWatchUnitsBulkEvents(c *tc.C) {
	// Alive unit...
	alive, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Dying unit...
	dying, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, dying)
	err = dying.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// Dead unit...
	dead, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	preventUnitDestroyRemove(c, dead)
	err = dead.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// Gone unit.
	gone, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = gone.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// All except gone unit are reported in initial event.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Name(), dying.Name(), dead.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = dying.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = dead.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *ApplicationSuite) TestWatchUnitsLifecycle(c *tc.C) {
	// Empty initial event when no units.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Create one unit, check one change.
	quick, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Destroy that unit (short-circuited to removal), check one change.
	err = quick.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Create another, check one change.
	slow, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Change unit itself, no change.
	preventUnitDestroyRemove(c, slow)
	wc.AssertNoChange()

	// Make unit Dying, change detected.
	err = slow.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Make unit Dead, change detected.
	err = slow.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Remove unit, final change not detected.
	err = slow.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationSuite) TestWatchRelations(c *tc.C) {
	// TODO(fwereade) split this test up a bit.
	w := s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a relation; check change.
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wpi := 0
	addRelation := func() *state.Relation {
		name := fmt.Sprintf("wp%d", wpi)
		wpi++
		wp := s.AddTestingApplication(c, name, wpch)
		wpep, err := wp.Endpoint("db")
		c.Assert(err, tc.ErrorIsNil)
		rel, err := s.State.AddRelation(mysqlep, wpep)
		c.Assert(err, tc.ErrorIsNil)
		return rel
	}
	rel0 := addRelation()
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Add another relation; check change.
	rel1 := addRelation()
	wc.AssertChange(rel1.String())
	wc.AssertNoChange()

	// Destroy a relation; check change.
	err = rel0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Stop watcher; check change chan is closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Add a new relation; start a new watcher; check initial event.
	rel2 := addRelation()
	w = s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, w)
	wc.AssertChange(rel1.String(), rel2.String())
	wc.AssertNoChange()

	// Add a unit to the new relation; check no change.
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru2, err := rel2.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the relation with the unit in scope, and add another; check
	// changes.
	err = rel2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = rel2.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	rel3 := addRelation()
	wc.AssertChange(rel2.String(), rel3.String())
	wc.AssertNoChange()

	// Leave scope, destroying the relation, and check that change as well.
	err = ru2.LeaveScope()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(rel2.String())
	wc.AssertNoChange()

	// Watch relations on the requirer application too (exercises a
	// different path of the WatchRelations filter function)
	wpx := s.AddTestingApplication(c, "wpx", wpch)
	wpxWatcher := wpx.WatchRelations()
	defer testing.AssertStop(c, wpxWatcher)
	wpxWatcherC := testing.NewStringsWatcherC(c, wpxWatcher)
	wpxWatcherC.AssertChange()
	wpxWatcherC.AssertNoChange()

	wpxep, err := wpx.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	relx, err := s.State.AddRelation(mysqlep, wpxep)
	c.Assert(err, tc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()

	err = relx.SetSuspended(true, "")
	c.Assert(err, tc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()
}

func removeAllUnits(c *tc.C, s *state.Application) {
	us, err := s.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, tc.ErrorIsNil)
		err = u.Remove()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestWatchApplication(c *tc.C) {
	w := s.mysql.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	application, err := s.State.Application(s.mysql.Name())
	c.Assert(err, tc.ErrorIsNil)
	err = application.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = application.ClearExposed()
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()

	cfg := state.SetCharmConfig{
		Charm:       s.charm,
		CharmOrigin: defaultCharmOrigin(s.charm.URL()),
		ForceUnits:  true,
	}
	err = application.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove application, start new watch, check single event.
	err = application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// The destruction needs to have been processed by the txn watcher before the
	// watcher in the test is started or the destroy notification may come through
	// as an additional event.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = s.mysql.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *ApplicationSuite) TestWatchStorageConstraints(c *tc.C) {
	mysqlWatcher, err := s.mysql.WatchStorageConstraints()
	c.Assert(err, tc.ErrorIsNil)
	defer testing.AssertStop(c, mysqlWatcher)

	// Initial event.
	mysqlWc := testing.NewNotifyWatcherC(c, mysqlWatcher)
	mysqlWc.AssertOneChange()

	// Make one change, check one event.
	constraints := map[string]state.StorageConstraints{
		"data": {Count: 1, Size: 1024, Pool: "mypool"},
	}
	err = state.UpdateStorageConstraints(s.State, s.mysql, constraints)
	c.Assert(err, tc.ErrorIsNil)
	mysqlWc.AssertOneChange()

	// Upgrade the charm. It should still receive events despite the charm URL changed.
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta, 999)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	// Make another change, check one event.
	constraints = map[string]state.StorageConstraints{
		"data": {Count: 1, Size: 2048, Pool: "mypool"},
	}
	err = state.UpdateStorageConstraints(s.State, s.mysql, constraints)
	c.Assert(err, tc.ErrorIsNil)
	mysqlWc.AssertOneChange()

	// Check the watcher does not react when the content remains the same.
	err = state.UpdateStorageConstraints(s.State, s.mysql, constraints)
	c.Assert(err, tc.ErrorIsNil)
	mysqlWc.AssertNoChange()

	// Stop, check closed.
	testing.AssertStop(c, mysqlWatcher)
	mysqlWc.AssertClosed()
}

func (s *ApplicationSuite) TestWatchStorageConstraintsDoesNotCrossApplications(c *tc.C) {
	mysqlWatcher, err := s.mysql.WatchStorageConstraints()
	c.Assert(err, tc.ErrorIsNil)
	defer testing.AssertStop(c, mysqlWatcher)

	mariadbApp := s.AddTestingApplication(c, "mariadb", s.AddTestingCharm(c, "mariadb"))

	mariadbWatcher, err := mariadbApp.WatchStorageConstraints()
	c.Assert(err, tc.ErrorIsNil)
	defer testing.AssertStop(c, mysqlWatcher)

	// Initial event.
	mysqlWc := testing.NewNotifyWatcherC(c, mysqlWatcher)
	mysqlWc.AssertOneChange()
	mariadbWc := testing.NewNotifyWatcherC(c, mariadbWatcher)
	mariadbWc.AssertOneChange()

	// Check mysql watcher does not react when mariadb is updated.
	constraints := map[string]state.StorageConstraints{
		"data": {Count: 1, Size: 1024, Pool: "mypool"},
	}
	err = state.UpdateStorageConstraints(s.State, mariadbApp, constraints)
	c.Assert(err, tc.ErrorIsNil)
	mysqlWc.AssertNoChange()

	// Stop, check closed.
	testing.AssertStop(c, mysqlWatcher)
	mysqlWc.AssertClosed()
	testing.AssertStop(c, mariadbWatcher)
	mariadbWc.AssertClosed()
}

func (s *ApplicationSuite) TestMetricCredentials(c *tc.C) {
	err := s.mysql.SetMetricCredentials([]byte("hello there"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.MetricCredentials(), tc.DeepEquals, []byte("hello there"))

	application, err := s.State.Application(s.mysql.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application.MetricCredentials(), tc.DeepEquals, []byte("hello there"))
}

func (s *ApplicationSuite) TestMetricCredentialsOnDying(c *tc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.SetMetricCredentials([]byte("set before dying"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.SetMetricCredentials([]byte("set after dying"))
	c.Assert(err, tc.ErrorMatches, "cannot update metric credentials: application is not found or not alive")
}

const oneRequiredStorageMeta = `
storage:
  data0:
    type: block
`

const oneOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
`

const oneRequiredOneOptionalStorageMeta = `
storage:
  data0:
    type: block
  data1:
    type: block
    multiple:
      range: 0-
`

const twoRequiredStorageMeta = `
storage:
  data0:
    type: block
  data1:
    type: block
`

const twoOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
  data1:
    type: block
    multiple:
      range: 0-
`

const oneRequiredFilesystemStorageMeta = `
storage:
  data0:
    type: filesystem
`

const oneOptionalSharedStorageMeta = `
storage:
  data0:
    type: block
    shared: true
    multiple:
      range: 0-
`

const oneRequiredReadOnlyStorageMeta = `
storage:
  data0:
    type: block
    read-only: true
`

const oneRequiredLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
`

const oneMultipleLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
    multiple:
      range: 1-
`

func storageRange(min, max int) string {
	var minStr, maxStr string
	if min > 0 {
		minStr = fmt.Sprint(min)
	}
	if max > 0 {
		maxStr = fmt.Sprint(max)
	}
	return fmt.Sprintf(`
    multiple:
      range: %s-%s
`[1:], minStr, maxStr)
}

func (s *ApplicationSuite) setCharmFromMeta(c *tc.C, oldMeta, newMeta string) error {
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	return app.SetCharm(cfg)
}

func (s *ApplicationSuite) TestSetCharmOptionalUnusedStorageRemoved(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredOneOptionalStorageMeta,
		mysqlBaseMeta+oneRequiredStorageMeta,
	)
	c.Assert(err, tc.ErrorIsNil)
	// It's valid to remove optional storage so long
	// as it is not in use.
}

func (s *ApplicationSuite) TestSetCharmOptionalUsedStorageRemoved(c *tc.C) {
	oldMeta := mysqlBaseMeta + oneRequiredOneOptionalStorageMeta
	newMeta := mysqlBaseMeta + oneRequiredStorageMeta
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "test",
		Charm: oldCh,
		Storage: map[string]state.StorageConstraints{
			"data0": {Count: 1},
			"data1": {Count: 1},
		},
	})
	defer state.SetBeforeHooks(c, s.State, func() {
		// Adding a unit will cause the storage to be in-use.
		_, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": in-use storage "data1" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageRemoved(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": required storage "data0" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageAddedDefaultConstraints(c *tc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoRequiredStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the new required storage was added for the unit.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachments, tc.HasLen, 2)
}

func (s *ApplicationSuite) TestSetCharmStorageAddedUserSpecifiedConstraints(c *tc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoOptionalStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		StorageConstraints: map[string]state.StorageConstraints{
			"data1": {Count: 3},
		},
	}
	err = app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)

	// Check that new storage was added for the unit, based on the
	// constraints specified in SetCharmConfig.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachments, tc.HasLen, 4)
}

func (s *ApplicationSuite) TestSetCharmOptionalStorageAdded(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+twoOptionalStorageMeta,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinDecreased(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinIncreased(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
	)
	// User must increase the storage constraints from 1 to 2.
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": validating storage constraints: charm "mysql" store "data0": 2 instances required, 1 specified`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxDecreased(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 2),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 1),
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from 2 to 1`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxUnboundedToBounded(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, -1),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 999),
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from \<unbounded\> to 999`)
}

func (s *ApplicationSuite) TestSetCharmStorageTypeChanged(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" type changed from "block" to "filesystem"`)
}

func (s *ApplicationSuite) TestSetCharmStorageSharedChanged(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneOptionalStorageMeta,
		mysqlBaseMeta+oneOptionalSharedStorageMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" shared changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageReadOnlyChanged(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredReadOnlyStorageMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" read-only changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageLocationChanged(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" location changed from "" to "/srv"`)
}

func (s *ApplicationSuite) TestSetCharmStorageWithLocationSingletonToMultipleAdded(c *tc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
		mysqlBaseMeta+oneMultipleLocationStorageMeta,
	)
	c.Assert(err, tc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" with location changed from single to multiple`)
}

func (s *ApplicationSuite) assertApplicationRemovedWithItsBindings(c *tc.C, application *state.Application) {
	// Removing the application removes the bindings with it.
	err := application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsReturnsDefaultsWhenNotFound(c *tc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)
	state.RemoveEndpointBindingsForApplication(c, application)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
}

func (s *ApplicationSuite) assertApplicationHasOnlyDefaultEndpointBindings(c *tc.C, application *state.Application) {
	charm, _, err := application.Charm()
	c.Assert(err, tc.ErrorIsNil)

	knownEndpoints := set.NewStrings("")
	allBindings, err := state.DefaultEndpointBindingsForCharm(s.State, charm.Meta())
	c.Assert(err, tc.ErrorIsNil)
	for endpoint := range allBindings {
		knownEndpoints.Add(endpoint)
	}

	setBindings, err := application.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(setBindings.Map(), tc.NotNil)

	for endpoint, space := range setBindings.Map() {
		c.Check(knownEndpoints.Contains(endpoint), tc.IsTrue)
		c.Check(space, tc.Equals, network.AlphaSpaceId, tc.Commentf("expected default space for endpoint %q, got %q", endpoint, space))
	}
}

func (s *ApplicationSuite) TestEndpointBindingsJustDefaults(c *tc.C) {
	// With unspecified bindings, all endpoints are explicitly bound to the
	// default space when saved in state.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsWithExplictOverrides(c *tc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	haSpace, err := s.State.AddSpace("ha", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	bindings := map[string]string{
		"server":  dbSpace.Id(),
		"cluster": haSpace.Id(),
	}
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, bindings)

	setBindings, err := application.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(setBindings.Map(), tc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  dbSpace.Id(),
		"client":  network.AlphaSpaceId,
		"cluster": haSpace.Id(),
	})

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmExtraBindingsUseDefaults(c *tc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 42)
	oldBindings := map[string]string{
		"server": dbSpace.Id(),
		"kludge": dbSpace.Id(),
		"client": dbSpace.Id(),
	}
	application := s.AddTestingApplicationWithBindings(c, "yoursql", oldCharm, oldBindings)
	setBindings, err := application.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	effectiveOld := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  dbSpace.Id(),
		"kludge":  dbSpace.Id(),
		"client":  dbSpace.Id(),
		"cluster": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), tc.DeepEquals, effectiveOld)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)

	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err = application.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	setBindings, err = application.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	effectiveNew := map[string]string{
		"": network.AlphaSpaceId,
		// These three should be preserved from oldCharm.
		"client":  dbSpace.Id(),
		"server":  dbSpace.Id(),
		"cluster": network.AlphaSpaceId,
		// "kludge" is missing in newMeta
		// All the remaining are new and use the empty default.
		"foo":  network.AlphaSpaceId,
		"baz":  network.AlphaSpaceId,
		"just": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), tc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmHandlesMissingBindingsAsDefaults(c *tc.C) {
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 69)
	app := s.AddTestingApplicationWithBindings(c, "theirsql", oldCharm, nil)
	state.RemoveEndpointBindingsForApplication(c, app)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 70)

	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	setBindings, err := app.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	effectiveNew := map[string]string{
		// The following two exist for both oldCharm and newCharm.
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		// "kludge" is missing in newMeta, "server" is new and gets the default.
		"server": network.AlphaSpaceId,
		// All the remaining are new and use the empty default.
		"foo":  network.AlphaSpaceId,
		"baz":  network.AlphaSpaceId,
		"just": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), tc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, app)
}

func (s *ApplicationSuite) setupApplicationWithUnitsForUpgradeCharmScenario(c *tc.C, numOfUnits int) (deployedV int, err error) {
	originalCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgreplication
`
	originalCharm := s.AddMetaCharm(c, "mysql", originalCharmMeta, 2)
	cfg := state.SetCharmConfig{Charm: originalCharm, CharmOrigin: defaultCharmOrigin(originalCharm.URL())}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:replication")
	deployedV = s.mysql.CharmModifiedVersion()

	for i := 0; i < numOfUnits; i++ {
		_, err = s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
	}

	// New mysql charm renames peer relation.
	updatedCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgpeer
`
	updatedCharm := s.AddMetaCharm(c, "mysql", updatedCharmMeta, 3)

	cfg = state.SetCharmConfig{Charm: updatedCharm, CharmOrigin: defaultCharmOrigin(updatedCharm.URL())}
	err = s.mysql.SetCharm(cfg)
	return
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithOneUnit(c *tc.C) {
	obtainedV, err := s.setupApplicationWithUnitsForUpgradeCharmScenario(c, 1)

	// ensure upgrade happened
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV+1, tc.IsTrue)
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithMoreThanOneUnit(c *tc.C) {
	obtainedV, err := s.setupApplicationWithUnitsForUpgradeCharmScenario(c, 2)

	// ensure upgrade happened
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV+1, tc.IsTrue)
}

func (s *ApplicationSuite) TestWatchCharmConfig(c *tc.C) {
	oldCharm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", oldCharm)
	// Add a unit so when we change the application's charm,
	// the old charm isn't removed (due to a reference).
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u.SetCharmURL(oldCharm.URL())
	c.Assert(err, tc.ErrorIsNil)

	w, err := app.WatchCharmConfig()
	c.Assert(err, tc.ErrorIsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "superhero paparazzi"})
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", stringConfig, 123)
	err = app.SetCharm(state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application config for new charm; nothing detected.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

var updateApplicationConfigTests = []struct {
	about   string
	initial config.ConfigAttributes
	update  config.ConfigAttributes
	expect  config.ConfigAttributes
	err     string
}{{
	about:  "set string",
	update: config.ConfigAttributes{"outlook": "positive"},
	expect: config.ConfigAttributes{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: config.ConfigAttributes{"outlook": "positive"},
	update:  config.ConfigAttributes{"outlook": nil, "title": "sir"},
	expect:  config.ConfigAttributes{"title": "sir"},
}, {
	about:  "unset missing string",
	update: config.ConfigAttributes{"outlook": nil},
	expect: config.ConfigAttributes{},
}, {
	about:   `empty strings are valid`,
	initial: config.ConfigAttributes{"outlook": "positive"},
	update:  config.ConfigAttributes{"outlook": "", "title": ""},
	expect:  config.ConfigAttributes{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: config.ConfigAttributes{"title": "sir"},
	update:  config.ConfigAttributes{"username": "admin001"},
	expect:  config.ConfigAttributes{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: config.ConfigAttributes{"username": "admin001", "title": "sir"},
	update:  config.ConfigAttributes{"username": nil, "title": "My Title"},
	expect:  config.ConfigAttributes{"title": "My Title"},
}, {
	about:  "non-string type",
	update: config.ConfigAttributes{"skill-level": 303},
	expect: config.ConfigAttributes{"skill-level": 303},
}, {
	about:   "unset non-string type",
	initial: config.ConfigAttributes{"skill-level": 303},
	update:  config.ConfigAttributes{"skill-level": nil},
	expect:  config.ConfigAttributes{},
}}

func (s *ApplicationSuite) TestUpdateApplicationConfig(c *tc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range updateApplicationConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateApplicationConfig(t.initial, nil, sampleApplicationConfigSchema(), nil)
			c.Assert(err, tc.ErrorIsNil)
		}
		updates := make(map[string]interface{})
		var resets []string
		for k, v := range t.update {
			if v == nil {
				resets = append(resets, k)
			} else {
				updates[k] = v
			}
		}
		err := app.UpdateApplicationConfig(updates, resets, sampleApplicationConfigSchema(), nil)
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			cfg, err := app.ApplicationConfig()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(cfg, tc.DeepEquals, t.expect)
		}
		err = app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestApplicationConfigNotFoundNoError(c *tc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", ch)

	// Delete all the settings. We should get a nil return, but no error.
	_, _ = s.State.MongoSession().DB("juju").C("settings").RemoveAll(nil)

	cfg, err := app.ApplicationConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestStatusInitial(c *tc.C) {
	appStatus, err := s.mysql.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(appStatus.Status, tc.Equals, status.Unset)
	c.Check(appStatus.Message, tc.Equals, "")
	c.Check(appStatus.Data, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestUnitStatusesNoUnits(c *tc.C) {
	statuses, err := s.mysql.UnitStatuses()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statuses, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestUnitStatusesWithUnits(c *tc.C) {
	u1, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u1.SetStatus(status.StatusInfo{
		Status: status.Maintenance,
	})
	c.Assert(err, tc.ErrorIsNil)

	// If Agent status is in error, we see that.
	u2, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u2.Agent().SetStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "foo",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = u2.SetStatus(status.StatusInfo{
		Status: status.Blocked,
	})
	c.Assert(err, tc.ErrorIsNil)

	statuses, err := s.mysql.UnitStatuses()
	c.Check(err, tc.ErrorIsNil)

	check := tc.NewMultiChecker()
	check.AddExpr(`_[_].Since`, tc.Ignore)
	check.AddExpr(`_[_].Data`, tc.Ignore)
	c.Assert(statuses, check, map[string]status.StatusInfo{
		"mysql/0": {
			Status: status.Maintenance,
		},
		"mysql/1": {
			Status:  status.Error,
			Message: "foo",
		},
	})
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

func (s *ApplicationSuite) TestUpdateApplicationConfigWithDyingApplication(c *tc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.UpdateApplicationConfig(config.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyApplicationRemovesConfig(c *tc.C) {
	err := s.mysql.UpdateApplicationConfig(config.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, tc.ErrorIsNil)
	appConfig := state.GetApplicationConfig(s.State, s.mysql)
	err = appConfig.Read()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appConfig.Map(), tc.Not(tc.HasLen), 0)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, tc.ErrorIsNil)
	assertRemoved(c, s.mysql)
}

type CAASApplicationSuite struct {
	ConnSuite
	app    *state.Application
	caasSt *state.State
}

func TestCAASApplicationSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &CAASApplicationSuite{})
}

func (s *CAASApplicationSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.caasSt = s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { _ = s.caasSt.Close() })

	f := factory.NewFactory(s.caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	s.app = f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	// Consume the initial construction events from the watchers.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASApplicationSuite) TestUpdateCAASUnits(c *tc.C) {
	s.assertUpdateCAASUnits(c, true)
}

func (s *CAASApplicationSuite) TestUpdateCAASUnitsApplicationNotALive(c *tc.C) {
	s.assertUpdateCAASUnits(c, false)
}

func (s *CAASApplicationSuite) assertUpdateCAASUnits(c *tc.C, aliveApp bool) {
	existingUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("unit-uuid")})
	c.Assert(err, tc.ErrorIsNil)
	removedUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("removed-unit-uuid")})
	c.Assert(err, tc.ErrorIsNil)
	noContainerUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("never-cloud-container")})
	c.Assert(err, tc.ErrorIsNil)
	if !aliveApp {
		err := s.app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}

	var updateUnits state.UpdateUnitsOperation
	updateUnits.Deletes = []*state.DestroyUnitOperation{removedUnit.DestroyOperation()}
	updateUnits.Adds = []*state.AddUnitOperation{
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("new-unit-uuid"),
			Address:    strPtr("192.168.1.1"),
			Ports:      &[]string{"80"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new container running",
			},
		}),
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("add-never-cloud-container"),
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			// Status history should not show this as active.
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
	}
	updateUnits.Updates = []*state.UpdateUnitOperation{
		noContainerUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("never-cloud-container"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("unit-uuid"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing container running",
			},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	if !aliveApp {
		c.Assert(err, tc.Satisfies, state.IsNotAlive)
		return
	}
	c.Assert(err, tc.ErrorIsNil)

	units, err := s.app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 4)

	unitsById := make(map[string]*state.Unit)
	containerInfoById := make(map[string]state.CloudContainer)
	for _, u := range units {
		c.Assert(u.ShouldBeAssigned(), tc.IsFalse)
		containerInfo, err := u.ContainerInfo()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(containerInfo.Unit(), tc.Equals, u.Name())
		c.Assert(containerInfo.ProviderId(), tc.Not(tc.Equals), "")
		unitsById[containerInfo.ProviderId()] = u
		containerInfoById[containerInfo.ProviderId()] = containerInfo
	}
	u, ok := unitsById["unit-uuid"]
	c.Assert(ok, tc.IsTrue)
	info, ok := containerInfoById["unit-uuid"]
	c.Assert(ok, tc.IsTrue)
	c.Check(u.Name(), tc.Equals, existingUnit.Name())
	c.Check(info.Address(), tc.NotNil)
	c.Check(*info.Address(), tc.DeepEquals,
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeMachineLocal)))
	c.Check(info.Ports(), tc.DeepEquals, []string{"443"})
	statusInfo, err := u.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Running)
	c.Assert(statusInfo.Message, tc.Equals, "existing running")
	history, err := u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, tc.Equals, status.Running)
		c.Assert(history[1].Status, tc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, tc.Equals, status.Running)
		c.Assert(history[0].Status, tc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), tc.Equals, history[1].Since.Unix())
	}
	statusInfo, err = u.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, tc.Equals, "installing agent")
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Running)
	c.Assert(statusInfo.Message, tc.Equals, "existing container running")
	unitHistory, err := u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitHistory, tc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if unitHistory[0].Status == status.Running {
		c.Assert(unitHistory[0].Status, tc.Equals, status.Running)
		c.Assert(unitHistory[0].Message, tc.Equals, "existing container running")
		c.Assert(unitHistory[1].Status, tc.Equals, status.Waiting)
	} else {
		c.Assert(unitHistory[1].Status, tc.Equals, status.Running)
		c.Assert(unitHistory[1].Message, tc.Equals, "existing container running")
		c.Assert(unitHistory[0].Status, tc.Equals, status.Waiting)
		c.Assert(unitHistory[0].Since.Unix(), tc.Equals, history[1].Since.Unix())
	}

	u, ok = unitsById["never-cloud-container"]
	c.Assert(ok, tc.IsTrue)
	info, ok = containerInfoById["never-cloud-container"]
	c.Assert(ok, tc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, tc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, tc.Equals, status.MessageInstallingAgent)

	u, ok = unitsById["add-never-cloud-container"]
	c.Assert(ok, tc.IsTrue)
	info, ok = containerInfoById["add-never-cloud-container"]
	c.Assert(ok, tc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, tc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, tc.Equals, status.MessageInstallingAgent)

	u, ok = unitsById["new-unit-uuid"]
	c.Assert(ok, tc.IsTrue)
	info, ok = containerInfoById["new-unit-uuid"]
	c.Assert(ok, tc.IsTrue)
	c.Assert(u.Name(), tc.Equals, "gitlab/3")
	c.Check(info.Address(), tc.NotNil)
	c.Check(*info.Address(), tc.DeepEquals,
		network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal)))
	c.Assert(info.Ports(), tc.DeepEquals, []string{"80"})

	addr, err := u.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.DeepEquals, network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal)))

	statusInfo, err = u.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, tc.Equals, status.MessageInstallingAgent)
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Running)
	c.Assert(statusInfo.Message, tc.Equals, "new container running")
	statusInfo, err = u.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Running)
	c.Assert(statusInfo.Message, tc.Equals, "new running")
	history, err = u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, tc.Equals, status.Running)
		c.Assert(history[1].Status, tc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, tc.Equals, status.Running)
		c.Assert(history[0].Status, tc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), tc.Equals, history[1].Since.Unix())
	}
	// container status history must have overridden the unit status.
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitHistory, tc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if unitHistory[0].Status == status.Running {
		c.Assert(unitHistory[0].Status, tc.Equals, status.Running)
		c.Assert(unitHistory[0].Message, tc.Equals, "new container running")
		c.Assert(unitHistory[1].Status, tc.Equals, status.Waiting)
	} else {
		c.Assert(unitHistory[1].Status, tc.Equals, status.Running)
		c.Assert(unitHistory[1].Message, tc.Equals, "new container running")
		c.Assert(unitHistory[0].Status, tc.Equals, status.Waiting)
		c.Assert(unitHistory[0].Since.Unix(), tc.Equals, history[1].Since.Unix())
	}

	// check cloud container status history is stored.
	containerStatusHistory, err := state.GetCloudContainerStatusHistory(s.caasSt, u.Name(), status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containerStatusHistory, tc.HasLen, 1)
	c.Assert(containerStatusHistory[0].Status, tc.Equals, status.Running)
	c.Assert(containerStatusHistory[0].Message, tc.Equals, "new container running")

	err = removedUnit.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestAddUnitWithProviderId(c *tc.C) {
	u, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, tc.ErrorIsNil)
	info, err := u.ContainerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Unit(), tc.Equals, u.Name())
	c.Assert(info.ProviderId(), tc.Equals, "provider-id")
}

func (s *CAASApplicationSuite) TestServiceInfo(c *tc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("id", addrs)
		c.Assert(err, tc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, tc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(info.ProviderId(), tc.Equals, "id")
		c.Assert(info.Addresses(), tc.DeepEquals, addrs)
	}
}

func (s *CAASApplicationSuite) TestServiceInfoEmptyProviderId(c *tc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("", addrs)
		c.Assert(err, tc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, tc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(info.ProviderId(), tc.Equals, "")
		c.Assert(info.Addresses(), tc.DeepEquals, addrs)
	}
}

func (s *CAASApplicationSuite) TestRemoveApplicationDeletesServiceInfo(c *tc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	err := s.app.UpdateCloudService("id", addrs)
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	// Until cleanups run, no removal.
	si, err := s.app.ServiceInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si, tc.NotNil)
	assertCleanupCount(c, s.caasSt, 2)
	_, err = s.app.ServiceInfo()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestInvalidScale(c *tc.C) {
	err := s.app.SetScale(-1, 0, true)
	c.Assert(err, tc.ErrorMatches, "application scale -1 not valid")

	// set scale without force for caas workers - a new Generation is required.
	err = s.app.SetScale(3, 0, false)
	c.Assert(err, tc.Satisfies, errors.IsForbidden)
}

func (s *CAASApplicationSuite) TestSetScale(c *tc.C) {
	// set scale with force for CLI - DesiredScaleProtected set to true.
	err := s.app.SetScale(5, 0, true)
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.GetScale(), tc.Equals, 5)
	svcInfo, err := s.app.ServiceInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svcInfo.DesiredScaleProtected(), tc.IsTrue)

	// set scale without force for caas workers - a new Generation is required.
	err = s.app.SetScale(5, 1, false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.GetScale(), tc.Equals, 5)
	svcInfo, err = s.app.ServiceInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svcInfo.DesiredScaleProtected(), tc.IsFalse)
	c.Assert(svcInfo.Generation(), tc.DeepEquals, int64(1))
}

func (s *CAASApplicationSuite) TestInvalidChangeScale(c *tc.C) {
	newScale, err := s.app.ChangeScale(-1, []names.StorageTag{})
	c.Assert(err, tc.ErrorMatches, "cannot remove more units than currently exist not valid")
	c.Assert(newScale, tc.Equals, 0)
}

func (s *CAASApplicationSuite) TestChangeScale(c *tc.C) {
	newScale, err := s.app.ChangeScale(5, []names.StorageTag{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 5)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.GetScale(), tc.Equals, 5)

	newScale, err = s.app.ChangeScale(-4, []names.StorageTag{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 1)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.GetScale(), tc.Equals, 1)
}

func (s *CAASApplicationSuite) TestChangeScaleAttachStorage(c *tc.C) {
	ch, sb, st := s.setupCharmWithNewStorageBackend(c)
	storageTags := s.addExistingFilesystems(c, sb, 3, "database")

	f := factory.NewFactory(st, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
	err := app.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	newScale, err := app.ChangeScale(1, []names.StorageTag{storageTags[0]})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 1)

	newScale, err = app.ChangeScale(1, []names.StorageTag{storageTags[1]})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 2)

	newScale, err = app.ChangeScale(1, []names.StorageTag{storageTags[2]})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newScale, tc.Equals, 3)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	for i, unit := range units {
		attachments, err := sb.UnitStorageAttachments(unit.UnitTag())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(attachments[0].Unit(), tc.Equals,
			names.NewUnitTag(fmt.Sprintf("cockroachdb/%d", i)),
		)
		c.Assert(attachments[0].StorageInstance(), tc.Equals, storageTags[i])
	}
}

func (s *CAASApplicationSuite) TestWatchScale(c *tc.C) {
	// Empty initial event.
	w := s.app.WatchScale()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.app.SetScale(5, 0, true)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Set to same value, no change.
	err = s.app.SetScale(5, 0, true)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.SetScale(6, 0, true)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// An unrelated update, no change.
	err = s.app.SetMinUnits(2)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *CAASApplicationSuite) TestWatchCloudService(c *tc.C) {
	cloudSvc, err := s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id: s.app.Name(),
	})
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := cloudSvc.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	_, err = s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.app.Name(),
		ProviderId: "123",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove service by removing app, start new watch, check single event.
	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = cloudSvc.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *CAASApplicationSuite) TestRewriteStatusHistory(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	history, err := app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 1)
	c.Assert(history[0].Status, tc.Equals, status.Unset)
	c.Assert(history[0].Message, tc.Equals, "")

	// Must overwrite the history
	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Allocating,
		Message: "operator message",
	})
	c.Assert(err, tc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 2)
	c.Assert(history[0].Status, tc.Equals, status.Allocating)
	c.Assert(history[0].Message, tc.Equals, "operator message")
	c.Assert(history[1].Status, tc.Equals, status.Unset)
	c.Assert(history[1].Message, tc.Equals, "")

	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Running,
		Message: "operator running",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = app.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "app active",
	})
	c.Assert(err, tc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Log(history)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(history, tc.HasLen, 3)
	c.Assert(history[0].Status, tc.Equals, status.Active)
	c.Assert(history[0].Message, tc.Equals, "app active")
	c.Assert(history[1].Status, tc.Equals, status.Allocating)
	c.Assert(history[1].Message, tc.Equals, "operator message")
	c.Assert(history[2].Status, tc.Equals, status.Unset)
	c.Assert(history[2].Message, tc.Equals, "")
}

func (s *CAASApplicationSuite) TestClearResources(c *tc.C) {
	c.Assert(state.GetApplicationHasResources(s.app), tc.IsTrue)
	err := s.app.ClearResources()
	c.Assert(err, tc.ErrorMatches, `application "gitlab" is alive`)
	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)

	// ClearResources should be idempotent.
	for i := 0; i < 2; i++ {
		err := s.app.ClearResources()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(state.GetApplicationHasResources(s.app), tc.IsFalse)
	}
	// Resetting the app's HasResources the first time schedules a cleanup.
	assertCleanupCount(c, s.caasSt, 2)
}

func (s *CAASApplicationSuite) TestDestroySimple(c *tc.C) {
	err := s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	c.Assert(s.app.Life(), tc.Equals, state.Dead)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state.GetApplicationHasResources(s.app), tc.IsTrue)
}

func (s *CAASApplicationSuite) TestForceDestroyQueuesForceCleanup(c *tc.C) {
	op := s.app.DestroyOperation()
	op.Force = true
	err := s.caasSt.ApplyOperation(op)
	c.Assert(err, tc.ErrorIsNil)

	// Cleanup queued but won't run until scheduled.
	assertNeedsCleanup(c, s.caasSt)
	s.Clock.Advance(2 * time.Minute)
	assertCleanupRuns(c, s.caasSt)

	err = s.app.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyStillHasUnits(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.Life(), tc.Equals, state.Dying)

	c.Assert(unit.EnsureDead(), tc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)

	c.Assert(unit.Remove(), tc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyOnceHadUnits(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.Life(), tc.Equals, state.Dead)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyStaleNonZeroUnitCount(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.Life(), tc.Equals, state.Dead)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyStaleZeroUnitCount(c *tc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.app.Life(), tc.Equals, state.Dying)
	assertLife(c, s.app, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)

	c.Assert(unit.Remove(), tc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	c.Assert(err, tc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyWithRemovableRelation(c *tc.C) {
	ch := state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "mysql")
	mysql := state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "mysql", ch)
	eps, err := s.caasSt.InferEndpoints("gitlab", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Destroy a application with no units in relation scope; check application and
	// unit removed.
	err = mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = mysql.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, mysql, state.Dead)

	err = rel.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyWithReferencedRelation(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *CAASApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *CAASApplicationSuite) assertDestroyWithReferencedRelation(c *tc.C, refresh bool) {
	ch := state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "mysql")
	mysql := state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "mysql", ch)
	eps, err := s.caasSt.InferEndpoints("gitlab", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel0, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	ch = state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "proxy")
	state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "proxy", ch)
	eps, err = s.caasSt.InferEndpoints("proxy", "gitlab")
	c.Assert(err, tc.ErrorIsNil)
	rel1, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.app.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	c.Assert(s.app.Destroy(), tc.ErrorIsNil)
	assertLife(c, rel0, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are are both removed.
	c.Assert(ru.LeaveScope(), tc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)

	err = rel0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyQueuesUnitCleanup(c *tc.C) {
	// Add 5 units; block quick-remove of gitlab/1 and gitlab/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			unitState := state.NewUnitState()
			unitState.SetUniterState("idle")
			err := unit.SetState(unitState, state.UnitStateSizeLimits{})
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	assertDoesNotNeedCleanup(c, s.caasSt)

	// Destroy gitlab, and check units are not touched.
	err := s.app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	dirty, err := s.caasSt.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dirty, tc.IsTrue)
	assertCleanupCount(c, s.caasSt, 2)

	for i, unit := range units {
		if i%2 != 0 {
			assertLife(c, unit, state.Dying)
		} else {
			assertRemoved(c, unit)
		}
	}

	// App dying until units are gone.
	assertLife(c, s.app, state.Dying)
}

func (s *CAASApplicationSuite) TestGetUnitAttachmentInfosWithoutAttachStorage(c *tc.C) {
	app := s.setupApplicationWithAttachStorage(c, 2, []names.StorageTag{})
	infos, err := app.GetUnitAttachmentInfos()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 0)
}

func (s *CAASApplicationSuite) TestGetUnitAttachmentInfosWithAttachStorage(c *tc.C) {
	app := s.setupApplicationWithAttachStorage(c, 1, []names.StorageTag{names.NewStorageTag("database/0")})
	infos, err := app.GetUnitAttachmentInfos()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.DeepEquals, []state.UnitAttachmentInfo{{Unit: "cockroachdb/0", VolumeId: "pv-database-0", StorageId: "database/0"}})
}

func (s *CAASApplicationSuite) addExistingFilesystems(c *tc.C, sb *state.StorageBackend, num int, storageName string) []names.StorageTag {
	storageTags := make([]names.StorageTag, num+1)
	for i := 0; i < num; i++ {
		fsInfo := state.FilesystemInfo{
			Size: 100,
			Pool: "kubernetes",
		}
		volumeInfo := state.VolumeInfo{
			VolumeId:   fmt.Sprintf("pv-%s-%d", storageName, i),
			Size:       100,
			Pool:       "kubernetes",
			Persistent: true,
		}
		storageTag, err := sb.AddExistingFilesystem(fsInfo, &volumeInfo, storageName)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(storageTag.Id(), tc.Equals, fmt.Sprintf("database/%d", i))
		storageTags[i] = storageTag
	}
	return storageTags
}

func (s *CAASApplicationSuite) setupCharmWithNewStorageBackend(c *tc.C) (*state.Charm, *state.StorageBackend, *state.State) {
	registry := &storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummy.StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
		},
	}

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName: "caascloud",
	})
	s.AddCleanup(func(_ *tc.C) { _ = st.Close() })

	pm := poolmanager.New(state.NewStateSettings(st), registry)
	_, err := pm.Create("kubernetes", "kubernetes", map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)
	ch := state.AddTestingCharmForSeries(c, st, "quantal", "cockroachdb")
	return ch, sb, st
}

func (s *CAASApplicationSuite) setupApplicationWithAttachStorage(c *tc.C, unitNum int, attachStorage []names.StorageTag) *state.Application {
	ch, sb, st := s.setupCharmWithNewStorageBackend(c)
	s.addExistingFilesystems(c, sb, 3, "database")

	cockroachdb := state.AddTestingApplicationWithAttachStorage(c, st, "cockroachdb", ch, unitNum,
		map[string]state.StorageConstraints{
			"database": {
				Pool:  "kubernetes",
				Size:  100,
				Count: 0,
			},
		},
		attachStorage,
	)
	units, err := cockroachdb.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, unitNum)
	return cockroachdb
}

func (s *ApplicationSuite) TestSetOperatorStatusNonCAAS(c *tc.C) {
	_, err := state.ApplicationOperatorStatus(s.State, s.mysql.Name())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetOperatorStatus(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "broken",
		Since:   &now,
	}
	err := app.SetOperatorStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	appStatus, err := state.ApplicationOperatorStatus(st, app.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appStatus.Status, tc.DeepEquals, status.Error)
	c.Assert(appStatus.Message, tc.DeepEquals, "broken")
}

func (s *ApplicationSuite) TestCharmLegacyOnlySupportsOneSeries(c *tc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "precise", "mysql")
	app := s.AddTestingApplication(c, "legacy-charm", ch)
	err := app.VerifySupportedBase(state.UbuntuBase("12.10"))
	c.Assert(err, tc.ErrorIsNil)
	err = app.VerifySupportedBase(state.UbuntuBase("16.04"))
	c.Assert(err, tc.ErrorMatches, "base \"ubuntu@16.04\" not supported by charm, the charm supported bases are: ubuntu@12.10")
}

func (s *ApplicationSuite) TestCharmLegacyNoOSInvalid(c *tc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "precise", "sample-fail-no-os")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "sample-fail-no-os",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			},
		},
	})
	c.Assert(err, tc.ErrorMatches, `.*charm does not define any bases`)
}

func (s *ApplicationSuite) TestDeployedMachines(c *tc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "riak"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	machines, err := app.DeployedMachines()

	c.Assert(err, tc.ErrorIsNil)
	var ids []string
	for _, m := range machines {
		ids = append(ids, m.Id())
	}
	c.Assert(ids, tc.SameContents, []string{"0"})
}

func (s *ApplicationSuite) TestDeployedMachinesNotAssignedUnit(c *tc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "riak"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit.AssignedMachineId()
	c.Assert(err, tc.Satisfies, errors.IsNotAssigned)

	machines, err := app.DeployedMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 0)
}

func (s *ApplicationSuite) TestCAASSidecarCharm(c *tc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	sidecar, err := unit.IsSidecar()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sidecar, tc.IsTrue)
}

func (s *ApplicationSuite) addCAASSidecarApplication(c *tc.C) (*state.State, *state.Application) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	f := factory.NewFactory(st, s.StatePool)

	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
provides:
  data-port:
    interface: data
    scope: container
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	return st, f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
}

func (s *ApplicationSuite) TestCAASNonSidecarCharm(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)

	charmDef := `
name: mysql
description: foo
summary: foo
series:
  - kubernetes
deployment:
  mode: workload
`
	ch := state.AddCustomCharmForSeries(c, st, "mysql", "metadata.yaml", charmDef, "kubernetes", 1)
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch})

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	sidecar, err := unit.IsSidecar()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sidecar, tc.IsFalse)
}

func (s *ApplicationSuite) TestWatchApplicationsWithPendingCharms(c *tc.C) {
	w := s.State.WatchApplicationsWithPendingCharms()
	defer func() { _ = w.Stop() }()

	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange() // consume initial change set.

	// Add a pending charm with an origin and associate it with the
	// application. This should trigger a change.
	dummy2 := s.dummyCharm(c, "ch:dummy-1")
	dummy2.SHA256 = ""      // indicates that we don't have the data in the blobstore yet.
	dummy2.StoragePath = "" // indicates that we don't have the data in the blobstore yet.
	ch2, err := s.State.AddCharmMetadata(dummy2)
	c.Assert(err, tc.ErrorIsNil)
	twoOrigin := defaultCharmOrigin(ch2.URL())
	twoOrigin.Platform.OS = "ubuntu"
	twoOrigin.Platform.Channel = "22.04/stable"
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       ch2,
		CharmOrigin: twoOrigin,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(s.mysql.Name())

	// "Upload" a charm and check that we don't get a notification for it.
	dummy3 := s.dummyCharm(c, "ch:dummy-2")
	ch3, err := s.State.AddCharm(dummy3)
	c.Assert(err, tc.ErrorIsNil)
	threeOrigin := defaultCharmOrigin(ch3.URL())
	threeOrigin.Platform.OS = "ubuntu"
	threeOrigin.Platform.Channel = "22.04/stable"
	threeOrigin.ID = "charm-hub-id"
	threeOrigin.Hash = "charm-hub-hash"
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       ch3,
		CharmOrigin: threeOrigin,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	origin := &state.CharmOrigin{
		Source: "charm-hub",
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		},
	}
	// Simulate a bundle deploying multiple applications from a single
	// charm. The watcher needs to notify on the secondary applications.
	appSameCharm, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        "mysql-testing",
		Charm:       ch3,
		CharmOrigin: origin,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(appSameCharm.Name())
	origin.ID = "charm-hub-id"
	origin.Hash = "charm-hub-hash"
	_ = appSameCharm.SetCharm(state.SetCharmConfig{
		Charm:       ch3,
		CharmOrigin: origin,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationSuite) dummyCharm(c *tc.C, curlOverride string) state.CharmInfo {
	info := state.CharmInfo{
		Charm:       testcharms.Repo.CharmDir("dummy"),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
		Version:     "dummy-146-g725cfd3-dirty",
	}
	if curlOverride != "" {
		info.ID = curlOverride
	} else {
		info.ID = fmt.Sprintf("local:quantal/%s-%d", info.Charm.Meta().Name, info.Charm.Revision())
	}
	info.Charm.Meta().Series = []string{"quantal", "jammy"}
	return info
}

func (s *ApplicationSuite) TestWatch(c *tc.C) {
	w := s.mysql.WatchConfigSettingsHash()
	defer testing.AssertStop(c, w)

	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("1e11259677ef769e0ec4076b873c76dcc3a54be7bc651b081d0f0e2b87077717")

	schema := environschema.Fields{
		"username":    environschema.Attr{Type: environschema.Tstring},
		"alive":       environschema.Attr{Type: environschema.Tbool},
		"skill-level": environschema.Attr{Type: environschema.Tint},
		"options":     environschema.Attr{Type: environschema.Tattrs},
	}

	err := s.mysql.UpdateApplicationConfig(config.ConfigAttributes{
		"username":    "abbas",
		"alive":       true,
		"skill-level": 23,
		"options": map[string]string{
			"fortuna": "crescis",
			"luna":    "velut",
			"status":  "malus",
		},
	}, nil, schema, nil)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange("e1471e8a7299da0ac2150445ffc6d08d9d801194037d88416c54b01899b8a9b2")
}

func (s *ApplicationSuite) TestProvisioningState(c *tc.C) {
	ps := s.mysql.ProvisioningState()
	c.Assert(ps, tc.IsNil)

	err := s.mysql.SetProvisioningState(state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Assert(errors.Is(err, stateerrors.ProvisioningStateInconsistent), tc.IsTrue)

	err = s.mysql.SetScale(10, 0, true)
	c.Assert(err, tc.ErrorIsNil)

	err = s.mysql.SetProvisioningState(state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Assert(err, tc.ErrorIsNil)

	ps = s.mysql.ProvisioningState()
	c.Assert(ps, tc.DeepEquals, &state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
}

func (s *CAASApplicationSuite) TestUpsertCAASUnit(c *tc.C) {
	registry := &storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummy.StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
		},
	}

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName: "caascloud",
	})
	s.AddCleanup(func(_ *tc.C) { _ = st.Close() })

	pm := poolmanager.New(state.NewStateSettings(st), registry)
	_, err := pm.Create("kubernetes", "kubernetes", map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)

	fsInfo := state.FilesystemInfo{
		Size: 100,
		Pool: "kubernetes",
	}
	volumeInfo := state.VolumeInfo{
		VolumeId:   "pv-database-0",
		Size:       100,
		Pool:       "kubernetes",
		Persistent: true,
	}
	storageTag, err := sb.AddExistingFilesystem(fsInfo, &volumeInfo, "database")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag.Id(), tc.Equals, "database/0")

	ch := state.AddTestingCharmForSeries(c, st, "quantal", "cockroachdb")
	cockroachdb := state.AddTestingApplicationWithStorage(c, st, "cockroachdb", ch, map[string]state.StorageConstraints{
		"database": {
			Pool:  "kubernetes",
			Size:  100,
			Count: 0,
		},
	})

	unitName := "cockroachdb/0"
	providerId := "cockroachdb-0"
	address := "1.2.3.4"
	ports := []string{"80", "443"}

	// output of utils.AgentPasswordHash("juju")
	passwordHash := "v+jK3ht5NEdKeoQBfyxmlYe0"

	p := state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			UnitName:     &unitName,
			ProviderId:   &providerId,
			Address:      &address,
			Ports:        &ports,
			PasswordHash: &passwordHash,
		},
		OrderedScale:              true,
		OrderedId:                 0,
		ObservedAttachedVolumeIDs: []string{"pv-database-0"},
	}
	unit, err := cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, tc.ErrorMatches, `unrequired unit cockroachdb/0 is not assigned`)
	c.Assert(unit, tc.IsNil)

	err = cockroachdb.SetScale(1, 0, true)
	c.Assert(err, tc.ErrorIsNil)

	unit, err = cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit, tc.NotNil)
	c.Assert(unit.UnitTag().Id(), tc.Equals, "cockroachdb/0")
	c.Assert(unit.Life(), tc.Equals, state.Alive)
	containerInfo, err := unit.ContainerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containerInfo.ProviderId(), tc.Equals, "cockroachdb-0")
	c.Assert(containerInfo.Ports(), tc.SameContents, []string{"80", "443"})
	c.Assert(containerInfo.Address().Value, tc.Equals, "1.2.3.4")

	err = unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = sb.DetachStorage(storageTag, unit.UnitTag(), false, 0)
	c.Assert(err, tc.ErrorIsNil)

	err = sb.DetachFilesystem(unit.UnitTag(), names.NewFilesystemTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveFilesystemAttachment(unit.UnitTag(), names.NewFilesystemTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	err = sb.DetachVolume(unit.Tag(), names.NewVolumeTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(unit.Tag(), names.NewVolumeTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	unit2, err := cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, tc.ErrorMatches, `dead unit "cockroachdb/0" already exists`)
	c.Assert(unit2, tc.IsNil)

	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	err = st.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)

	unit, err = cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit, tc.NotNil)
	c.Assert(unit.UnitTag().Id(), tc.Equals, "cockroachdb/0")
	c.Assert(unit.Life(), tc.Equals, state.Alive)
	containerInfo, err = unit.ContainerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containerInfo.ProviderId(), tc.Equals, "cockroachdb-0")
	c.Assert(containerInfo.Ports(), tc.SameContents, []string{"80", "443"})
	c.Assert(containerInfo.Address().Value, tc.Equals, "1.2.3.4")
}

func intPtr(val int) *int {
	return &val
}

func defaultCharmOrigin(curlStr string) *state.CharmOrigin {
	// Use ParseURL here in test until either the charm and/or application
	// can easily provide the same data.
	curl, _ := charm.ParseURL(curlStr)
	var source string
	var channel *state.Channel
	if charm.CharmHub.Matches(curl.Schema) {
		source = corecharm.CharmHub.String()
		channel = &state.Channel{
			Risk: "stable",
		}
	} else if charm.Local.Matches(curl.Schema) {
		source = corecharm.Local.String()
	}

	base, _ := corebase.GetBaseFromSeries(curl.Series)

	platform := &state.Platform{
		Architecture: corearch.DefaultArchitecture,
		OS:           base.OS,
		Channel:      base.Channel.String(),
	}

	return &state.CharmOrigin{
		Source:   source,
		Type:     "charm",
		Revision: intPtr(curl.Revision),
		Channel:  channel,
		Platform: platform,
	}
}
