// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachetest

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

// ModelChangeFromState returns a ModelChange representing the current
// model for the state object.
func ModelChangeFromState(c *tc.C, st *state.State) cache.ModelChange {
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	return ModelChange(c, m)
}

// ModelChange returns a ModelChange representing the input state model.
func ModelChange(c *tc.C, model *state.Model) cache.ModelChange {
	cfg, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)

	status, err := model.Status()
	c.Assert(err, tc.ErrorIsNil)

	users, err := model.Users()
	c.Assert(err, tc.ErrorIsNil)
	permissions := make(map[string]permission.Access)
	for _, user := range users {
		// Cache permission map is always lower case.
		permissions[strings.ToLower(user.UserName)] = user.Access
	}

	return cache.ModelChange{
		ModelUUID:       model.UUID(),
		Name:            model.Name(),
		Life:            life.Value(model.Life().String()),
		Owner:           model.Owner().Name(),
		IsController:    model.IsControllerModel(),
		Config:          cfg.AllAttrs(),
		Status:          status,
		UserPermissions: permissions,
	}
}

// CharmChange returns a CharmChange representing the input state charm.
func CharmChange(modelUUID string, ch *state.Charm) cache.CharmChange {
	prof := ch.LXDProfile()
	cProf := lxdprofile.Profile{
		Config:      prof.Config,
		Description: prof.Description,
		Devices:     prof.Devices,
	}

	return cache.CharmChange{
		ModelUUID:     modelUUID,
		CharmURL:      ch.URL(),
		CharmVersion:  ch.Version(),
		LXDProfile:    cProf,
		DefaultConfig: ch.Config().DefaultSettings(),
	}
}

// ApplicationChange returns an ApplicationChange
// representing the input state application.
func ApplicationChange(c *tc.C, modelUUID string, app *state.Application) cache.ApplicationChange {
	// Note that this will include charm defaults as if explicitly set.
	// If this matters for tests, we will have to pass a state and attempt
	// to access the settings document for this application charm config.
	config, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)

	cons, err := app.Constraints()
	c.Assert(err, tc.ErrorIsNil)

	sts, err := app.Status()
	c.Assert(err, tc.ErrorIsNil)

	cURL, _ := app.CharmURL()

	return cache.ApplicationChange{
		ModelUUID:   modelUUID,
		Name:        app.Name(),
		Exposed:     app.IsExposed(),
		CharmURL:    *cURL,
		Life:        life.Value(app.Life().String()),
		MinUnits:    app.MinUnits(),
		Constraints: cons,
		Config:      config,
		Status:      sts,
		// TODO: Subordinate, WorkloadVersion.
	}
}

func MachineChange(c *tc.C, modelUUID string, machine *state.Machine) cache.MachineChange {
	iid, err := machine.InstanceId()
	c.Assert(err, tc.ErrorIsNil)

	aSts, err := machine.Status()
	c.Assert(err, tc.ErrorIsNil)

	iSts, err := machine.InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)

	hwc, err := machine.HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)

	chProf, err := machine.CharmProfiles()
	c.Assert(err, tc.ErrorIsNil)

	isManual, err := machine.IsManual()
	c.Assert(err, tc.ErrorIsNil)

	sc, scKnown := machine.SupportedContainers()

	return cache.MachineChange{
		ModelUUID:                modelUUID,
		Id:                       machine.Id(),
		InstanceId:               string(iid),
		AgentStatus:              aSts,
		InstanceStatus:           iSts,
		Life:                     life.Value(machine.Life().String()),
		Base:                     machine.Base().String(),
		ContainerType:            string(machine.ContainerType()),
		IsManual:                 isManual,
		SupportedContainers:      sc,
		SupportedContainersKnown: scKnown,
		HardwareCharacteristics:  hwc,
		CharmProfiles:            chProf,
		HasVote:                  true,
		WantsVote:                true,
		// TODO: Config, Addresses.
	}

}

// UnitChange returns a UnitChange representing the input state unit.
func UnitChange(c *tc.C, modelUUID string, unit *state.Unit) cache.UnitChange {
	// If these addresses are not set in state, we simply eschew setting them
	// in the cache rather than propagating such errors.
	publicAddr, err := unit.PublicAddress()
	if !network.IsNoAddressError(err) {
		c.Assert(err, tc.ErrorIsNil)
	}
	privateAddr, err := unit.PrivateAddress()
	if !network.IsNoAddressError(err) {
		c.Assert(err, tc.ErrorIsNil)
	}

	machineId, err := unit.AssignedMachineId()
	if !errors.IsNotAssigned(err) {
		c.Assert(err, tc.ErrorIsNil)
	}

	var charmURL string
	if cURL := unit.CharmURL(); cURL != nil {
		charmURL = *cURL
	}

	pr, err := unit.OpenedPortRanges()
	if !errors.IsNotAssigned(err) {
		c.Assert(err, tc.ErrorIsNil)
	}

	sts, err := unit.Status()
	c.Assert(err, tc.ErrorIsNil)

	aSts, err := unit.AgentStatus()
	c.Assert(err, tc.ErrorIsNil)

	principal, _ := unit.PrincipalName()

	base, err := corebase.ParseBase(unit.Base().OS, unit.Base().Channel)
	c.Assert(err, tc.ErrorIsNil)
	return cache.UnitChange{
		ModelUUID:                modelUUID,
		Name:                     unit.Name(),
		Application:              unit.ApplicationName(),
		Base:                     base.String(),
		CharmURL:                 charmURL,
		Life:                     life.Value(unit.Life().String()),
		PublicAddress:            publicAddr.String(),
		PrivateAddress:           privateAddr.String(),
		MachineId:                machineId,
		OpenPortRangesByEndpoint: pr.ByEndpoint(),
		Principal:                principal,
		WorkloadStatus:           sts,
		AgentStatus:              aSts,
		// TODO: Subordinate
	}
}
