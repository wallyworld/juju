// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type AnnotationsSuite struct {
	ConnSuite
	// any entity that implements
	// state.GlobalEntity will do
	testEntity *state.Machine
}

func TestAnnotationsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &AnnotationsSuite{})
}

func (s *AnnotationsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.testEntity, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *AnnotationsSuite) TestSetAnnotationsInvalidKey(c *tc.C) {
	key := "tes.tkey"
	expected := "typo"
	err := s.setAnnotationResult(c, key, expected)
	c.Assert(errors.Cause(err), tc.ErrorMatches, ".*invalid key.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsCreate(c *tc.C) {
	s.createTestAnnotation(c)
}

func (s *AnnotationsSuite) createTestAnnotation(c *tc.C) string {
	key := "testkey"
	expected := "typo"
	s.assertSetAnnotation(c, key, expected)
	assertAnnotation(c, s.Model, s.testEntity, key, expected)
	return key
}

func (s *AnnotationsSuite) setAnnotationResult(c *tc.C, key, value string) error {
	annts := map[string]string{key: value}
	return s.Model.SetAnnotations(s.testEntity, annts)
}

func (s *AnnotationsSuite) assertSetAnnotation(c *tc.C, key, value string) {
	err := s.setAnnotationResult(c, key, value)
	c.Assert(err, tc.ErrorIsNil)
}

func assertAnnotation(c *tc.C, model *state.Model, entity state.GlobalEntity, key, expected string) {
	value, err := model.Annotation(entity, key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value, tc.DeepEquals, expected)
}

func (s *AnnotationsSuite) TestSetAnnotationsUpdate(c *tc.C) {
	key := s.createTestAnnotation(c)
	updated := "fixed"

	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.Model, s.testEntity, key, updated)
}

func (s *AnnotationsSuite) TestSetAnnotationsRemove(c *tc.C) {
	key := s.createTestAnnotation(c)
	updated := ""
	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.Model, s.testEntity, key, updated)

	annts, err := s.Model.Annotations(s.testEntity)
	c.Assert(err, tc.ErrorIsNil)

	// we are expecting not to find this key...
	for akey := range annts {
		c.Assert(akey == key, tc.IsFalse)
	}
}

func (s *AnnotationsSuite) TestSetAnnotationsDestroyedEntity(c *tc.C) {
	key := s.createTestAnnotation(c)

	err := s.testEntity.ForceDestroy(dontWait)
	c.Assert(err, tc.ErrorIsNil)
	err = s.testEntity.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.testEntity.Remove()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.Machine(s.testEntity.Id())
	c.Assert(errors.Cause(err), tc.ErrorMatches, ".*not found.*")

	annts, err := s.Model.Annotations(s.testEntity)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(annts, tc.DeepEquals, map[string]string{})

	annts[key] = "oops"
	err = s.Model.SetAnnotations(s.testEntity, annts)
	c.Assert(errors.Cause(err), tc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, tc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsNonExistentEntity(c *tc.C) {
	annts := map[string]string{"key": "oops"}
	err := s.Model.SetAnnotations(state.MockGlobalEntity{}, annts)

	c.Assert(errors.Cause(err), tc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, tc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsConcurrently(c *tc.C) {
	key := "conkey"
	first := "alpha"
	last := "omega"

	setAnnotations := func() {
		s.assertSetAnnotation(c, key, first)
		assertAnnotation(c, s.Model, s.testEntity, key, first)
	}
	defer state.SetBeforeHooks(c, s.State, setAnnotations).Check()
	s.assertSetAnnotation(c, key, last)
	assertAnnotation(c, s.Model, s.testEntity, key, last)
}

type AnnotationsModelSuite struct {
	ConnSuite
}

func TestAnnotationsModelSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &AnnotationsModelSuite{})
}

func (s *AnnotationsModelSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.ConnSuite.PatchValue(&state.TagToCollectionAndId, func(st *state.State, tag names.Tag) (string, interface{}, error) {
		return "", nil, errors.Errorf("this error should not be reached with current implementation %v", tag)
	})
}

func (s *AnnotationsModelSuite) TestSetAnnotationsDestroyedModel(c *tc.C) {
	model, st := s.createTestModel(c)
	defer st.Close()

	key := "key"
	expected := "oops"
	annts := map[string]string{key: expected}
	err := model.SetAnnotations(model, annts)
	c.Assert(err, tc.ErrorIsNil)
	assertAnnotation(c, model, model, key, expected)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = st.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)
	err = st.Close()
	c.Assert(err, tc.ErrorIsNil)

	expected = "fail"
	annts[key] = expected
	err = s.Model.SetAnnotations(model, annts)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "model.* no longer exists")
	c.Assert(err, tc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsModelSuite) createTestModel(c *tc.C) (*state.Model, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:        state.ModelTypeIAAS,
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	return model, st
}
