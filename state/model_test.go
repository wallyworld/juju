// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/storage"
)

type ModelSuite struct {
	ConnSuite
}

func TestModelSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelSuite{})
}

func (s *ModelSuite) TestModel(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.IsControllerModel(), tc.IsTrue)

	expectedTag := names.NewModelTag(model.UUID())
	c.Assert(model.Tag(), tc.Equals, expectedTag)
	c.Assert(model.ControllerTag(), tc.Equals, s.State.ControllerTag())
	c.Assert(model.Name(), tc.Equals, "testmodel")
	c.Assert(model.Owner(), tc.Equals, s.Owner)
	c.Assert(model.Life(), tc.Equals, state.Alive)
	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeNone)
}

func (s *ModelSuite) TestModelDestroy(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestModelDestroyWithoutVolumes(c *tc.C) {
	//https://bugs.launchpad.net/juju/+bug/1800872
	// Models introduced in 2.1 and then upgraded to 2.2 don't have Volumes or Filesystem attributes
	// on their modelEntitiesRefs documents
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelEntities, closer := state.GetCollection(s.State, state.ModelEntityRefsC)
	defer closer()
	rawModelEntities := modelEntities.Writeable().Underlying()
	err = rawModelEntities.Update(bson.M{"_id": model.UUID()}, bson.M{"$unset": bson.M{"volumes": 1, "filesystems": 1}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestSetPassword(c *tc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Model()
	})
}

func (s *ModelSuite) TestNewModelSameUserSameNameFails(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), tc.IsTrue)

	// Attempt to create another model with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	errMsg := fmt.Sprintf("model %q for %s already exists", cfg2.Name(), owner.Id())
	c.Assert(err, tc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), tc.IsTrue)

	// Remove the first model.
	model1, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model1.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Destroy only sets the model to dying and RemoveDyingModel can
	// only be called on a dead model. Normally, the environ's lifecycle
	// would be set to dead after machines and applications have been cleaned up.
	err = model1.SetDead()
	c.Assert(err, tc.ErrorIsNil)
	err = st1.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2, tc.NotNil)
	c.Assert(st2, tc.NotNil)
}

func (s *ModelSuite) TestNewCAASModelDifferentUser(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()
	owner2 := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), tc.IsTrue)

	// Attempt to create another model with a different UUID and owner
	// but the name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner2,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2.UniqueIndexExists(), tc.IsTrue)
}

func (s *ModelSuite) TestNewCAASModelSameUserFails(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	model, st1, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st1.Close()
	c.Assert(model.UniqueIndexExists(), tc.IsTrue)

	// Attempt to create another model with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	errMsg := fmt.Sprintf("model %q for %s already exists", cfg2.Name(), owner.Name())
	c.Assert(err, tc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), tc.IsTrue)

	// Remove the first model.
	model1, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model1.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Destroy only sets the model to dying and RemoveDyingModel can
	// only be called on a dead model. Normally, the environ's lifecycle
	// would be set to dead after machines and applications have been cleaned up.
	err = model1.SetDead()
	c.Assert(err, tc.ErrorIsNil)
	err = st1.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	// We should now be able to create the other model.
	model2, st2, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg2,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st2.Close()
	c.Assert(model2, tc.NotNil)
	c.Assert(st2, tc.NotNil)
}

func (s *ModelSuite) TestNewModelMissingType(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")
	_, _, err := s.Controller.NewModel(state.ModelArgs{
		// No type
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, "empty Type not valid")

}

func (s *ModelSuite) TestNewModel(c *tc.C) {
	cfg, uuid := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.IsControllerModel(), tc.IsFalse)
	defer st.Close()

	modelTag := names.NewModelTag(uuid)
	assertModelMatches := func(model *state.Model) {
		c.Assert(model.UUID(), tc.Equals, modelTag.Id())
		c.Assert(model.Type(), tc.Equals, state.ModelTypeIAAS)
		c.Assert(model.Tag(), tc.Equals, modelTag)
		c.Assert(model.ControllerTag(), tc.Equals, s.State.ControllerTag())
		c.Assert(model.Owner(), tc.Equals, owner)
		c.Assert(model.Name(), tc.Equals, "testing")
		c.Assert(model.Life(), tc.Equals, state.Alive)
		c.Assert(model.CloudRegion(), tc.Equals, "dummy-region")
	}
	assertModelMatches(model)

	model, ph, err := s.StatePool.GetModel(uuid)
	c.Assert(err, tc.ErrorIsNil)
	defer ph.Release()
	assertModelMatches(model)

	model, err = st.Model()
	c.Assert(err, tc.ErrorIsNil)
	assertModelMatches(model)

	// Check that the cloud's model count is incremented.
	testCloud, err := s.State.Cloud("dummy")
	c.Assert(err, tc.ErrorIsNil)
	refCount, err := state.CloudModelRefCount(st, testCloud.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 2)

	// Since the model tag for the State connection is different,
	// asking for this model through FindEntity returns a not found error.
	_, err = s.State.FindEntity(modelTag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(modelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Tag(), tc.Equals, modelTag)

	// Ensure the model is functional by adding a machine
	_, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the default model was created.
	_, err = st.SpaceByName(network.AlphaSpaceName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelSuite) TestNewModelRegionNameEscaped(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dotty.region",
		Config:                  cfg,
		Owner:                   names.NewUserTag("test@remote"),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()
	c.Assert(model.CloudRegion(), tc.Equals, "dotty.region")
}

func (s *ModelSuite) TestNewModelImportingMode(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		MigrationMode:           state.MigrationModeImporting,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()

	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeImporting)
}

func (s *ModelSuite) TestSetMigrationMode(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

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

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeExporting)
}

func (s *ModelSuite) TestModelExists(c *tc.C) {
	modelExists, err := s.State.ModelExists(s.State.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelExists, tc.IsTrue)
}

func (s *ModelSuite) TestModelExistsNoModel(c *tc.C) {
	modelExists, err := s.State.ModelExists("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelExists, tc.IsFalse)
}

func (s *ModelSuite) TestSLA(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

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

	level, err := st.SLALevel()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(level, tc.Equals, "unsupported")
	c.Assert(model.SLACredential(), tc.DeepEquals, []byte{})
	for _, goodLevel := range []string{"unsupported", "essential", "standard", "advanced"} {
		err = st.SetSLA(goodLevel, "bob", []byte("auth "+goodLevel))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(model.Refresh(), tc.ErrorIsNil)
		level, err = st.SLALevel()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(level, tc.Equals, goodLevel)
		c.Assert(model.SLALevel(), tc.Equals, goodLevel)
		c.Assert(model.SLAOwner(), tc.Equals, "bob")
		c.Assert(model.SLACredential(), tc.DeepEquals, []byte("auth "+goodLevel))
	}

	defaultLevel, err := state.NewSLALevel("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(defaultLevel, tc.Equals, state.SLAUnsupported)

	err = model.SetSLA("nope", "nobody", []byte("auth nope"))
	c.Assert(err, tc.ErrorMatches, `.*SLA level "nope" not valid.*`)

	c.Assert(model.SLALevel(), tc.Equals, "advanced")
	c.Assert(model.SLAOwner(), tc.Equals, "bob")
	c.Assert(model.SLACredential(), tc.DeepEquals, []byte("auth advanced"))
	slaCreds, err := st.SLACredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(slaCreds, tc.DeepEquals, []byte("auth advanced"))
}

func (s *ModelSuite) TestConfigForOtherModel(c *tc.C) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "other"})
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Obtain another instance of the model via the StatePool
	model, ph, err := s.StatePool.GetModel(otherModel.UUID())
	c.Assert(err, tc.ErrorIsNil)
	defer ph.Release()

	conf, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conf.Name(), tc.Equals, "other")
	c.Assert(conf.UUID(), tc.Equals, otherModel.UUID())
}

func (s *ModelSuite) TestAllUnits(c *tc.C) {
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "mysql",
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	units, err := model.AllUnits()
	c.Assert(err, tc.ErrorIsNil)

	var unitNames []string
	for _, u := range units {
		if !u.ShouldBeAssigned() {
			c.Fail()
		}
		unitNames = append(unitNames, u.Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, tc.DeepEquals, []string{
		"mysql/0", "wordpress/0", "wordpress/1",
	})
}

func (s *ModelSuite) TestMetrics(c *tc.C) {
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "wordpress",
	})
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "mysql",
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	// Add a machine/unit/application and destroy it, to
	// ensure we're only counting entities that are alive.
	m := s.Factory.MakeMachine(c, &factory.MachineParams{})
	err := m.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	one := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "one",
	})
	u := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})
	err = one.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	obtained, err := model.Metrics()
	c.Assert(err, tc.ErrorIsNil)

	expected := state.ModelMetrics{
		ApplicationCount: "2",
		MachineCount:     "3",
		UnitCount:        "3",
		CloudName:        "dummy",
		CloudRegion:      "dummy-region",
		Provider:         "dummy",
		UUID:             s.Model.UUID(),
		ControllerUUID:   s.Model.ControllerUUID(),
	}

	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *ModelSuite) TestAllEndpointBindings(c *tc.C) {
	oneSpace := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})
	app := state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": oneSpace.Id()})

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	listBindings, err := model.AllEndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(listBindings, tc.HasLen, 1)

	expected := map[string]string{
		"":                network.AlphaSpaceId,
		"cache":           network.AlphaSpaceId,
		"foo-bar":         network.AlphaSpaceId,
		"db-client":       network.AlphaSpaceId,
		"admin-api":       network.AlphaSpaceId,
		"url":             network.AlphaSpaceId,
		"logging-dir":     network.AlphaSpaceId,
		"monitoring-port": network.AlphaSpaceId,
		"db":              oneSpace.Id(),
	}
	c.Assert(listBindings[app.Name()].Map(), tc.DeepEquals, expected)
}

func (s *ModelSuite) TestAllEndpointBindingsSpaceNames(c *tc.C) {
	oneSpace := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": oneSpace.Id()})

	spaceNames, err := s.State.AllEndpointBindingsSpaceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaceNames.Size(), tc.Equals, 2)
	c.Assert(spaceNames.SortedValues(), tc.DeepEquals, []string{"alpha", "one"})
}

func (s *ModelSuite) TestAllEndpointBindingsSpaceNamesWithoutAnySpaces(c *tc.C) {
	spaceNames, err := s.State.AllEndpointBindingsSpaceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaceNames.Size(), tc.Equals, 0)
}

// createTestModelConfig returns a new model config and its UUID for testing.
func (s *ModelSuite) createTestModelConfig(c *tc.C) (*config.Config, string) {
	return createTestModelConfig(c, s.modelTag.Id())
}

func createTestModelConfig(c *tc.C, controllerUUID string) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	}), uuid.String()
}

func (s *ModelSuite) TestModelConfigSameModelAsState(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	cfg, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.UUID(), tc.Equals, s.State.ModelUUID())
}

func (s *ModelSuite) TestModelConfigDifferentModelThanState(c *tc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	model, err := otherState.Model()
	c.Assert(err, tc.ErrorIsNil)
	cfg, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)
	uuid := cfg.UUID()
	c.Assert(uuid, tc.Equals, model.UUID())
	c.Assert(uuid, tc.Not(tc.Equals), s.State.ModelUUID())
}

func (s *ModelSuite) TestDestroyControllerModel(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyOtherModel(c *tc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(st2.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
	// Destroying an empty model also removes the name index doc.
	c.Assert(model.UniqueIndexExists(), tc.IsFalse)
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelFails(c *tc.C) {
	s.assertDestroyControllerNonEmptyModelFails(c, nil)
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelWithForceFails(c *tc.C) {
	force := true
	s.assertDestroyControllerNonEmptyModelFails(c, &force)
}

func (s *ModelSuite) assertDestroyControllerNonEmptyModelFails(c *tc.C, force *bool) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2, s.StatePool).MakeApplication(c, nil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{Force: force}), tc.ErrorMatches, "failed to destroy model: hosting 1 other model")
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Alive)
	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model2.Refresh(), tc.ErrorIsNil)
	c.Assert(model2.Life(), tc.Equals, state.Alive)
}

func (s *ModelSuite) TestDestroyControllerWithEmptyModel(c *tc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), tc.ErrorIsNil)
	c.Assert(controllerModel.Life(), tc.Equals, state.Dying)
	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	hostedModel, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostedModel.Refresh(), tc.ErrorIsNil)
	c.Logf("model %s, life %s", hostedModel.UUID(), hostedModel.Life())
	c.Assert(hostedModel.Life(), tc.Equals, state.Dying)
	c.Assert(st2.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(hostedModel.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModels(c *tc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2, s.StatePool).MakeApplication(c, nil)

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	// Cleanups for hosted model enqueued by controller model cleanups.
	assertNeedsCleanup(c, st2)
	assertCleanupRuns(c, st2)

	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model2.Life(), tc.Equals, state.Dying)

	c.Assert(st2.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), tc.ErrorIsNil)

	c.Assert(model2.Refresh(), tc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithResources(c *tc.C) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	assertModel := func(model *state.Model, st *state.State, life state.Life, expectedMachines, expectedApplications int) {
		c.Assert(model.Refresh(), tc.ErrorIsNil)
		c.Assert(model.Life(), tc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(machines, tc.HasLen, expectedMachines)

		applications, err := st.AllApplications()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(applications, tc.HasLen, expectedApplications)
	}

	// add some machines and applications
	otherModel, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = otherSt.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)

	ch := state.AddTestingCharm(c, otherSt, "dummy")
	args := state.AddApplicationArgs{
		Name:  application.Name(),
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	}
	_, err = otherSt.AddApplication(args)
	c.Assert(err, tc.ErrorIsNil)

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := true
	force := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		Force:               &force,
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), tc.ErrorIsNil)

	assertCleanupCountDirty(c, s.State, 4)
	assertAllMachinesDeadAndRemove(c, s.State)
	assertModel(controllerModel, s.State, state.Dying, 0, 0)

	err = s.State.ProcessDyingModel()
	c.Assert(errors.Is(err, stateerrors.HasHostedModelsError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `hosting 1 other model`)

	assertCleanupCount(c, otherSt, 3)
	assertAllMachinesDeadAndRemove(c, otherSt)
	assertModel(otherModel, otherSt, state.Dying, 0, 0)
	c.Assert(otherSt.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(otherSt.RemoveDyingModel(), tc.ErrorIsNil)

	c.Assert(otherModel.Refresh(), tc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) assertDestroyControllerAndHostedModelsWithPersistentStorage(c *tc.C, force *bool) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	// Add a unit with persistent storage, which will prevent Destroy
	// from succeeding on account of DestroyStorage being nil.
	otherFactory := factory.NewFactory(otherSt, s.StatePool)
	otherFactory.MakeUnit(c, &factory.UnitParams{
		Application: otherFactory.MakeApplication(c, &factory.ApplicationParams{
			Charm: otherFactory.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
				URL:  "ch:quantal/storage-block-1",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		Force:               force,
	})
	c.Assert(errors.Is(err, stateerrors.PersistentStorageError), tc.IsTrue)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithPersistentStorage(c *tc.C) {
	s.assertDestroyControllerAndHostedModelsWithPersistentStorage(c, nil)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithPersistentStorageWithForce(c *tc.C) {
	force := true
	s.assertDestroyControllerAndHostedModelsWithPersistentStorage(c, &force)
}

func (s *ModelSuite) TestDestroyControllerEmptyModelRace(c *tc.C) {
	defer s.Factory.MakeModel(c, nil).Close()

	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.Factory.MakeModel(c, nil).Close()
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerRemoveEmptyAddNonEmptyModel(c *tc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	// Simulate an empty model being removed, and a new non-empty
	// model being added, just before the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		// Destroy the empty model, which should move it right
		// along to Dead, and then remove it.
		model, err := st2.Model()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
		err = st2.RemoveDyingModel()
		c.Assert(err, tc.ErrorIsNil)

		// Add a new, non-empty model. This should still prevent
		// the controller from being destroyed.
		st3 := s.Factory.MakeModel(c, nil)
		defer st3.Close()
		factory.NewFactory(st3, s.StatePool).MakeApplication(c, nil)
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorMatches, "failed to destroy model: hosting 1 other model")
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelRace(c *tc.C) {
	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		st := s.Factory.MakeModel(c, nil)
		defer st.Close()
		factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	}).Check()

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorMatches, "failed to destroy model: hosting 1 other model")
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingRaceNoOp(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Simulate an model being destroyed by another client just before
	// the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	}).Check()

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingNoOp(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyModelNonEmpty(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Add a application to prevent the model from transitioning directly to Dead.
	s.Factory.MakeApplication(c, nil)

	c.Assert(m.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)

	// Since the model is only dying and not dead, the unique index is still there.
	c.Assert(m.UniqueIndexExists(), tc.IsTrue)
}

func (s *ModelSuite) assertDestroyModelPersistentStorage(c *tc.C, force *bool) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Add a unit with persistent storage, which will prevent Destroy
	// from succeeding on account of DestroyStorage being nil.
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	err = m.Destroy(state.DestroyModelParams{Force: force})
	c.Assert(errors.Is(err, stateerrors.PersistentStorageError), tc.IsTrue)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Alive)
}

func (s *ModelSuite) TestDestroyModelPersistentStorage(c *tc.C) {
	s.assertDestroyModelPersistentStorage(c, nil)
}

func (s *ModelSuite) TestDestroyModelPersistentStorageWithForce(c *tc.C) {
	force := true
	s.assertDestroyModelPersistentStorage(c, &force)
}

func (s *ModelSuite) TestDestroyModelNonPersistentStorage(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Add a unit with non-persistent storage, which should not prevent
	// Destroy from succeeding.
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "loop"},
			},
		}),
	})

	err = m.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelDestroyStorage(c *tc.C) {
	s.testDestroyModelDestroyStorage(c, true)
}

func (s *ModelSuite) TestDestroyModelReleaseStorage(c *tc.C) {
	s.testDestroyModelDestroyStorage(c, false)
}

func (s *ModelSuite) testDestroyModelDestroyStorage(c *tc.C, destroyStorage bool) {
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped"},
			},
		}),
	})

	err := s.Model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.Model.Refresh(), tc.ErrorIsNil)
	c.Assert(s.Model.Life(), tc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State) // destroy application
	assertCleanupRuns(c, s.State) // destroy unit
	assertCleanupRuns(c, s.State) // destroy/release storage

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	volume, err := sb.Volume(names.NewVolumeTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volume.Life(), tc.Equals, state.Dying)
	c.Assert(volume.Releasing(), tc.Equals, !destroyStorage)
}

func (s *ModelSuite) assertDestroyModelReleaseStorageUnreleasable(c *tc.C, force *bool) {
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.AddTestingCharm(c, "storage-block"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024, Pool: "modelscoped-unreleasable"},
			},
		}),
	})

	destroyStorage := false
	err := s.Model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage, Force: force})
	if force != nil && *force {
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(s.Model.Refresh(), tc.ErrorIsNil)
		c.Assert(s.Model.Life(), tc.Equals, state.Dying)
		assertNeedsCleanup(c, s.State)
	} else {
		expectedErr := fmt.Sprintf(`failed to destroy model: ` +
			`storage provider "modelscoped-unreleasable" does not support releasing storage`)
		c.Assert(err, tc.ErrorMatches, expectedErr)
		c.Assert(s.Model.Refresh(), tc.ErrorIsNil)
		c.Assert(s.Model.Life(), tc.Equals, state.Alive)
		assertDoesNotNeedCleanup(c, s.State)
	}
}

func (s *ModelSuite) TestDestroyModelReleaseStorageUnreleasable(c *tc.C) {
	s.assertDestroyModelReleaseStorageUnreleasable(c, nil)
}

func (s *ModelSuite) TestDestroyModelReleaseStorageUnreleasableWithForce(c *tc.C) {
	force := true
	s.assertDestroyModelReleaseStorageUnreleasable(c, &force)
}

func (s *ModelSuite) TestDestroyModelAddApplicationConcurrently(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	}).Check()

	c.Assert(m.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelAddMachineConcurrently(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st, s.StatePool).MakeMachine(c, nil)
	}).Check()

	c.Assert(m.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelEmpty(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(m.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)
	c.Assert(st.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestDestroyModelWithApplicationOffers(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	app := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))

	ao := state.NewApplicationOffers(s.State)
	offer, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = m.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Refresh(), tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, state.Dying)

	// Run the cleanups, check that the application and offer are
	// both removed.
	assertCleanupCount(c, s.State, 2)

	_, err = ao.ApplicationOffer(offer.OfferName)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = app.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ModelSuite) TestForceDestroySetsForceDestroyed(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.ForceDestroyed(), tc.Equals, false)

	force := true
	err = model.Destroy(state.DestroyModelParams{
		Force: &force,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = model.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(model.ForceDestroyed(), tc.Equals, true)
}

func (s *ModelSuite) TestDestroyWithTimeoutSetsTimeout(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.DestroyTimeout(), tc.IsNil)

	timeout := time.Minute
	err = model.Destroy(state.DestroyModelParams{
		Timeout: &timeout,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = model.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Life(), tc.Equals, state.Dying)
	got := model.DestroyTimeout()
	c.Assert(got, tc.NotNil)
	c.Assert(*got, tc.Equals, time.Minute)
}

func (s *ModelSuite) TestNonForceDestroy(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	noForce := false
	err = model.Destroy(state.DestroyModelParams{
		Force: &noForce,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = model.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(model.ForceDestroyed(), tc.Equals, false)
}

func (s *ModelSuite) TestProcessDyingServerModelTransitionDyingToDead(c *tc.C) {
	s.assertDyingModelTransitionDyingToDead(c, s.State)
}

func (s *ModelSuite) TestProcessDyingHostedModelTransitionDyingToDead(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	s.assertDyingModelTransitionDyingToDead(c, st)
}

func (s *ModelSuite) assertDyingModelTransitionDyingToDead(c *tc.C, st *state.State) {
	// Add a application to prevent the model from transitioning directly to Dead.
	// Add the application before getting the Model, otherwise we'll have to run
	// the transaction twice, and hit the hook point too early.
	app := factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	// ProcessDyingModel is called by a worker after Destroy is called. To
	// avoid a race, we jump the gun here and test immediately after the
	// environement was set to dead.
	defer state.SetAfterHooks(c, st, func() {
		c.Assert(model.Refresh(), tc.ErrorIsNil)
		c.Assert(model.Life(), tc.Equals, state.Dying)

		err := app.Destroy()
		c.Assert(err, tc.ErrorIsNil)

		c.Check(model.UniqueIndexExists(), tc.IsTrue)
		c.Assert(st.ProcessDyingModel(), tc.ErrorIsNil)
		c.Assert(st.RemoveDyingModel(), tc.ErrorIsNil)

		c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
		c.Check(model.UniqueIndexExists(), tc.IsFalse)
	}).Check()

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingModelWithMachinesAndApplicationsNoOp(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	// calling ProcessDyingModel on a live environ should fail.
	err := st.ProcessDyingModel()
	c.Assert(err, tc.ErrorMatches, "model is not dying")

	// add some machines and applications
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, nil)

	ch := state.AddTestingCharm(c, st, "dummy")
	args := state.AddApplicationArgs{
		Name:  application.Name(),
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	}
	_, err = st.AddApplication(args)
	c.Assert(err, tc.ErrorIsNil)

	assertModel := func(life state.Life, expectedMachines, expectedApplications int) {
		c.Assert(model.Refresh(), tc.ErrorIsNil)
		c.Assert(model.Life(), tc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(machines, tc.HasLen, expectedMachines)

		applications, err := st.AllApplications()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(applications, tc.HasLen, expectedApplications)
	}

	// Simulate processing a dying model after an model is set to
	// dying, but before the cleanup has removed machines and applications.
	defer state.SetAfterHooks(c, st, func() {
		assertModel(state.Dying, 1, 1)
		err := st.ProcessDyingModel()
		c.Assert(errors.Is(err, stateerrors.ModelNotEmptyError), tc.IsTrue)
		c.Assert(err, tc.ErrorMatches, `model not empty, found 1 machine, 1 application`)
	}).Check()

	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingModelWithVolumeBackedFilesystems(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	machine, err := st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{
				Pool: "modelscoped-block",
				Size: 123,
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)
	filesystems, err := sb.AllFilesystems()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystems, tc.HasLen, 1)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(errors.Is(err, stateerrors.PersistentStorageError), tc.IsTrue)

	destroyStorage := true
	c.Assert(model.Destroy(state.DestroyModelParams{
		DestroyStorage: &destroyStorage,
	}), tc.ErrorIsNil)

	err = sb.DetachFilesystem(machine.MachineTag(), names.NewFilesystemTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveFilesystemAttachment(machine.MachineTag(), names.NewFilesystemTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.DetachVolume(machine.MachineTag(), names.NewVolumeTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(machine.MachineTag(), names.NewVolumeTag("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), tc.ErrorIsNil)
	c.Assert(machine.Remove(), tc.ErrorIsNil)

	// The filesystem will be gone, but the volume is persistent and should
	// not have been removed.
	err = st.ProcessDyingModel()
	c.Assert(errors.Is(err, stateerrors.ModelNotEmptyError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `model not empty, found 1 volume, 1 filesystem`)
}

func (s *ModelSuite) TestProcessDyingModelWithVolumes(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	machine, err := st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped",
				Size: 123,
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)
	volumes, err := sb.AllVolumes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumes, tc.HasLen, 1)
	volumeTag := volumes[0].VolumeTag()

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(errors.Is(err, stateerrors.PersistentStorageError), tc.IsTrue)

	destroyStorage := true
	c.Assert(model.Destroy(state.DestroyModelParams{
		DestroyStorage: &destroyStorage,
	}), tc.ErrorIsNil)

	err = sb.DetachVolume(machine.MachineTag(), volumeTag, false)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(machine.MachineTag(), volumeTag, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), tc.ErrorIsNil)
	c.Assert(machine.Remove(), tc.ErrorIsNil)

	// The volume is persistent and should not have been removed along with
	// the machine it was attached to.
	err = st.ProcessDyingModel()
	c.Assert(errors.Is(err, stateerrors.ModelNotEmptyError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `model not empty, found 1 volume`)
}

func (s *ModelSuite) TestProcessDyingControllerModelWithHostedModelsNoOp(c *tc.C) {
	// Add a non-empty model to the controller.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	factory.NewFactory(st, s.StatePool).MakeApplication(c, nil)

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
	}), tc.ErrorIsNil)

	err = s.State.ProcessDyingModel()
	c.Assert(errors.Is(err, stateerrors.HasHostedModelsError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `hosting 1 other model`)

	c.Assert(controllerModel.Refresh(), tc.ErrorIsNil)
	c.Assert(controllerModel.Life(), tc.Equals, state.Dying)
}

func (s *ModelSuite) TestListModelUsers(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	expected := s.addModelUsers(c, s.State)
	obtained, err := model.Users()
	c.Assert(err, tc.IsNil)

	assertObtainedUsersMatchExpectedUsers(c, obtained, expected)
}

func (s *ModelSuite) TestListUsersIgnoredDeletedUsers(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	expectedUsers := s.addModelUsers(c, s.State)

	obtainedUsers, err := model.Users()
	c.Assert(err, tc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	lastUser := obtainedUsers[len(obtainedUsers)-1]
	err = s.State.RemoveUser(lastUser.UserTag)
	c.Assert(err, tc.ErrorIsNil)
	expectedAfterDeletion := obtainedUsers[:len(obtainedUsers)-1]

	obtainedUsers, err = model.Users()
	c.Assert(err, tc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedAfterDeletion)
}

func (s *ModelSuite) TestListUsersTwoModels(c *tc.C) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	otherModelState := s.Factory.MakeModel(c, nil)
	defer otherModelState.Close()
	otherModel, err := otherModelState.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Add users to both models
	expectedUsers := s.addModelUsers(c, s.State)
	expectedUsersOtherModel := s.addModelUsers(c, otherModelState)

	// test that only the expected users are listed for each model
	obtainedUsers, err := model.Users()
	c.Assert(err, tc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	obtainedUsersOtherModel, err := otherModel.Users()
	c.Assert(err, tc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherModel, expectedUsersOtherModel)

	// It doesn't matter how you obtain the Model.
	otherModel2, ph, err := s.StatePool.GetModel(otherModel.UUID())
	c.Assert(err, tc.ErrorIsNil)
	defer ph.Release()
	obtainedUsersOtherModel2, err := otherModel2.Users()
	c.Assert(err, tc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherModel2, expectedUsersOtherModel)
}

func (s *ModelSuite) addModelUsers(c *tc.C, st *state.State) (expected []permission.UserAccess) {
	// get the model owner
	testAdmin := names.NewUserTag("test-admin")
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	owner, err := st.UserAccess(testAdmin, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	return []permission.UserAccess{
		// we expect the owner to be an existing model user
		owner,
		// add new users to the model
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
	}
}

func assertObtainedUsersMatchExpectedUsers(c *tc.C, obtainedUsers, expectedUsers []permission.UserAccess) {
	c.Assert(len(obtainedUsers), tc.Equals, len(expectedUsers))
	expectedByUser := make(map[string]permission.UserAccess, len(expectedUsers))
	for _, access := range expectedUsers {
		expectedByUser[access.UserName] = access
	}
	for _, obtained := range obtainedUsers {
		expect := expectedByUser[obtained.UserName]
		// We shouldn't get the same entry again
		delete(expectedByUser, obtained.UserName)
		c.Check(obtained.Object.Id(), tc.Equals, expect.Object.Id())
		c.Check(obtained.UserTag, tc.Equals, expect.UserTag)
		c.Check(obtained.DisplayName, tc.Equals, expect.DisplayName)
		c.Check(obtained.CreatedBy, tc.Equals, expect.CreatedBy)
	}
	c.Check(expectedByUser, tc.DeepEquals, map[string]permission.UserAccess{})
}

func (s *ModelSuite) TestAllModelUUIDs(c *tc.C) {
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	obtained, err := s.State.AllModelUUIDs()
	c.Assert(err, tc.ErrorIsNil)
	expected := []string{
		s.State.ModelUUID(),
		st1.ModelUUID(),
		st2.ModelUUID(),
	}
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *ModelSuite) TestAllModelUUIDsExcludesDead(c *tc.C) {
	expected := []string{
		s.State.ModelUUID(),
	}

	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	m1, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	expectedWithAddition := append(expected, m1.UUID())
	obtained, err := s.State.AllModelUUIDs()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expectedWithAddition)

	err = m1.SetDead()
	c.Assert(err, tc.ErrorIsNil)

	obtained, err = s.State.AllModelUUIDs()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expected)

	obtained, err = s.State.AllModelUUIDsIncludingDead()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expectedWithAddition)
}

func (s *ModelSuite) TestHostedModelCount(c *tc.C) {
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 0)

	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 1)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 2)

	model1, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model1.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 1)

	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model2.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 0)
}

func (s *ModelSuite) TestNewModelEnvironVersion(c *tc.C) {
	v := 123
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		EnvironVersion: v,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), tc.Equals, v)
}

func (s *ModelSuite) TestSetEnvironVersion(c *tc.C) {
	v := 123
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		m, err := s.State.Model()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), tc.Equals, 0)
		err = m.SetEnvironVersion(v)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), tc.Equals, v)
	}).Check()

	err = m.SetEnvironVersion(v)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), tc.Equals, v)
}

func (s *ModelSuite) TestSetEnvironVersionCannotDecrease(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		m, err := s.State.Model()
		c.Assert(err, tc.ErrorIsNil)
		err = m.SetEnvironVersion(2)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.EnvironVersion(), tc.Equals, 2)
	}).Check()

	err = m.SetEnvironVersion(1)
	c.Assert(err, tc.ErrorMatches, `cannot set environ version to 1, which is less than the current version 2`)
	// m's cached version is only updated on success
	c.Assert(m.EnvironVersion(), tc.Equals, 0)

	err = m.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.EnvironVersion(), tc.Equals, 2)
}

func (s *ModelSuite) TestDestroyForceWorksWhenRemoteRelationScopesAreStuck(c *tc.C) {
	mysqlEps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	ms := s.Factory.MakeModel(c, nil)
	defer ms.Close()
	remoteApp, err := ms.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "t0",
		Endpoints:   mysqlEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	wordpress := state.AddTestingApplication(c, ms, "wordpress", state.AddTestingCharm(c, ms, "wordpress"))
	eps, err := ms.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := ms.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	f := factory.NewFactory(ms, s.StatePool)
	machine := f.MakeMachine(c, nil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
	localRelUnit, err := rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = localRelUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	remoteRelUnit, err := rel.RemoteUnit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	err = remoteRelUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Refetch the remoteapp to ensure that its relationcount is
	// current. Otherwise it just silently fails? (See errRefresh
	// handling in DestroyRemoteApplicationOperation.Build)
	err = remoteApp.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = remoteApp.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, remoteApp, state.Dying)

	err = wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = localRelUnit.LeaveScope()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	// Cleanups
	assertCleanupCount(c, ms, 1)

	// wordpress is kept around because the relation can't be removed.
	assertLife(c, wordpress, state.Dying)

	// Force-destroying the model cleans them up.
	model, err := ms.Model()
	c.Assert(err, tc.ErrorIsNil)
	force := true
	err = model.Destroy(state.DestroyModelParams{
		Force: &force,
	})
	c.Assert(err, tc.ErrorIsNil)

	assertCleanupCount(c, ms, 4)
	assertRemoved(c, wordpress)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(ms.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(ms.RemoveDyingModel(), tc.ErrorIsNil)
}

type ModelCloudValidationSuite struct {
	mgotesting.MgoSuite
}

func TestModelCloudValidationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelCloudValidationSuite{})
}

// TODO(axw) concurrency tests when we can modify the cloud definition,
// and update/remove credentials.

func (s *ModelCloudValidationSuite) TestNewModelDifferentCloud(c *tc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	aCloud := cloud.Cloud{
		Name:      "another",
		Type:      "dummy",
		AuthTypes: cloud.AuthTypes{"empty", "userpass"},
	}
	err = st.AddCloud(aCloud, owner.Name())
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err = cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, tc.ErrorIsNil)
	m, newSt, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "another",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer newSt.Close()
	c.Assert(m.CloudName(), tc.Equals, "another")
	cloudValue, err := m.Cloud()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudValue, tc.DeepEquals, aCloud)
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudRegion(c *tc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err = controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, `region "dummy-region" not found \(expected one of \["some-region"\]\)`)
}

func (s *ModelCloudValidationSuite) TestNewModelDefaultCloudRegion(c *tc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "dummy-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err = cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, tc.ErrorIsNil)
	m, newSt, err := controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer newSt.Close()
	c.Assert(m.CloudRegion(), tc.Equals, "dummy-region")
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudRegion(c *tc.C) {
	controller, owner := s.initializeState(c, []cloud.Region{{Name: "dummy-region"}, {Name: "dummy-region2"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err = controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, "missing cloud region not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudCredential(c *tc.C) {
	regions := []cloud.Region{{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	unknownCredentialTag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/unknown-credential")
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err = controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         unknownCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, `credential "dummy/test@remote/unknown-credential" not found`)
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredential(c *tc.C) {
	regions := []cloud.Region{{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err = controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, "missing CloudCredential not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredentialSupportsEmptyAuth(c *tc.C) {
	regions := []cloud.Region{
		{
			Name:             "dummy-region",
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
		},
	}
	controller, owner := s.initializeState(c, regions, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err = cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, tc.ErrorIsNil)
	_, newSt, err := controller.NewModel(state.ModelArgs{
		Type:      state.ModelTypeIAAS,
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	newSt.Close()
}

func (s *ModelCloudValidationSuite) TestNewModelOtherUserCloudCredential(c *tc.C) {
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	controller, _ := s.initializeState(
		c, nil, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	owner := factory.NewFactory(st, controller.StatePool()).MakeUser(c, nil).UserTag()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err = controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         controllerCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorMatches, `credential "dummy/test@remote/controller-credential" not found`)
}

func (s *ModelCloudValidationSuite) initializeState(
	c *tc.C,
	regions []cloud.Region,
	authTypes []cloud.AuthType,
	credentials map[names.CloudCredentialTag]cloud.Credential,
) (*state.Controller, names.UserTag) {
	owner := names.NewUserTag("test@remote")
	cfg, _ := createTestModelConfig(c, "")
	var controllerRegion string
	var controllerCredential names.CloudCredentialTag
	if len(regions) > 0 {
		controllerRegion = regions[0].Name
	}
	if len(credentials) > 0 {
		// pick an arbitrary credential
		for controllerCredential = range credentials {
		}
	}
	controllerCfg := testing.FakeControllerConfig()
	controller, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			Owner:                   owner,
			Config:                  cfg,
			CloudName:               "dummy",
			CloudRegion:             controllerRegion,
			CloudCredential:         controllerCredential,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: authTypes,
			Regions:   regions,
		},
		CloudCredentials: credentials,
		MongoSession:     s.Session,
		AdminPassword:    "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	return controller, owner
}

func assertCleanupRuns(c *tc.C, st *state.State) {
	err := st.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
}

func assertNeedsCleanup(c *tc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actual, tc.IsTrue)
}

func assertDoesNotNeedCleanup(c *tc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actual, tc.IsFalse)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func assertCleanupCount(c *tc.C, st *state.State, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		assertNeedsCleanup(c, st)
		assertCleanupRuns(c, st)
	}
	assertDoesNotNeedCleanup(c, st)
}

// assertCleanupCountDirty is the same as assertCleanupCount, but it
// checks that there are still cleanups to run.
func assertCleanupCountDirty(c *tc.C, st *state.State, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		assertNeedsCleanup(c, st)
		assertCleanupRuns(c, st)
	}
	assertNeedsCleanup(c, st)
}

// The provisioner will remove dead machines once their backing instances are
// stopped. For the tests, we remove them directly.
func assertAllMachinesDeadAndRemove(c *tc.C, st *state.State) {
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		c.Assert(err, tc.ErrorIsNil)
		if manual {
			continue
		}

		c.Assert(m.Life(), tc.Equals, state.Dead)
		c.Assert(m.Remove(), tc.ErrorIsNil)
	}
}
