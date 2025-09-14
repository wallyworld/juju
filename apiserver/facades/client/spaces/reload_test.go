// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/context"
	environmocks "github.com/juju/juju/environs/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

// ReloadSpacesAPISuite is used to test API calls using mocked model operations.
type ReloadSpacesAPISuite struct {
	testhelpers.IsolationSuite
}

func TestReloadSpacesAPISuite(t *tctesting.T) {
	tc.Run(t, &ReloadSpacesAPISuite{})
}

func (s *ReloadSpacesAPISuite) TestReloadSpaces(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ModelCloudInfo: mockEnvirons,
			controllerUUID: coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(context, mockState, mockNetworkEnviron).Return(nil)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, tc.ErrorIsNil)
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesGetControllerConfigFail(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{}, errors.New("broken controller"))

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Assert(err, tc.ErrorMatches, "get controller config: broken controller")
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithNoEnviron(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ModelCloudInfo: mockEnvirons,
			controllerUUID: coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, errors.New("boom"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithReloadSpaceError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ModelCloudInfo: mockEnvirons,
			controllerUUID: coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(context, mockState, mockNetworkEnviron).Return(errors.New("boom"))

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, tc.ErrorMatches, "boom")
}

type ReloadSpacesAuthorizerSuite struct {
	testhelpers.IsolationSuite
}

func TestReloadSpacesAuthorizerSuite(t *tctesting.T) {
	tc.Run(t, &ReloadSpacesAuthorizerSuite{})
}

func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizer(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(nil)

	blockChecker := NewMockBlockChecker(ctrl)
	blockChecker.EXPECT().ChangeAllowed().Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, tc.ErrorIsNil)
}

func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizerCannotWrite(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(apiservererrors.ErrPerm)

	blockChecker := NewMockBlockChecker(ctrl)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, tc.ErrorMatches, "permission denied")
}

// Note: If HasPermission returns an error, but returns true then they can go
// through to the block checker.
func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizerNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(nil)

	blockChecker := NewMockBlockChecker(ctrl)
	blockChecker.EXPECT().ChangeAllowed().Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, tc.ErrorIsNil)
}
