// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/annotations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type annotationSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	annotationsAPI *annotations.API
	authorizer     apiservertesting.FakeAuthorizer
}

func TestAnnotationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &annotationSuite{})
}

func (s *annotationSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.annotationsAPI, err = annotations.NewAPI(facadetest.Context{
		State_: s.State,
		Auth_:  s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *annotationSuite) TestModelAnnotations(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.testSetGetEntitiesAnnotations(c, model.Tag())
}

func (s *annotationSuite) TestMachineAnnotations(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.testSetGetEntitiesAnnotations(c, machine.Tag())

	// on machine removal
	err := machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, machine.Tag())
}

func (s *annotationSuite) TestCharmAnnotations(c *tc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress", URL: "local:wordpress-1"})
	s.testSetGetEntitiesAnnotations(c, charm.Tag())
}

func (s *annotationSuite) TestApplicationAnnotations(c *tc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: charm,
	})
	s.testSetGetEntitiesAnnotations(c, wordpress.Tag())

	// on application removal
	err := wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, wordpress.Tag())
}

func (s *annotationSuite) assertAnnotationsRemoval(c *tc.C, tag names.Tag) {
	entity := tag.String()
	entities := params.Entities{[]params.Entity{{entity}}}
	ann := s.annotationsAPI.Get(entities)
	c.Assert(ann.Results, tc.HasLen, 1)

	aResult := ann.Results[0]
	c.Assert(aResult.EntityTag, tc.DeepEquals, entity)
	c.Assert(aResult.Annotations, tc.HasLen, 0)
}

func (s *annotationSuite) TestInvalidEntityAnnotations(c *tc.C) {
	entity := "charm-invalid"
	entities := params.Entities{[]params.Entity{{entity}}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		params.AnnotationsSet{Annotations: constructSetParameters([]string{entity}, annotations)})
	c.Assert(setResult.OneError().Error(), tc.Matches, ".*permission denied.*")

	got := s.annotationsAPI.Get(entities)
	c.Assert(got.Results, tc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, tc.DeepEquals, entity)
	c.Assert(aResult.Error.Error.Error(), tc.Matches, ".*permission denied.*")
}

func (s *annotationSuite) TestUnitAnnotations(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: wordpress,
		Machine:     machine,
	})
	s.testSetGetEntitiesAnnotations(c, unit.Tag())

	// on unit removal
	err := unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, wordpress.Tag())
}

func (s *annotationSuite) makeRelation(c *tc.C) (*state.Application, *state.Relation) {
	s1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application1",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)

	s2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application2",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := s2.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)

	relation := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(relation, tc.NotNil)
	return s1, relation
}

// Cannot annotate relations...
func (s *annotationSuite) TestRelationAnnotations(c *tc.C) {
	_, relation := s.makeRelation(c)

	tag := relation.Tag().String()
	entity := params.Entity{tag}
	entities := params.Entities{[]params.Entity{entity}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		params.AnnotationsSet{Annotations: constructSetParameters([]string{tag}, annotations)})
	c.Assert(setResult.OneError().Error(), tc.Matches, ".*does not support annotations.*")

	got := s.annotationsAPI.Get(entities)
	c.Assert(got.Results, tc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, tc.DeepEquals, tag)
	c.Assert(aResult.Error.Error.Error(), tc.Matches, ".*does not support annotations.*")
}

func constructSetParameters(
	entities []string,
	annotations map[string]string) []params.EntityAnnotations {
	result := []params.EntityAnnotations{}
	for _, entity := range entities {
		one := params.EntityAnnotations{
			EntityTag:   entity,
			Annotations: annotations,
		}
		result = append(result, one)
	}
	return result
}

func (s *annotationSuite) TestMultipleEntitiesAnnotations(c *tc.C) {
	s1, relation := s.makeRelation(c)

	rTag := relation.Tag()
	rEntity := rTag.String()
	sTag := s1.Tag()
	sEntity := sTag.String()

	entities := []string{
		sEntity, //application: expect success in set/get
		rEntity, //relation:expect failure in set/get - cannot annotate relations
	}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, annotations)})
	c.Assert(setResult.Results, tc.HasLen, 1)

	oneError := setResult.Results[0].Error.Error()
	// Only attempt at annotate relation should have erred
	c.Assert(oneError, tc.Matches, fmt.Sprintf(".*%q.*", rTag))
	c.Assert(oneError, tc.Matches, ".*does not support annotations.*")

	got := s.annotationsAPI.Get(params.Entities{[]params.Entity{
		{rEntity},
		{sEntity}}})
	c.Assert(got.Results, tc.HasLen, 2)

	var rGet, sGet bool
	for _, aResult := range got.Results {
		if aResult.EntityTag == rTag.String() {
			rGet = true
			c.Assert(aResult.Error.Error.Error(), tc.Matches, ".*does not support annotations.*")
		} else {
			sGet = true
			c.Assert(aResult.EntityTag, tc.DeepEquals, sEntity)
			c.Assert(aResult.Annotations, tc.DeepEquals, annotations)
		}
	}
	// Both entities should have processed
	c.Assert(sGet, tc.IsTrue)
	c.Assert(rGet, tc.IsTrue)
}

func (s *annotationSuite) testSetGetEntitiesAnnotations(c *tc.C, tag names.Tag) {
	entity := tag.String()
	entities := []string{entity}
	for i, t := range clientAnnotationsTests {
		c.Logf("test %d. %s. entity %s", i, t.about, tag.Id())
		s.setupEntity(c, entities, t.initial)
		s.assertSetEntityAnnotations(c, entities, t.input, t.err)
		if t.err != "" {
			continue
		}
		aResult := s.assertGetEntityAnnotations(c, params.Entities{[]params.Entity{{entity}}}, entity, t.expected)
		s.cleanupEntityAnnotations(c, entities, aResult)
	}
}

func (s *annotationSuite) setupEntity(
	c *tc.C,
	entities []string,
	initialAnnotations map[string]string) {
	if initialAnnotations != nil {
		initialResult := s.annotationsAPI.Set(
			params.AnnotationsSet{
				Annotations: constructSetParameters(entities, initialAnnotations)})
		c.Assert(initialResult.Combine(), tc.ErrorIsNil)
	}
}

func (s *annotationSuite) assertSetEntityAnnotations(c *tc.C,
	entities []string,
	annotations map[string]string,
	expectedError string) {
	setResult := s.annotationsAPI.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, annotations)})
	if expectedError != "" {
		c.Assert(setResult.OneError().Error(), tc.Matches, expectedError)
	} else {
		c.Assert(setResult.Combine(), tc.ErrorIsNil)
	}
}

func (s *annotationSuite) assertGetEntityAnnotations(c *tc.C,
	entities params.Entities,
	entity string,
	expected map[string]string) params.AnnotationsGetResult {
	got := s.annotationsAPI.Get(entities)
	c.Assert(got.Results, tc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, tc.DeepEquals, entity)
	c.Assert(aResult.Annotations, tc.DeepEquals, expected)
	return aResult
}

func (s *annotationSuite) cleanupEntityAnnotations(c *tc.C,
	entities []string,
	aResult params.AnnotationsGetResult) {
	cleanup := make(map[string]string)
	for key := range aResult.Annotations {
		cleanup[key] = ""
	}
	cleanupResult := s.annotationsAPI.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, cleanup)})
	c.Assert(cleanupResult.Combine(), tc.ErrorIsNil)
}

var clientAnnotationsTests = []struct {
	about    string
	initial  map[string]string
	input    map[string]string
	expected map[string]string
	err      string
}{
	{
		about:    "test setting an annotation",
		input:    map[string]string{"mykey": "myvalue"},
		expected: map[string]string{"mykey": "myvalue"},
	},
	{
		about:    "test setting multiple annotations",
		input:    map[string]string{"key1": "value1", "key2": "value2"},
		expected: map[string]string{"key1": "value1", "key2": "value2"},
	},
	{
		about:    "test overriding annotations",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": "another-value"},
		expected: map[string]string{"mykey": "another-value"},
	},
	{
		about: "test setting an invalid annotation",
		input: map[string]string{"invalid.key": "myvalue"},
		err:   `.*: invalid key "invalid.key"`,
	},
}
