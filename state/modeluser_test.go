// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type ModelUserSuite struct {
	ConnSuite
}

func TestModelUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelUserSuite{})
}

func (s *ModelUserSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
}

func (s *ModelUserSuite) TestAddModelUser(c *tc.C) {
	now := state.NowToTheSecond(s.State)
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.WriteAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelUser.UserID, tc.Equals, fmt.Sprintf("%s:validusername", s.modelTag.Id()))
	c.Assert(modelUser.Object, tc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, tc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), tc.Equals, "createdby")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), tc.IsTrue)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, tc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), tc.IsTrue)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.UserID, tc.Equals, fmt.Sprintf("%s:validusername", s.modelTag.Id()))
	c.Assert(modelUser.Object, tc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, tc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), tc.Equals, "createdby")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), tc.IsTrue)
	when, err = s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, tc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), tc.IsTrue)
}

func (s *ModelUserSuite) TestAddReadOnlyModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, tc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestAddReadWriteModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.WriteAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, tc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)
}

func (s *ModelUserSuite) TestAddAdminModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.AdminAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, tc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, tc.Equals, permission.AdminAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.UserName, tc.Equals, "validusername")
	c.Assert(modelUser.Access, tc.Equals, permission.AdminAccess)
}

func (s *ModelUserSuite) TestDefaultAccessModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestSetAccessModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.AdminAccess,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.Access, tc.Equals, permission.AdminAccess)

	s.State.SetUserAccess(modelUser.UserTag, s.Model.ModelTag(), permission.ReadAccess)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestCaseUserNameVsId(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	user, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("Bob@RandomProvider"),
			CreatedBy: model.Owner(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.IsNil)
	c.Assert(user.UserName, tc.Equals, "Bob@RandomProvider")
	c.Assert(user.UserID, tc.Equals, state.DocID(s.State, "bob@randomprovider"))
}

func (s *ModelUserSuite) TestCaseSensitiveModelUserErrors(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "Bob@ubuntuone"})

	_, err = s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("boB@ubuntuone"),
			CreatedBy: model.Owner(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorMatches, `user access "boB@ubuntuone" already exists`)
	c.Assert(errors.IsAlreadyExists(err), tc.IsTrue)
}

func (s *ModelUserSuite) TestCaseInsensitiveLookupInMultiEnvirons(c *tc.C) {
	assertIsolated := func(st1, st2 *state.State, usernames ...string) {
		f := factory.NewFactory(st1, s.StatePool)
		expectedUser := f.MakeModelUser(c, &factory.ModelUserParams{User: usernames[0]})

		m1, err := st1.Model()
		c.Assert(err, tc.ErrorIsNil)

		m2, err := st2.Model()
		c.Assert(err, tc.ErrorIsNil)

		// assert case insensitive lookup for each username
		for _, username := range usernames {
			userTag := names.NewUserTag(username)
			obtainedUser, err := st1.UserAccess(userTag, m1.ModelTag())
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(obtainedUser, tc.DeepEquals, expectedUser)

			_, err = st2.UserAccess(userTag, m2.ModelTag())
			c.Assert(errors.IsNotFound(err), tc.IsTrue)
		}
	}

	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()
	assertIsolated(s.State, otherSt,
		"Bob@UbuntuOne",
		"bob@ubuntuone",
		"BOB@UBUNTUONE",
	)
	assertIsolated(otherSt, s.State,
		"Sam@UbuntuOne",
		"sam@ubuntuone",
		"SAM@UBUNTUONE",
	)
}

func (s *ModelUserSuite) TestAddModelDisplayName(c *tc.C) {
	modelUserDefault := s.Factory.MakeModelUser(c, nil)
	c.Assert(modelUserDefault.DisplayName, tc.Matches, "display name-[0-9]*")

	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{DisplayName: "Override user display name"})
	c.Assert(modelUser.DisplayName, tc.Equals, "Override user display name")
}

func (s *ModelUserSuite) TestAddModelNoUserFails(c *tc.C) {
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewLocalUserTag("validusername"),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *ModelUserSuite) TestAddModelNoCreatedByUserFails(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: names.NewLocalUserTag("createdby"),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorMatches, `createdBy user "createdby" does not exist locally: user "createdby" not found`)
}

func (s *ModelUserSuite) TestRemoveModelUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	_, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveUserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestRemoveModelUserFails(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.State.RemoveUserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestUpdateLastConnection(c *tc.C) {
	now := state.NowToTheSecond(s.State)
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	err = s.Model.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	// It is possible that the update is done over a second boundary, so we need
	// to check for after now as well as equal.
	c.Assert(when.After(now) || when.Equal(now), tc.IsTrue)
}

func (s *ModelUserSuite) TestUpdateLastConnectionTwoModelUsers(c *tc.C) {
	now := state.NowToTheSecond(s.State)

	// Create a user and add them to the initial model.
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	// Create a second model and add the same user to this.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelUser2, err := model2.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	// Now we have two model users with the same username. Ensure we get
	// separate last connections.

	// Connect modelUser and get last connection.
	err = s.Model.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), tc.IsTrue)

	// Try to get last connection for modelUser2. As they have never connected,
	// we expect to get an error.
	_, err = model2.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, tc.ErrorMatches, `never connected: "validusername"`)

	// Connect modelUser2 and get last connection.
	err = s.Model.UpdateLastModelConnection(modelUser2.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	when, err = s.Model.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), tc.IsTrue)
}

func (s *ModelUserSuite) TestModelUUIDsForUserNone(c *tc.C) {
	tag := names.NewUserTag("non-existent@remote")
	models, err := s.State.ModelUUIDsForUser(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUserNewLocalUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUser(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, []string{s.State.ModelUUID()})

	modelTag := names.NewModelTag(models[0])
	access, err := s.State.UserAccess(user.UserTag(), modelTag)
	c.Assert(err, tc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(access.UserTag)
	c.Assert(err, tc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), tc.IsTrue)
}

func (s *ModelUserSuite) TestImportingModelUUIDsForUser(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, []string{s.State.ModelUUID()})

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	models, err = s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUserModelOwner(c *tc.C) {
	owner := names.NewUserTag("external@remote")
	model := s.newModelWithOwner(c, owner)

	models, err := s.State.ModelUUIDsForUser(owner)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, []string{model.UUID()})
}

func (s *ModelUserSuite) TestModelUUIDsForUserOfNewModel(c *tc.C) {
	userTag := names.NewUserTag("external@remote")
	model := s.newModelWithUser(c, userTag, state.ModelTypeIAAS)

	models, err := s.State.ModelUUIDsForUser(userTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, []string{model.UUID()})
}

func (s *ModelUserSuite) TestModelUUIDsForUserMultiple(c *tc.C) {
	userTag := names.NewUserTag("external@remote")
	expected := []string{
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithOwner(c, userTag).UUID(),
		s.newModelWithOwner(c, userTag).UUID(),
	}

	models, err := s.State.ModelUUIDsForUser(userTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.SameContents, expected)
}

func (s *ModelUserSuite) TestModelBasicInfoForUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	model := s.newModelWithUser(c, user.UserTag(), state.ModelTypeIAAS)
	model2 := s.newModelWithUser(c, user.UserTag(), state.ModelTypeCAAS)

	models, err := s.State.ModelBasicInfoForUser(user.UserTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.SameContents, []state.ModelAccessInfo{
		{
			Name:  model.Name(),
			Type:  model.Type(),
			UUID:  model.UUID(),
			Owner: "test-admin",
		}, {
			Name:  model2.Name(),
			Type:  model2.Type(),
			UUID:  model2.UUID(),
			Owner: "test-admin",
		},
	})
}

func (s *ModelUserSuite) TestIsControllerAdmin(c *tc.C) {
	isAdmin, err := s.State.IsControllerAdmin(s.Owner)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsTrue)

	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsFalse)

	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsTrue)

	readonly := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: permission.ReadAccess})
	isAdmin, err = s.State.IsControllerAdmin(readonly.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsFalse)
}

func (s *ModelUserSuite) TestIsControllerAdminFromOtherState(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Owner: user.UserTag()})
	defer otherState.Close()

	isAdmin, err := otherState.IsControllerAdmin(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsFalse)

	isAdmin, err = otherState.IsControllerAdmin(s.Owner)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isAdmin, tc.IsTrue)
}

func (s *ModelUserSuite) newModelWithOwner(c *tc.C, owner names.UserTag) *state.Model {
	// Don't use the factory to call MakeModel because it may at some
	// time in the future be modified to do additional things.  Instead call
	// the state method directly to create an model to make sure that
	// the owner is able to access the model.
	uuid, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	uuidStr := uuid.String()

	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": uuidStr[:8],
		"uuid": uuidStr,
	})
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()
	return model
}

func (s *ModelUserSuite) newModelWithUser(c *tc.C, user names.UserTag, modelType state.ModelType) *state.Model {
	params := &factory.ModelParams{Type: modelType}
	st := s.Factory.MakeModel(c, params)
	defer st.Close()
	newModel, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	_, err = newModel.AddUser(
		state.UserAccessSpec{
			User: user, CreatedBy: newModel.Owner(),
			Access: permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorIsNil)
	return newModel
}
