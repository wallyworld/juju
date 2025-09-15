// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type CloudUserSuite struct {
	ConnSuite
}

func TestCloudUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &CloudUserSuite{})
}

func (s *CloudUserSuite) makeCloud(c *tc.C, access permission.Access) (cloud.Cloud, names.UserTag) {
	cloud := cloud.Cloud{
		Name:      "fluffy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}
	err := s.State.AddCloud(cloud, "test-admin")
	c.Assert(err, tc.ErrorIsNil)
	user := s.Factory.MakeUser(c,
		&factory.UserParams{Name: "validusername"})

	// Initially no access.
	_, err = s.State.UserPermission(user.UserTag(), names.NewCloudTag(cloud.Name))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = s.State.CreateCloudAccess(cloud.Name, user.UserTag(), access)
	c.Assert(err, tc.ErrorIsNil)
	return cloud, user.UserTag()
}

func (s *CloudUserSuite) assertAddCloud(c *tc.C, wantedAccess permission.Access) string {
	cloud, user := s.makeCloud(c, wantedAccess)

	access, err := s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, wantedAccess)

	// Creator of cloud has admin.
	access, err = s.State.GetCloudAccess(cloud.Name, names.NewUserTag("test-admin"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)

	// Everyone else has no access.
	_, err = s.State.GetCloudAccess(cloud.Name, names.NewUserTag("everyone@external"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	return cloud.Name
}

func (s *CloudUserSuite) TestAddModelUser(c *tc.C) {
	s.assertAddCloud(c, permission.AddModelAccess)
}

func (s *CloudUserSuite) TestGetCloudAccess(c *tc.C) {
	cloud := s.assertAddCloud(c, permission.AddModelAccess)
	users, err := s.State.GetCloudUsers(cloud)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.DeepEquals, map[string]permission.Access{
		"test-admin":    permission.AdminAccess,
		"validusername": permission.AddModelAccess,
	})
}

func (s *CloudUserSuite) TestUpdateCloudAccess(c *tc.C) {
	cloud, user := s.makeCloud(c, permission.AdminAccess)
	err := s.State.UpdateCloudAccess(cloud.Name, user, permission.AddModelAccess)
	c.Assert(err, tc.ErrorIsNil)

	access, err := s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AddModelAccess)
}

func (s *CloudUserSuite) TestCreateCloudAccessNoUserFails(c *tc.C) {
	cloud := cloud.Cloud{
		Name:      "fluffy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}
	err := s.State.AddCloud(cloud, "test-admin")
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.CreateCloudAccess(
		"fluffy",
		names.NewUserTag("validusername"), permission.AddModelAccess)
	c.Assert(err, tc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *CloudUserSuite) TestRemoveCloudAccess(c *tc.C) {
	cloud, user := s.makeCloud(c, permission.AddModelAccess)

	err := s.State.RemoveCloudAccess(cloud.Name, user)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CloudUserSuite) TestRemoveCloudAccessNoUser(c *tc.C) {
	cloud, _ := s.makeCloud(c, permission.AddModelAccess)
	err := s.State.RemoveCloudAccess(cloud.Name, names.NewUserTag("fred"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *CloudUserSuite) TestCloudsForUser(c *tc.C) {
	cloudName := s.assertAddCloud(c, permission.AddModelAccess)
	info, err := s.State.CloudsForUser(names.NewUserTag("validusername"), false)
	c.Assert(err, tc.ErrorIsNil)
	cloud, err := s.State.Cloud(cloudName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, []state.CloudInfo{
		{
			Cloud:  cloud,
			Access: permission.AddModelAccess,
		},
	})
}

func (s *CloudUserSuite) TestCloudsForUserAll(c *tc.C) {
	cloudName := s.assertAddCloud(c, permission.AddModelAccess)
	info, err := s.State.CloudsForUser(names.NewUserTag("test-admin"), true)
	c.Assert(err, tc.ErrorIsNil)
	cloud, err := s.State.Cloud(cloudName)
	c.Assert(err, tc.ErrorIsNil)
	controllerInfo, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	controllerCloud, err := s.State.Cloud(controllerInfo.CloudName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, []state.CloudInfo{
		{
			Cloud:  controllerCloud,
			Access: permission.AdminAccess,
		}, {
			Cloud:  cloud,
			Access: permission.AdminAccess,
		},
	})
}
