// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
)

type internalUserSuite struct {
	internalStateSuite
}

func TestInternalUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &internalUserSuite{})
}

func (s *internalUserSuite) TestCreateInitialUserOps(c *tc.C) {
	tag := names.NewUserTag("AdMiN")
	ops := createInitialUserOps(s.state.ControllerUUID(), tag, "abc", "salt", testing.ZeroTime())
	c.Assert(ops, tc.HasLen, 3)
	op := ops[0]
	c.Assert(op.Id, tc.Equals, "admin")

	doc := op.Insert.(*userDoc)
	c.Assert(doc.DocID, tc.Equals, "admin")
	c.Assert(doc.Name, tc.Equals, "AdMiN")
	c.Assert(doc.PasswordSalt, tc.Equals, "salt")

	// controller user permissions
	op = ops[1]
	permdoc := op.Insert.(*permissionDoc)
	c.Assert(permdoc.Access, tc.Equals, string(permission.SuperuserAccess))
	c.Assert(permdoc.ID, tc.Equals, permissionID(controllerKey(s.state.ControllerUUID()), userGlobalKey(strings.ToLower(tag.Id()))))
	c.Assert(permdoc.SubjectGlobalKey, tc.Equals, userGlobalKey(strings.ToLower(tag.Id())))
	c.Assert(permdoc.ObjectGlobalKey, tc.Equals, controllerKey(s.state.ControllerUUID()))

	// controller user
	op = ops[2]
	cudoc := op.Insert.(*userAccessDoc)
	c.Assert(cudoc.ID, tc.Equals, "admin")
	c.Assert(cudoc.ObjectUUID, tc.Equals, s.state.ControllerUUID())
	c.Assert(cudoc.UserName, tc.Equals, "AdMiN")
	c.Assert(cudoc.DisplayName, tc.Equals, "AdMiN")
	c.Assert(cudoc.CreatedBy, tc.Equals, "AdMiN")
}

func (s *internalUserSuite) TestCaseNameVsId(c *tc.C) {
	user, err := s.state.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(err, tc.IsNil)
	c.Assert(user.Name(), tc.Equals, "boB")
	c.Assert(user.doc.DocID, tc.Equals, "bob")
}
