// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type UserSuite struct {
	ConnSuite
}

func TestUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &UserSuite{})
}

func (s *UserSuite) TestAddInvalidNames(c *tc.C) {
	for _, name := range []string{
		"",
		"a",
		"b^b",
		"a.",
		"a-",
		"user@local",
		"@ubuntuone",
	} {
		c.Logf("check invalid name %q", name)
		user, err := s.State.AddUser(name, "ignored", "ignored", "ignored")
		c.Check(err, tc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Check(user, tc.IsNil)
	}
}

func (s *UserSuite) TestAddUser(c *tc.C) {
	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"
	creator := "admin"

	now := testing.NonZeroTime().Round(time.Second).UTC()

	user, err := s.State.AddUser(name, displayName, password, creator)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user, tc.NotNil)
	c.Assert(user.Name(), tc.Equals, name)
	c.Assert(user.DisplayName(), tc.Equals, displayName)
	c.Assert(user.PasswordValid(password), tc.IsTrue)
	c.Assert(user.CreatedBy(), tc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), tc.IsTrue)
	lastLogin, err := user.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(lastLogin, tc.DeepEquals, time.Time{})

	user, err = s.State.User(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user, tc.NotNil)
	c.Assert(user.Name(), tc.Equals, name)
	c.Assert(user.DisplayName(), tc.Equals, displayName)
	c.Assert(user.PasswordValid(password), tc.IsTrue)
	c.Assert(user.CreatedBy(), tc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), tc.IsTrue)
	lastLogin, err = user.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(lastLogin, tc.DeepEquals, time.Time{})
}

func (s *UserSuite) TestCheckUserExists(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	exists, err := state.CheckUserExists(s.State, user.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
	exists, err = state.CheckUserExists(s.State, strings.ToUpper(user.Name()))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
	exists, err = state.CheckUserExists(s.State, "notAUser")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsFalse)
}

func (s *UserSuite) TestString(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foo"})
	c.Assert(user.String(), tc.Equals, "foo")
}

func (s *UserSuite) TestUpdateLastLogin(c *tc.C) {
	now := testing.NonZeroTime().Round(time.Second).UTC()
	user := s.Factory.MakeUser(c, nil)
	err := user.UpdateLastLogin()
	c.Assert(err, tc.ErrorIsNil)
	lastLogin, err := user.LastLogin()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lastLogin.After(now) ||
		lastLogin.Equal(now), tc.IsTrue)
}

func (s *UserSuite) TestSetPassword(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(user.UserTag())
	})
}

func (s *UserSuite) TestAddUserSetsSalt(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password"})
	salt, hash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(hash, tc.Not(tc.Equals), "")
	c.Assert(salt, tc.Not(tc.Equals), "")
	c.Assert(utils.UserPasswordHash("a-password", salt), tc.Equals, hash)
	c.Assert(user.PasswordValid("a-password"), tc.IsTrue)
}

func (s *UserSuite) TestSetPasswordChangesSalt(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	origSalt, origHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(origSalt, tc.Not(tc.Equals), "")
	user.SetPassword("a-password")
	newSalt, newHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(newSalt, tc.Not(tc.Equals), "")
	c.Assert(newSalt, tc.Not(tc.Equals), origSalt)
	c.Assert(newHash, tc.Not(tc.Equals), origHash)
	c.Assert(user.PasswordValid("a-password"), tc.IsTrue)
}

func (s *UserSuite) TestRemoveUserNonExistent(c *tc.C) {
	err := s.State.RemoveUser(names.NewUserTag("harvey"))
	c.Assert(errors.IsNotFound(err), tc.IsTrue)
}

func (s *UserSuite) TestAllUsersSkipsDeletedUsers(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "one"})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "two"})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "three"})

	all, err := s.State.AllUsers(true)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(all), tc.DeepEquals, 4)

	var got []string
	for _, u := range all {
		got = append(got, u.Name())
	}
	c.Check(got, tc.SameContents, []string{"test-admin", "one", "two", "three"})

	s.State.RemoveUser(user.UserTag())

	all, err = s.State.AllUsers(true)
	got = nil
	for _, u := range all {
		got = append(got, u.Name())
	}
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(all), tc.DeepEquals, 3)
	c.Check(got, tc.SameContents, []string{"test-admin", "two", "three"})

}

func (s *UserSuite) TestRemoveUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), tc.IsTrue)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(u, tc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, tc.ErrorIsNil)

	// Check that we cannot update last login.
	err = u.UpdateLastLogin()
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, state.IsDeletedUserError)
	c.Check(err.Error(), tc.DeepEquals,
		fmt.Sprintf("cannot update last login: user %q is permanently deleted", user.Name()))

	// Check that we cannot set a password.
	err = u.SetPassword("should fail too")
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, state.IsDeletedUserError)
	c.Check(err.Error(), tc.DeepEquals,
		fmt.Sprintf("cannot set password: user %q is permanently deleted", user.Name()))

	// Check that we cannot set the password hash.
	err = u.SetPasswordHash("also", "fail")
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, state.IsDeletedUserError)
	c.Check(err.Error(), tc.DeepEquals,
		fmt.Sprintf("cannot set password hash: user %q is permanently deleted", user.Name()))

	// Check that we cannot validate a password.
	c.Check(u.PasswordValid("should fail"), tc.IsFalse)

	// Check that we cannot enable the user.
	err = u.Enable()
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, state.IsDeletedUserError)
	c.Check(err.Error(), tc.DeepEquals,
		fmt.Sprintf("cannot enable: user %q is permanently deleted", user.Name()))

	// Check that we cannot disable the user.
	err = u.Disable()
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, state.IsDeletedUserError)
	c.Check(err.Error(), tc.DeepEquals,
		fmt.Sprintf("cannot disable: user %q is permanently deleted", user.Name()))

	// Check again to verify the user cannot be retrieved.
	u, err = s.State.User(user.UserTag())
	c.Check(err, tc.ErrorMatches, `user "username-\d+" is permanently deleted`)
}

func (s *UserSuite) TestRemoveUserUppercaseName(c *tc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     name,
		Password: "wow very sea cret",
	})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("wow very sea cret"), tc.IsTrue)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(u, tc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, tc.ErrorIsNil)

	// Check to verify the user cannot be retrieved.
	_, err = s.State.User(user.UserTag())
	c.Check(err, tc.ErrorMatches, fmt.Sprintf(`user "%s" is permanently deleted`, name))
}

func (s *UserSuite) TestRemoveUserRemovesUserAccess(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), tc.IsTrue)

	s.State.SetUserAccess(user.UserTag(), s.Model.ModelTag(), permission.AdminAccess)
	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)

	uam, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uam.Access, tc.Equals, permission.AdminAccess)

	uac, err := s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uac.Access, tc.Equals, permission.SuperuserAccess)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(u, tc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, tc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("user %q is permanently deleted", user.UserTag().Name()))

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("user %q is permanently deleted", user.UserTag().Name()))
}

func (s *UserSuite) TestRecreatedUsersResetPermissions(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), tc.IsTrue)

	s.State.SetUserAccess(user.UserTag(), s.Model.ModelTag(), permission.AdminAccess)
	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)

	uam, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uam.Access, tc.Equals, permission.AdminAccess)

	uac, err := s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uac.Access, tc.Equals, permission.SuperuserAccess)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(u, tc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, tc.ErrorIsNil)

	// Add the user again with other password and access
	userRecreated := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "otherpassword",
		Access:   permission.ReadAccess})

	// Assert user exists and can authenticate.
	c.Assert(userRecreated.PasswordValid("otherpassword"), tc.IsTrue)

	// Check that the recreated user does not have the permissions set previously
	urac, err := s.State.UserAccess(userRecreated.UserTag(), s.State.ControllerTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(urac.Access, tc.Equals, permission.LoginAccess)

	// No model access was set yet
	uram, err := s.State.UserAccess(userRecreated.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uram.Access, tc.Equals, permission.ReadAccess)
}

func (s *UserSuite) TestDisableUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password"})
	c.Assert(user.IsDisabled(), tc.IsFalse)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{"test-admin", user.Name()})

	err := user.Disable()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user.IsDisabled(), tc.IsTrue)
	c.Assert(user.PasswordValid("a-password"), tc.IsFalse)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{"test-admin"})

	err = user.Enable()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user.IsDisabled(), tc.IsFalse)
	c.Assert(user.PasswordValid("a-password"), tc.IsTrue)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{"test-admin", user.Name()})
}

func (s *UserSuite) TestDisableUserUppercaseName(c *tc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password", Name: name})
	c.Assert(user.IsDisabled(), tc.IsFalse)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{name, "test-admin"})

	err := user.Disable()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user.IsDisabled(), tc.IsTrue)
	c.Assert(user.PasswordValid("a-password"), tc.IsFalse)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{"test-admin"})

	err = user.Enable()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(user.IsDisabled(), tc.IsFalse)
	c.Assert(user.PasswordValid("a-password"), tc.IsTrue)
	c.Assert(s.activeUsers(c), tc.DeepEquals, []string{name, "test-admin"})
}

func (s *UserSuite) TestDisableUserDisablesUserAccess(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), tc.IsTrue)

	s.State.SetUserAccess(user.UserTag(), s.Model.ModelTag(), permission.AdminAccess)
	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)

	uam, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uam.Access, tc.Equals, permission.AdminAccess)

	uac, err := s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uac.Access, tc.Equals, permission.SuperuserAccess)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(u, tc.DeepEquals, user)

	// Disable the user.
	err = u.Disable()
	c.Check(err, tc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("user %q is disabled", user.UserTag().Name()))

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("user %q is disabled", user.UserTag().Name()))

	// Re-enable the user.
	err = u.Refresh()
	c.Check(err, tc.ErrorIsNil)
	err = u.Enable()
	c.Check(err, tc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uam.Access, tc.Equals, permission.AdminAccess)

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uac.Access, tc.Equals, permission.SuperuserAccess)
}

func (s *UserSuite) activeUsers(c *tc.C) []string {
	users, err := s.State.AllUsers(false)
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Name()
	}
	return names
}

func (s *UserSuite) TestSetPasswordHash(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)

	salt, err := utils.RandomSalt()
	c.Assert(err, tc.ErrorIsNil)
	err = user.SetPasswordHash(utils.UserPasswordHash("foo", salt), salt)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo"), tc.IsTrue)
	c.Assert(user.PasswordValid("bar"), tc.IsFalse)

	// User passwords should *not* use the fast PasswordHash function
	hash := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	err = user.SetPasswordHash(hash, "")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo-12345678901234567890"), tc.IsFalse)
}

func (s *UserSuite) TestSetPasswordHashUppercaseName(c *tc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: name})

	salt, err := utils.RandomSalt()
	c.Assert(err, tc.ErrorIsNil)
	err = user.SetPasswordHash(utils.UserPasswordHash("foo", salt), salt)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo"), tc.IsTrue)
	c.Assert(user.PasswordValid("bar"), tc.IsFalse)

	// User passwords should *not* use the fast PasswordHash function
	hash := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	err = user.SetPasswordHash(hash, "")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo-12345678901234567890"), tc.IsFalse)
}

func (s *UserSuite) TestSetPasswordHashWithSalt(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)

	err := user.SetPasswordHash(utils.UserPasswordHash("foo", "salted"), "salted")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo"), tc.IsTrue)
	salt, _ := state.GetUserPasswordSaltAndHash(user)
	c.Assert(salt, tc.Equals, "salted")
}

func (s *UserSuite) TestCantDisableAdmin(c *tc.C) {
	user, err := s.State.User(s.Owner)
	c.Assert(err, tc.ErrorIsNil)
	err = user.Disable()
	c.Assert(err, tc.ErrorMatches, "cannot disable controller model owner")
}

func (s *UserSuite) TestCaseSensitiveUsersErrors(c *tc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "Bob"})

	_, err := s.State.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(err, tc.ErrorMatches, "user boB already exists")
}

func (s *UserSuite) TestCaseInsensitiveLookup(c *tc.C) {
	expectedUser := s.Factory.MakeUser(c, &factory.UserParams{Name: "Bob"})

	assertCaseInsensitiveLookup := func(name string) {
		userTag := names.NewUserTag(name)
		obtainedUser, err := s.State.User(userTag)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(obtainedUser, tc.DeepEquals, expectedUser)
	}

	assertCaseInsensitiveLookup("bob")
	assertCaseInsensitiveLookup("bOb")
	assertCaseInsensitiveLookup("boB")
	assertCaseInsensitiveLookup("BOB")
}

func (s *UserSuite) TestAllUsers(c *tc.C) {
	// Create in non-alphabetical order.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "conrad"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "adam"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "debbie", Disabled: true})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred", Disabled: true})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "erica"})
	// There is the existing controller owner called "test-admin"

	includeDeactivated := false
	users, err := s.State.AllUsers(includeDeactivated)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.HasLen, 5)
	c.Check(users[0].Name(), tc.Equals, "adam")
	c.Check(users[1].Name(), tc.Equals, "barbara")
	c.Check(users[2].Name(), tc.Equals, "conrad")
	c.Check(users[3].Name(), tc.Equals, "erica")
	c.Check(users[4].Name(), tc.Equals, "test-admin")

	includeDeactivated = true
	users, err = s.State.AllUsers(includeDeactivated)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.HasLen, 7)
	c.Check(users[0].Name(), tc.Equals, "adam")
	c.Check(users[1].Name(), tc.Equals, "barbara")
	c.Check(users[2].Name(), tc.Equals, "conrad")
	c.Check(users[3].Name(), tc.Equals, "debbie")
	c.Check(users[4].Name(), tc.Equals, "erica")
	c.Check(users[5].Name(), tc.Equals, "fred")
	c.Check(users[6].Name(), tc.Equals, "test-admin")
}

func (s *UserSuite) TestAddDeletedUser(c *tc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "bob"})

	_ = s.State.RemoveUser(names.NewUserTag("bob"))

	u, err := s.State.AddUser(
		"bob", "displayname", "password", "creator")

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.Name(), tc.Equals, "bob")
	c.Assert(u.DisplayName(), tc.Equals, "displayname")
	c.Assert(u.CreatedBy(), tc.Equals, "creator")
}

func (s *UserSuite) TestAddUserNoSecretKey(c *tc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.IsNil)
}

func (s *UserSuite) TestAddUserSecretKey(c *tc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.HasLen, 32)
	c.Assert(u.PasswordValid(""), tc.IsFalse)
}

func (s *UserSuite) TestSetPasswordClearsSecretKey(c *tc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.HasLen, 32)
	err = u.SetPassword("anything")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.IsNil)
	err = u.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.IsNil)
}

func (s *UserSuite) TestResetPassword(c *tc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.HasLen, 32)
	oldKey := u.SecretKey()

	key, err := u.ResetPassword()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.Not(tc.DeepEquals), oldKey)
	c.Assert(key, tc.NotNil)
	c.Assert(u.SecretKey(), tc.DeepEquals, key)
}

func (s *UserSuite) TestResetPasswordUppercaseName(c *tc.C) {
	u, err := s.State.AddUserWithSecretKey("BobHasAnUppercaseName", "display", "admin")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.HasLen, 32)
	oldKey := u.SecretKey()

	key, err := u.ResetPassword()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.Not(tc.DeepEquals), oldKey)
	c.Assert(key, tc.NotNil)
	c.Assert(u.SecretKey(), tc.DeepEquals, key)
}

func (s *UserSuite) TestResetPasswordFailIfDeactivated(c *tc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, tc.ErrorIsNil)

	err = u.Disable()
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.ResetPassword()
	c.Assert(err, tc.ErrorMatches, `cannot reset password for user "bob": user deactivated`)
	c.Assert(u.SecretKey(), tc.IsNil)
}

func (s *UserSuite) TestResetPasswordFailIfDeleted(c *tc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveUser(u.Tag().(names.UserTag))
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.ResetPassword()
	c.Assert(err, tc.ErrorMatches, `cannot reset password for user "bob": user "bob" is permanently deleted`)
	c.Assert(u.SecretKey(), tc.IsNil)
}

func (s *UserSuite) TestResetPasswordIfPasswordSet(c *tc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, tc.ErrorIsNil)

	err = u.SetPassword("anything")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.PasswordValid("anything"), tc.IsTrue)
	c.Assert(u.SecretKey(), tc.IsNil)

	key, err := u.ResetPassword()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u.SecretKey(), tc.DeepEquals, key)
	c.Assert(u.PasswordValid("anything"), tc.IsFalse)
}
