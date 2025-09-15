// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
)

type ModelCredentialSuite struct {
	ConnSuite

	credentialTag names.CloudCredentialTag
}

func TestModelCredentialSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelCredentialSuite{})
}

func (s *ModelCredentialSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	s.credentialTag = s.createCloudCredential(c, "foobar")
}

func (s *ModelCredentialSuite) TestInvalidateModelCredentialNone(c *tc.C) {
	// The model created in ConnSuite does not have a credential.
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, exists := m.CloudCredentialTag()
	c.Assert(exists, tc.IsFalse)
	_, exists, err = m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsFalse)

	reason := "special invalidation"
	err = s.State.InvalidateModelCredential(reason)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelCredentialSuite) TestInvalidateModelCredential(c *tc.C) {
	st := s.addModel(c, "abcmodel", s.credentialTag)
	defer st.Close()
	credential, err := s.State.CloudCredential(s.credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credential.IsValid(), tc.IsTrue)

	reason := "special invalidation"
	err = st.InvalidateModelCredential(reason)
	c.Assert(err, tc.ErrorIsNil)

	invalidated, err := s.State.CloudCredential(s.credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(invalidated.IsValid(), tc.IsFalse)
	c.Assert(invalidated.InvalidReason, tc.DeepEquals, reason)

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	info, err := m.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, status.StatusInfo{
		Status:  "suspended",
		Message: "suspended since cloud credential is not valid",
		Data:    map[string]interface{}{"reason": "special invalidation"},
	})
}

func (s *ModelCredentialSuite) TestValidateCloudCredentialWrongCloud(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, tc.ErrorMatches, `validating credential "stratus/bob/foobar" for cloud "dummy": cloud "stratus" not valid`)
}

func (s *ModelCredentialSuite) TestValidateCloudCredentialWrongAuthType(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, tc.ErrorMatches, `validating credential "dummy/bob/foobar" for cloud "dummy": supported auth-types \["empty"\], "access-key" not supported`)
}

func (s *ModelCredentialSuite) TestValidateCloudCredentialModel(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	cred := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err = m.ValidateCloudCredential(tag, cred)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelCredentialSuite) TestSetCloudCredential(c *tc.C) {
	s.assertSetCloudCredential(c,
		names.NewCloudCredentialTag("dummy/bob/foobar"),
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialNoUpdate(c *tc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	m := s.assertSetCloudCredential(c,
		tag,
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	set, err := m.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	// This should be false as cloud credential change was an no-op.
	c.Assert(set, tc.IsFalse)

	// Check credential is still set.
	credentialTag, credentialSet := m.CloudCredentialTag()
	c.Assert(credentialTag, tc.DeepEquals, tag)
	c.Assert(credentialSet, tc.IsTrue)
	cred, credentialSet, err := m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsTrue)
	stateCred, err := s.State.CloudCredential(credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred, tc.DeepEquals, stateCred)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialInvalidCredentialContent(c *tc.C) {
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	credential := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.InvalidateCloudCredential(tag, "test")
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorMatches, `credential "dummy/bob/foobar" not valid`)
	c.Assert(set, tc.IsFalse)

	credentialTag, credentialSet := m.CloudCredentialTag()
	// Make sure no credential is set.
	c.Assert(credentialTag, tc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, tc.IsFalse)
	_, credentialSet, err = m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsFalse)
}

func (s *ModelCredentialSuite) TestSetCloudCredentialInvalidCredentialForModel(c *tc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewCloudCredentialTag("stratus/bob/foobar")
	credential := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"access-key": "someverysecretaccesskey",
		"secret-key": "someverysercretplainkey",
	})
	err = s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorMatches, `cloud "stratus" not valid`)
	c.Assert(set, tc.IsFalse)

	credentialTag, credentialSet := m.CloudCredentialTag()
	// Make sure no credential is set.
	c.Assert(credentialTag, tc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, tc.IsFalse)
	_, credentialSet, err = m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsFalse)
}

func (s *ModelCredentialSuite) TestWatchModelCredential(c *tc.C) {
	// Credential to use in this test.
	tag := names.NewCloudCredentialTag("dummy/bob/foobar")
	credential := cloud.NewCredential(cloud.EmptyAuthType, nil)
	err := s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, tc.ErrorIsNil)

	// Model with credential watcher for this test.
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	w := m.WatchModelCredential()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event.
	wc.AssertOneChange()

	// Check the watcher reacts to credential reference changes.
	set, err := m.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(set, tc.IsTrue)
	wc.AssertOneChange()

	// Check the watcher does not react to other changes on this model.
	err = m.SetDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Check that changes on another model do not affect this watcher.
	st := s.addModel(c, "abcmodel", s.credentialTag)
	defer st.Close()
	anotherM, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	set, err = anotherM.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(set, tc.IsTrue)
	wc.AssertNoChange()
}

func (s *ModelCredentialSuite) assertSetCloudCredential(c *tc.C, tag names.CloudCredentialTag, credential cloud.Credential) *state.Model {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	credentialTag, credentialSet := m.CloudCredentialTag()
	// Make sure no credential is set.
	c.Assert(credentialTag, tc.DeepEquals, names.CloudCredentialTag{})
	c.Assert(credentialSet, tc.IsFalse)
	_, credentialSet, err = m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsFalse)

	err = s.State.UpdateCloudCredential(tag, credential)
	c.Assert(err, tc.ErrorIsNil)

	set, err := m.SetCloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(set, tc.IsTrue)

	// Check credential is set.
	credentialTag, credentialSet = m.CloudCredentialTag()
	c.Assert(credentialTag, tc.DeepEquals, tag)
	c.Assert(credentialSet, tc.IsTrue)
	cred, credentialSet, err := m.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsTrue)
	stateCred, err := s.State.CloudCredential(credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred, tc.DeepEquals, stateCred)
	return m
}

func (s *ModelCredentialSuite) createCloudCredential(c *tc.C, credentialName string) names.CloudCredentialTag {
	// Cloud name is always "dummy" as deep within the testing infrastructure,
	// we create a testing controller on a cloud "dummy".
	// Test cloud "dummy" only allows credentials with an empty auth type.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", "dummy", s.Owner.Id(), credentialName))
	err := s.State.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, tc.ErrorIsNil)
	return tag
}

func (s *ModelCredentialSuite) addModel(c *tc.C, modelName string, tag names.CloudCredentialTag) *state.State {
	uuid, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": modelName,
		"uuid": uuid.String(),
	})
	_, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   tag.Owner(),
		CloudCredential:         tag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	return st
}

func (s *ModelCredentialSuite) TestInvalidateModelCredentialTouchesAllCredentialModels(c *tc.C) {
	// This test checks that all models are affected when one of them invalidates a credential they all use...

	// 1. create a credential
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)

	// 2. create some models to use it
	modelUUIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		modelUUIDs[i] = assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), fmt.Sprintf("model-for-cloud%v", i))
	}

	// 3. invalidate credential
	oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	defer helper.Release()
	c.Assert(oneModelState.State().InvalidateModelCredential("testing invalidate for all credential models"), tc.ErrorIsNil)

	// 4. check all models are suspended
	for _, uuid := range modelUUIDs {
		assertModelStatus(c, s.StatePool, uuid, status.Suspended)
		assertModelHistories(c, s.StatePool, uuid, status.Suspended, status.Available)
	}
}

func (s *ModelCredentialSuite) TestSetCredentialRevertsModelStatus(c *tc.C) {
	// 1. create a credential
	cloudName, credentialOwner, credentialTag := assertCredentialCreated(c, s.ConnSuite)

	// 2. create some models to use it
	validModelStatuses := []status.Status{
		status.Available,
		status.Busy,
		status.Destroying,
		status.Error,
	}
	desiredNumber := len(validModelStatuses)

	modelUUIDs := make([]string, desiredNumber)
	for i := 0; i < desiredNumber; i++ {
		modelUUIDs[i] = assertModelCreated(c, s.ConnSuite, cloudName, credentialTag, credentialOwner.Tag(), fmt.Sprintf("model-for-cloud%v", i))
		oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[i])
		c.Assert(err, tc.ErrorIsNil)
		defer helper.Release()
		if validModelStatuses[i] != status.Available {
			// any model would be in 'available' status on setup.
			err = oneModelState.SetStatus(status.StatusInfo{Status: validModelStatuses[i]})
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(oneModelState.Refresh(), tc.ErrorIsNil)
		}
		if i == desiredNumber-1 {
			// 3. invalidate credential on last model
			c.Assert(oneModelState.State().InvalidateModelCredential("testing"), tc.ErrorIsNil)
		}
	}

	// 4. check model is suspended
	for i := 0; i < desiredNumber; i++ {
		assertModelStatus(c, s.StatePool, modelUUIDs[i], status.Suspended)
		if validModelStatuses[i] == status.Available {
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Suspended, status.Available)
		} else {
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Suspended, validModelStatuses[i], status.Available)
		}
	}

	// 5. create another credential on the same cloud
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
		Name:     "uncle",
	})
	anotherCredentialTag := createCredential(c, s.ConnSuite, cloudName, owner.Name(), "barfoo")

	for i := 0; i < desiredNumber; i++ {
		oneModelState, helper, err := s.StatePool.GetModel(modelUUIDs[i])
		c.Assert(err, tc.ErrorIsNil)
		defer helper.Release()

		isSet, err := oneModelState.SetCloudCredential(anotherCredentialTag)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(isSet, tc.IsTrue)

		// 5. Check model status is reverted
		if validModelStatuses[i] == status.Available {
			assertModelStatus(c, s.StatePool, modelUUIDs[i], status.Available)
			assertModelHistories(c, s.StatePool, modelUUIDs[i], status.Available, status.Suspended, status.Available)
		} else {
			assertModelStatus(c, s.StatePool, modelUUIDs[i], validModelStatuses[i])
			assertModelHistories(c, s.StatePool, modelUUIDs[i], validModelStatuses[i], status.Suspended, validModelStatuses[i], status.Available)
		}
	}
}

func assertModelHistories(c *tc.C, pool *state.StatePool, testModelUUID string, expected ...status.Status) []status.StatusInfo {
	aModel, helper, err := pool.GetModel(testModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer helper.Release()
	statusHistories, err := aModel.StatusHistory(status.StatusHistoryFilter{Size: 100})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusHistories, tc.HasLen, len(expected))
	for i, one := range expected {
		c.Assert(statusHistories[i].Status, tc.Equals, one)
	}
	return statusHistories
}
