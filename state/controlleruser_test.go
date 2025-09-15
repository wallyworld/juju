// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
)

type ControllerUserSuite struct {
	ConnSuite
}

func TestControllerUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ControllerUserSuite{})
}

func (s *ControllerUserSuite) TestDefaultAccessControllerUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	t := user.Tag()
	userTag := t.(names.UserTag)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerUser.Access, tc.Equals, permission.LoginAccess)
}

func (s *ControllerUserSuite) TestSetAccessControllerUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	t := user.Tag()
	userTag := t.(names.UserTag)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerUser.Access, tc.Equals, permission.LoginAccess)

	s.State.SetUserAccess(userTag, ctag, permission.SuperuserAccess)

	controllerUser, err = s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerUser.Access, tc.Equals, permission.SuperuserAccess)
}

func (s *ControllerUserSuite) TestRemoveControllerUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	_, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ControllerUserSuite) TestRemoveControllerUserSucceeds(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerUserSuite) TestRemoveControllerUserFails(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}
