// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
)

type PermissionSuite struct {
	testing.BaseSuite
}

func TestPermissionSuite(t *tctesting.T) {
	tc.Run(t, &PermissionSuite{})
}

type fakeUserAccess struct {
	subjects []names.UserTag
	objects  []names.Tag
	access   permission.Access
	err      error
}

func (f *fakeUserAccess) call(subject names.UserTag, object names.Tag) (permission.Access, error) {
	f.subjects = append(f.subjects, subject)
	f.objects = append(f.objects, object)
	return f.access, f.err
}

func (r *PermissionSuite) TestNoUserTagLacksPermission(c *tc.C) {
	nonUser := names.NewModelTag("beef1beef1-0000-0000-000011112222")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	hasPermission, err := common.HasPermission((&fakeUserAccess{}).call, nonUser, permission.ReadAccess, target)
	c.Assert(hasPermission, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
}

func (r *PermissionSuite) TestHasPermission(c *tc.C) {
	testCases := []struct {
		title            string
		userGetterAccess permission.Access
		user             names.UserTag
		target           names.Tag
		access           permission.Access
		expected         bool
	}{
		{
			title:            "user has lesser permissions than required",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         false,
		},
		{
			title:            "user has equal permission than required",
			userGetterAccess: permission.WriteAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         true,
		},
		{
			title:            "user has greater permission than required",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         true,
		},
		{
			title:            "user requests model permission on controller",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
		{
			title:            "user requests controller permission on model",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.AdminAccess, // notice user has this permission for model.
			expected:         false,
		},
		{
			title:            "controller permissions also work",
			userGetterAccess: permission.SuperuserAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.SuperuserAccess,
			expected:         true,
		},
		{
			title:            "cloud permissions work",
			userGetterAccess: permission.AddModelAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewCloudTag("mycloud"),
			access:           permission.AddModelAccess,
			expected:         true,
		},
		{
			title:            "user has lesser cloud permissions than required",
			userGetterAccess: permission.NoAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewCloudTag("mycloud"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
		{
			title:            "user has lesser offer permissions than required",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.WriteAccess,
			expected:         false,
		},
		{
			title:            "user has equal offer permission than required",
			userGetterAccess: permission.ConsumeAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.ConsumeAccess,
			expected:         true,
		},
		{
			title:            "user has greater offer permission than required",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.ConsumeAccess,
			expected:         true,
		},
		{
			title:            "user requests controller permission on offer",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
	}
	for i, t := range testCases {
		userGetter := &fakeUserAccess{
			access: t.userGetterAccess,
		}
		c.Logf("HasPermission test n %d: %s", i, t.title)
		hasPermission, err := common.HasPermission(userGetter.call, t.user, t.access, t.target)
		c.Assert(hasPermission, tc.Equals, t.expected)
		c.Assert(err, tc.ErrorIsNil)
	}

}

func (r *PermissionSuite) TestUserGetterErrorReturns(c *tc.C) {
	user := names.NewUserTag("validuser")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	userGetter := &fakeUserAccess{
		access: permission.NoAccess,
		err:    errors.NotFoundf("a user"),
	}
	hasPermission, err := common.HasPermission(userGetter.call, user, permission.ReadAccess, target)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hasPermission, tc.IsFalse)
	c.Assert(userGetter.subjects, tc.HasLen, 1)
	c.Assert(userGetter.subjects[0], tc.DeepEquals, user)
	c.Assert(userGetter.objects, tc.HasLen, 1)
	c.Assert(userGetter.objects[0], tc.DeepEquals, target)
}

type fakeEveryoneUserAccess struct {
	user     permission.Access
	everyone permission.Access
}

func (f *fakeEveryoneUserAccess) call(subject names.UserTag, object names.Tag) (permission.Access, error) {
	if subject.Id() == common.EveryoneTagName {
		return f.everyone, nil
	}
	return f.user, nil
}

func (r *PermissionSuite) TestEveryoneAtExternal(c *tc.C) {
	testCases := []struct {
		title            string
		userGetterAccess permission.Access
		everyoneAccess   permission.Access
		user             names.UserTag
		target           names.Tag
		access           permission.Access
		expected         bool
	}{
		{
			title:            "user has lesser permissions than everyone",
			userGetterAccess: permission.LoginAccess,
			everyoneAccess:   permission.SuperuserAccess,
			user:             names.NewUserTag("validuser@external"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.SuperuserAccess,
			expected:         true,
		},
		{
			title:            "user has greater permissions than everyone",
			userGetterAccess: permission.SuperuserAccess,
			everyoneAccess:   permission.LoginAccess,
			user:             names.NewUserTag("validuser@external"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.SuperuserAccess,
			expected:         true,
		},
		{
			title:            "everibody not considered if user is local",
			userGetterAccess: permission.LoginAccess,
			everyoneAccess:   permission.SuperuserAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.SuperuserAccess,
			expected:         false,
		},
	}

	for i, t := range testCases {
		userGetter := &fakeEveryoneUserAccess{
			user:     t.userGetterAccess,
			everyone: t.everyoneAccess,
		}
		c.Logf(`HasPermission "everyone" test n %d: %s`, i, t.title)
		hasPermission, err := common.HasPermission(userGetter.call, t.user, t.access, t.target)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(hasPermission, tc.Equals, t.expected)
	}
}

func (r *PermissionSuite) TestHasModelAdminSuperUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(nil)

	has, err := common.HasModelAdmin(auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

func (r *PermissionSuite) TestHasModelAdminYes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	auth.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(nil)

	has, err := common.HasModelAdmin(auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

func (r *PermissionSuite) TestHasModelAdminNo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	auth.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(authentication.ErrorEntityMissingPermission)

	has, err := common.HasModelAdmin(auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsFalse)
}

func (r *PermissionSuite) TestHasModelAdminError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	auth := mocks.NewMockAuthorizer(ctrl)
	auth.EXPECT().HasPermission(permission.SuperuserAccess, testing.ControllerTag).Return(authentication.ErrorEntityMissingPermission)
	someError := errors.New("error")
	auth.EXPECT().HasPermission(permission.AdminAccess, testing.ModelTag).Return(someError)

	has, err := common.HasModelAdmin(auth, testing.ControllerTag, testing.ModelTag)
	c.Assert(err, tc.ErrorIs, someError)
	c.Assert(has, tc.IsFalse)
}
