// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/facades/client/machinemanager/mocks"
	"github.com/juju/juju/charmhub/transport"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type UpgradeSeriesSuiteValidate struct {
	testhelpers.IsolationSuite
}

func TestUpgradeSeriesSuiteValidate(t *tctesting.T) {
	tc.Run(t, &UpgradeSeriesSuiteValidate{})
}

func (s *UpgradeSeriesSuiteValidate) TestValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	applications := []machinemanager.Application{application}

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("foo/0"))
	units := []machinemanager.Unit{unit}

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false).Return(nil)
	validator.EXPECT().ValidateMachine(machine).Return(nil)
	validator.EXPECT().ValidateUnits(units).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []machinemanager.ValidationEntity{
		{Tag: "machine-0", Channel: "20.04"},
	}

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []machinemanager.ValidationResult{
		{UnitNames: []string{"foo/0"}},
	})
}

func (s *UpgradeSeriesSuiteValidate) TestValidateWithValidateBase(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0").Return(errors.New("boom"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []machinemanager.ValidationEntity{
		{Tag: "machine-0", Channel: "20.04"},
	}

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result[0].Error, tc.ErrorMatches, `boom`)
}

func (s *UpgradeSeriesSuiteValidate) TestValidateApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	applications := []machinemanager.Application{application}

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false).Return(errors.New("boom"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []machinemanager.ValidationEntity{
		{Tag: "machine-0", Channel: "20.04"},
	}

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result[0].Error, tc.ErrorMatches, `boom`)
}

type UpgradeSeriesSuitePrepare struct {
	testhelpers.IsolationSuite
}

func TestUpgradeSeriesSuitePrepare(t *tctesting.T) {
	tc.Run(t, &UpgradeSeriesSuitePrepare{})
}

func (s UpgradeSeriesSuitePrepare) TestPrepare(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	applications := []machinemanager.Application{application}

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []machinemanager.Unit{unit}

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, state.Base{OS: "ubuntu", Channel: "20.04"})
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, `started upgrade from "ubuntu@18.04" to "ubuntu@20.04"`)

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0")
	validator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "20.04", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareWithRollback(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	applications := []machinemanager.Application{application}

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []machinemanager.Unit{unit}

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, state.Base{OS: "ubuntu", Channel: "20.04"})
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, `started upgrade from "ubuntu@18.04" to "ubuntu@20.04"`).Return(errors.New("bad"))
	machine.EXPECT().RemoveUpgradeSeriesLock().Return(nil)

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "20.04", false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareWithRollbackError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	applications := []machinemanager.Application{application}

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []machinemanager.Unit{unit}

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, state.Base{OS: "ubuntu", Channel: "20.04"})
	machine.EXPECT().Base().Return(state.UbuntuBase("18.04")).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, `started upgrade from "ubuntu@18.04" to "ubuntu@20.04"`).Return(errors.New("bad"))
	machine.EXPECT().RemoveUpgradeSeriesLock().Return(errors.New("boom"))

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateBase(corebase.MakeDefaultBase("ubuntu", "20.04"),
		corebase.MakeDefaultBase("ubuntu", "18.04"), "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := mocks.NewMockAuthorizer(ctrl)

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "20.04", false)
	c.Assert(err, tc.ErrorMatches, `boom occurred while cleaning up from: bad`)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareValidationFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Base().Return(state.UbuntuBase("20.04"))

	state := mocks.NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := mocks.NewMockUpgradeBaseValidator(ctrl)
	validator.EXPECT().ValidateMachine(machine).Return(errors.New("bad"))

	authorizer := mocks.NewMockAuthorizer(ctrl)

	api := machinemanager.NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "20.04", false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

type ValidatorSuite struct {
	testhelpers.IsolationSuite
}

func TestValidatorSuite(t *tctesting.T) {
	tc.Run(t, &ValidatorSuite{})
}

func (s ValidatorSuite) TestValidateApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	localApp := mocks.NewMockApplication(ctrl)
	localApp.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source: corecharm.Local.String(),
	})
	charmhubApp := mocks.NewMockApplication(ctrl)
	charmhubApp.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source: corecharm.CharmHub.String(),
	})
	applications := []machinemanager.Application{
		localApp,
		charmhubApp,
	}

	localValidator := mocks.NewMockUpgradeBaseValidator(ctrl)
	localValidator.EXPECT().ValidateApplications([]machinemanager.Application{localApp}, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	remoteValidator := mocks.NewMockUpgradeBaseValidator(ctrl)
	remoteValidator.EXPECT().ValidateApplications([]machinemanager.Application{charmhubApp}, corebase.MakeDefaultBase("ubuntu", "20.04"), false)

	validator := machinemanager.NewTestUpgradeSeriesValidator(localValidator, remoteValidator)

	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateApplicationsWithNoOrigin(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(nil)
	applications := []machinemanager.Application{application}

	localValidator := mocks.NewMockUpgradeBaseValidator(ctrl)
	localValidator.EXPECT().ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	remoteValidator := mocks.NewMockUpgradeBaseValidator(ctrl)
	remoteValidator.EXPECT().ValidateApplications([]machinemanager.Application(nil), corebase.MakeDefaultBase("ubuntu", "20.04"), false)

	validator := machinemanager.NewTestUpgradeSeriesValidator(localValidator, remoteValidator)

	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateMachineIsManager(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().IsManager().Return(true)

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateMachine(machine)
	c.Assert(err, tc.ErrorMatches, `machine-0 is a controller and cannot be targeted for series upgrade`)
}

func (s ValidatorSuite) TestValidateMachineIsLockedForSeriesUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Id().Return("0")
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(true, nil)
	machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareRunning, nil)

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateMachine(machine)
	c.Assert(err, tc.ErrorMatches, `upgrade series lock found for "0"; series upgrade is in the "prepare running" state`)
}

func (s ValidatorSuite) TestValidateUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Idle}, nil)
	unit.EXPECT().Status().Return(status.StatusInfo{Status: status.Active}, nil)
	units := []machinemanager.Unit{unit}

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateUnits(units)
	c.Assert(err, tc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateUnitsNotIdle(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().Name().Return("foo/0")
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Blocked}, nil)
	units := []machinemanager.Unit{unit}

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateUnits(units)
	c.Assert(err, tc.ErrorMatches, `unit foo/0 is not ready to start a series upgrade; its agent status is: "blocked" `)
}

func (s ValidatorSuite) TestValidateUnitsInErrorState(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().Name().Return("foo/0")
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Idle}, nil)
	unit.EXPECT().Status().Return(status.StatusInfo{Status: status.Error}, nil)
	units := []machinemanager.Unit{unit}

	validator := machinemanager.NewTestUpgradeSeriesValidator(nil, nil)

	err := validator.ValidateUnits(units)
	c.Assert(err, tc.ErrorMatches, `unit foo/0 is not ready to start a series upgrade; its status is: "error" `)
}

type StateValidatorSuite struct {
	testhelpers.IsolationSuite
}

func TestStateValidatorSuite(t *tctesting.T) {
	tc.Run(t, &StateValidatorSuite{})
}

func (s StateValidatorSuite) TestValidateApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := mocks.NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"focal", "bionic"}}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestStateSeriesValidator()
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationsWithNoBases(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := mocks.NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name: "my-charm",
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestStateSeriesValidator()
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `charm "my-charm" does not support any bases. Not valid`)
}

func (s StateValidatorSuite) TestValidateApplicationsWithUnsupportedSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := mocks.NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name:   "my-charm",
		Series: []string{"xenial", "bionic"},
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestStateSeriesValidator()
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `base "ubuntu@20.04" not supported by charm "my-charm", supported bases are: ubuntu@16.04, ubuntu@18.04`)
}

func (s StateValidatorSuite) TestValidateApplicationsWithUnsupportedSeriesWithForce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := mocks.NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"xenial", "bionic"}}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestStateSeriesValidator()
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), true)
	c.Assert(err, tc.ErrorIsNil)
}

type CharmhubValidatorSuite struct {
	testhelpers.IsolationSuite
}

func TestCharmhubValidatorSuite(t *tctesting.T) {
	tc.Run(t, &CharmhubValidatorSuite{})
}

func (s CharmhubValidatorSuite) TestValidateApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		}},
	}, nil)

	revision := 1

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestCharmhubSeriesValidator(client)
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithNoRevision(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockCharmhubClient(ctrl)

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{})
	application.EXPECT().Name().Return("foo")

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestCharmhubSeriesValidator(client)
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `no revision found for application "foo"`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithClientRefreshError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, errors.Errorf("bad"))

	revision := 1

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestCharmhubSeriesValidator(client)
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithRefreshError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestCharmhubSeriesValidator(client)
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `unable to locate application with base ubuntu@20.04: bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithRefreshErrorAndForce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{{
		Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		},
		Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := mocks.NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	applications := []machinemanager.Application{application}

	validator := machinemanager.NewTestCharmhubSeriesValidator(client)
	err := validator.ValidateApplications(applications, corebase.MakeDefaultBase("ubuntu", "20.04"), true)
	c.Assert(err, tc.ErrorIsNil)
}
