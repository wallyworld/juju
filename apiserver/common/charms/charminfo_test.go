// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/common/charms/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
)

type charmInfoSuite struct{}

func TestCharmInfoSuite(t *tctesting.T) {
	tc.Run(t, &charmInfoSuite{})
}

func (s *charmInfoSuite) TestBasic(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().Model().Return(nil, nil)
	ch := mocks.NewMockCharm(ctrl)
	st.EXPECT().Charm("foo-1").Return(ch, nil)

	// The convertCharm logic is tested in the CharmInfo tests, so just test
	// the minimal set of fields here.
	ch.EXPECT().Revision().Return(1)
	ch.EXPECT().Config().Return(&charm.Config{})
	ch.EXPECT().Meta().Return(&charm.Meta{Name: "foo"})
	ch.EXPECT().Actions().Return(&charm.Actions{})
	ch.EXPECT().Metrics().Return(&charm.Metrics{})
	ch.EXPECT().Manifest().Return(&charm.Manifest{})
	ch.EXPECT().LXDProfile().Return(&charm.LXDProfile{})
	ch.EXPECT().URL().Return("ch:foo-1")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(true)

	// Make the CharmInfo call
	api, err := charms.NewCharmInfoAPI(st, authorizer)
	c.Assert(err, tc.IsNil)
	charmInfo, err := api.CharmInfo(params.CharmURL{URL: "foo-1"})
	c.Assert(err, tc.IsNil)

	c.Check(charmInfo.URL, tc.Equals, "ch:foo-1")
	c.Check(charmInfo.Meta.Name, tc.Equals, "foo")
}

func (s *charmInfoSuite) TestPermissionDenied(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().Return(model, nil)

	modelTag := names.NewModelTag("1")
	model.EXPECT().ModelTag().Return(modelTag)

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(false)
	authorizer.EXPECT().HasPermission(permission.ReadAccess, modelTag).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	// Make the CharmInfo call
	api, err := charms.NewCharmInfoAPI(st, authorizer)
	c.Assert(err, tc.IsNil)
	_, err = api.CharmInfo(params.CharmURL{URL: "foo"})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
