// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type CredentialModelsSuite struct {
	ConnSuite

	credentialTag names.CloudCredentialTag
	abcModelTag   names.ModelTag
}

func TestCredentialModelsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &CredentialModelsSuite{})
}

func (s *CredentialModelsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	s.credentialTag = s.createCloudCredential(c, "foobar")
	s.abcModelTag = s.addModel(c, "abcmodel", s.credentialTag)
}

func (s *CredentialModelsSuite) createCloudCredential(c *tc.C, credentialName string) names.CloudCredentialTag {
	// Cloud name is always "dummy" as deep within the testing infrastructure,
	// we create a testing controller on a cloud "dummy".
	// Test cloud "dummy" only allows credentials with an empty auth type.
	tag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", "dummy", s.Owner.Id(), credentialName))
	err := s.State.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, tc.ErrorIsNil)
	return tag
}

func (s *CredentialModelsSuite) addModel(c *tc.C, modelName string, tag names.CloudCredentialTag) names.ModelTag {
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
	defer st.Close()
	return names.NewModelTag(uuid.String())
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccess(c *tc.C) {
	out, err := s.State.CredentialModelsAndOwnerAccess(s.credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []state.CredentialOwnerModelAccess{
		{ModelName: "abcmodel", OwnerAccess: permission.AdminAccess, ModelUUID: s.abcModelTag.Id()},
	})
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccessMany(c *tc.C) {
	// add another model with the same credential
	xyzModelTag := s.addModel(c, "xyzmodel", s.credentialTag)

	// add another model with a different credential - should not be in the output.
	anotherCredential := s.createCloudCredential(c, "another")
	s.addModel(c, "dontshow", anotherCredential)

	out, err := s.State.CredentialModelsAndOwnerAccess(s.credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.SameContents, []state.CredentialOwnerModelAccess{
		{ModelName: "abcmodel", OwnerAccess: permission.AdminAccess, ModelUUID: s.abcModelTag.Id()},
		{ModelName: "xyzmodel", OwnerAccess: permission.AdminAccess, ModelUUID: xyzModelTag.Id()},
	})
}

func (s *CredentialModelsSuite) TestCredentialModelsAndOwnerAccessNoModels(c *tc.C) {
	anotherCredential := s.createCloudCredential(c, "another")

	out, err := s.State.CredentialModelsAndOwnerAccess(anotherCredential)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(out, tc.HasLen, 0)
}

func (s *CredentialModelsSuite) TestCredentialModels(c *tc.C) {
	out, err := s.State.CredentialModels(s.credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, map[string]string{s.abcModelTag.Id(): "abcmodel"})
}

func (s *CredentialModelsSuite) TestCredentialModelsExcludesDeadModels(c *tc.C) {
	checkModels := func(expected ...string) {
		out, err := s.State.CredentialModels(s.credentialTag)
		c.Assert(err, tc.ErrorIsNil)

		var obtained []string
		for k := range out {
			obtained = append(obtained, k)
		}
		c.Assert(obtained, tc.SameContents, expected)
	}

	// Add another model with the same credential.
	xyzModelTag := s.addModel(c, "xyzmodel", s.credentialTag)
	checkModels(s.abcModelTag.Id(), xyzModelTag.Id())

	// Set one of the models to Dead.
	m, r, err := s.StatePool.GetModel(s.abcModelTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	defer r.Release()

	err = m.SetDead()
	c.Assert(err, tc.ErrorIsNil)

	checkModels(xyzModelTag.Id())
}

func (s *CredentialModelsSuite) TestCredentialNoModels(c *tc.C) {
	anotherCredential := s.createCloudCredential(c, "another")

	out, err := s.State.CredentialModels(anotherCredential)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(out, tc.HasLen, 0)
}
