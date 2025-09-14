// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/permission"
)

type accessSuite struct{}

func TestAccessSuite(t *tctesting.T) {
	tc.Run(t, &accessSuite{})
}

func (*accessSuite) TestEqualOrGreaterModelAccessThan(c *tc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the controller permissions return true for any comparison.
	for _, value := range []permission.Access{login, addmodel, superuser} {
		c.Check(value.EqualOrGreaterModelAccessThan(undefined), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(read), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(write), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(admin), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(login), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.EqualOrGreaterModelAccessThan(login), tc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.EqualOrGreaterModelAccessThan(addmodel), tc.IsFalse)
	}

	c.Check(undefined.EqualOrGreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(undefined.EqualOrGreaterModelAccessThan(read), tc.IsFalse)
	c.Check(undefined.EqualOrGreaterModelAccessThan(write), tc.IsFalse)
	c.Check(undefined.EqualOrGreaterModelAccessThan(admin), tc.IsFalse)

	c.Check(read.EqualOrGreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(read.EqualOrGreaterModelAccessThan(read), tc.IsTrue)
	c.Check(read.EqualOrGreaterModelAccessThan(write), tc.IsFalse)
	c.Check(read.EqualOrGreaterModelAccessThan(admin), tc.IsFalse)

	c.Check(write.EqualOrGreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(read), tc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(write), tc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(admin), tc.IsFalse)

	c.Check(admin.EqualOrGreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(read), tc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(write), tc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(admin), tc.IsTrue)
}

func (*accessSuite) TestGreaterModelAccessThan(c *tc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of undefined or the controller permissions return true for any comparison.
	for _, value := range []permission.Access{undefined, login, addmodel, superuser} {
		c.Check(value.GreaterModelAccessThan(undefined), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(read), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(write), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(admin), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(login), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterModelAccessThan(login), tc.IsFalse)
		c.Check(value.GreaterModelAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterModelAccessThan(addmodel), tc.IsFalse)
	}

	c.Check(read.GreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(read.GreaterModelAccessThan(read), tc.IsFalse)
	c.Check(read.GreaterModelAccessThan(write), tc.IsFalse)
	c.Check(read.GreaterModelAccessThan(admin), tc.IsFalse)

	c.Check(write.GreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(write.GreaterModelAccessThan(read), tc.IsTrue)
	c.Check(write.GreaterModelAccessThan(write), tc.IsFalse)
	c.Check(write.GreaterModelAccessThan(admin), tc.IsFalse)

	c.Check(admin.GreaterModelAccessThan(undefined), tc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(read), tc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(write), tc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(admin), tc.IsFalse)
}

func (*accessSuite) TestEqualOrGreaterControllerAccessThan(c *tc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the model permissions return true for any comparison.
	for _, value := range []permission.Access{read, write, admin} {
		c.Check(value.EqualOrGreaterControllerAccessThan(undefined), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(read), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(admin), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(login), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(read), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(admin), tc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), tc.IsFalse)
	}

	c.Check(undefined.EqualOrGreaterControllerAccessThan(undefined), tc.IsTrue)
	c.Check(undefined.EqualOrGreaterControllerAccessThan(login), tc.IsFalse)
	c.Check(undefined.EqualOrGreaterControllerAccessThan(superuser), tc.IsFalse)

	c.Check(login.EqualOrGreaterControllerAccessThan(undefined), tc.IsTrue)
	c.Check(login.EqualOrGreaterControllerAccessThan(login), tc.IsTrue)
	c.Check(login.EqualOrGreaterControllerAccessThan(superuser), tc.IsFalse)

	c.Check(superuser.EqualOrGreaterControllerAccessThan(undefined), tc.IsTrue)
	c.Check(superuser.EqualOrGreaterControllerAccessThan(login), tc.IsTrue)
	c.Check(superuser.EqualOrGreaterControllerAccessThan(superuser), tc.IsTrue)
}

func (*accessSuite) TestGreaterControllerAccessThan(c *tc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of undefined or the model permissions return true for any comparison.
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterControllerAccessThan(undefined), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(read), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(write), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(admin), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(login), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(superuser), tc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.GreaterControllerAccessThan(read), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(write), tc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(admin), tc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.GreaterModelAccessThan(addmodel), tc.IsFalse)
	}

	c.Check(login.GreaterControllerAccessThan(undefined), tc.IsTrue)
	c.Check(login.GreaterControllerAccessThan(login), tc.IsFalse)
	c.Check(login.GreaterControllerAccessThan(superuser), tc.IsFalse)

	c.Check(superuser.GreaterControllerAccessThan(undefined), tc.IsTrue)
	c.Check(superuser.GreaterControllerAccessThan(login), tc.IsTrue)
	c.Check(superuser.GreaterControllerAccessThan(superuser), tc.IsFalse)
}

func (*accessSuite) TestEqualOrGreaterCloudAccessThan(c *tc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the model permissions return true for any comparison.
	for _, value := range []permission.Access{read, write} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), tc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{addmodel} {
		c.Check(value.EqualOrGreaterControllerAccessThan(read), tc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), tc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), tc.IsFalse)
	}

	c.Check(addmodel.EqualOrGreaterCloudAccessThan(addmodel), tc.IsTrue)
	c.Check(addmodel.EqualOrGreaterCloudAccessThan(admin), tc.IsFalse)
	c.Check(admin.EqualOrGreaterCloudAccessThan(addmodel), tc.IsTrue)
}
