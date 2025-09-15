// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/charmhub/transport"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type UpdateBaseSuite struct {
	testhelpers.IsolationSuite
}

func TestUpdateBaseSuite(t *tctesting.T) {
	tc.Run(t, &UpdateBaseSuite{})
}

func (s *UpdateBaseSuite) TestUpdateBase(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)
	app.EXPECT().UpdateApplicationBase(state.UbuntuBase("20.04"), false)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)
	coreBase := corebase.MakeDefaultBase("ubuntu", "20.04")
	validator.EXPECT().ValidateApplication(app, coreBase, false).Return(nil)

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", coreBase, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpdateBaseSuite) TestUpdateBaseNoSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := NewUpdateBaseAPI(nil, nil)
	err := api.UpdateBase("application-foo", corebase.Base{}, false)
	c.Assert(err, tc.ErrorMatches, `base missing from args`)
}

func (s *UpdateBaseSuite) TestUpdateBaseNotPrincipal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(false)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `"foo" is a subordinate application, update-series not supported`)
}

func (s *UpdateBaseSuite) TestUpdateBaseNotValid(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)
	validator.EXPECT().ValidateApplication(app, corebase.MakeDefaultBase("ubuntu", "20.04"), false).Return(errors.New("bad"))

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

type StateValidatorSuite struct {
	testhelpers.IsolationSuite
}

func TestStateValidatorSuite(t *tctesting.T) {
	tc.Run(t, &StateValidatorSuite{})
}

func (s StateValidatorSuite) TestValidateApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"focal", "bionic"},
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationWithNoBases(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name: "my-charm",
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `charm "my-charm" does not support any bases. Not valid`)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
		Name:   "my-charm",
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `base "ubuntu@20.04" not supported by charm "my-charm", supported bases are: ubuntu@16.04, ubuntu@18.04`)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeriesWithForce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), true)
	c.Assert(err, tc.ErrorIsNil)
}

type CharmhubValidatorSuite struct {
	testhelpers.IsolationSuite
}

func TestCharmhubValidatorSuite(t *tctesting.T) {
	tc.Run(t, &CharmhubValidatorSuite{})
}

func (s CharmhubValidatorSuite) TestValidateApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithNoRevision(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{})
	application.EXPECT().Name().Return("foo")

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `no revision found for application "foo"`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithClientRefreshError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, errors.Errorf("bad"))

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithRefreshError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, tc.ErrorMatches, `unable to locate application with base ubuntu@20.04: bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithRefreshErrorAndForce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{{
		Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		},
		Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, corebase.MakeDefaultBase("ubuntu", "20.04"), true)
	c.Assert(err, tc.ErrorIsNil)
}
