// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type ModelSummariesSuite struct {
	ConnSuite
}

func TestModelSummariesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ModelSummariesSuite{})
}

func (s *ModelSummariesSuite) Setup4Models(c *tc.C) map[string]string {
	modelUUIDs := make(map[string]string)
	user1 := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "user1write",
		NoModelUser: true,
	})
	st1 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "user1model",
		Owner: user1.Tag(),
	})
	modelUUIDs["user1model"] = st1.ModelUUID()
	st1.Close()
	user2 := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "user2read",
		NoModelUser: true,
	})
	st2 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "user2model",
		Owner: user2.Tag(),
		Type:  state.ModelTypeCAAS,
	})
	modelUUIDs["user2model"] = st2.ModelUUID()
	f2 := factory.NewFactory(st2, s.StatePool)
	f2.MakeUnit(c, nil)
	st2.Close()
	user3 := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "user3admin",
		NoModelUser: true,
	})
	st3 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "user3model",
		Owner: user3.Tag(),
	})
	modelUUIDs["user3model"] = st3.ModelUUID()
	st3.Close()
	owner := s.Model.Owner()
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Regions: []cloud.Region{
			{
				Name:             "dummy-region",
				Endpoint:         "dummy-endpoint",
				IdentityEndpoint: "dummy-identity-endpoint",
				StorageEndpoint:  "dummy-storage-endpoint",
			},
		},
	}, s.Owner.Name())
	c.Assert(err, tc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	tag := names.NewCloudCredentialTag(fmt.Sprintf("stratus/%v/foobar", owner.Name()))
	err = s.State.UpdateCloudCredential(tag, cred)
	c.Assert(err, tc.ErrorIsNil)

	sharedSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "shared",
		// Owned by test-admin
		Owner:           owner,
		CloudName:       "stratus",
		CloudCredential: tag,
	})
	modelUUIDs["shared"] = sharedSt.ModelUUID()
	defer sharedSt.Close()
	sharedModel, err := sharedSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = sharedModel.AddUser(state.UserAccessSpec{
		User:      user1.UserTag(),
		CreatedBy: owner,
		Access:    "write",
	})
	c.Assert(err, tc.ErrorIsNil)
	// User 2 has read access to the shared model
	_, err = sharedModel.AddUser(state.UserAccessSpec{
		User:      user2.UserTag(),
		CreatedBy: owner,
		Access:    "read",
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = sharedModel.AddUser(state.UserAccessSpec{
		User:      user3.UserTag(),
		CreatedBy: owner,
		Access:    "admin",
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUIDs
}

func (s *ModelSummariesSuite) modelNamesForUser(c *tc.C, user string, isSuperuser bool) []string {
	tag := names.NewUserTag(user)
	modelQuery, closer, err := s.State.ModelQueryForUser(tag, isSuperuser)
	defer closer()
	c.Assert(err, tc.ErrorIsNil)
	var docs []struct {
		Name string `bson:"name"`
	}
	modelQuery.Select(bson.M{"name": 1})
	err = modelQuery.All(&docs)
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, 0)
	for _, doc := range docs {
		names = append(names, doc.Name)
	}
	sort.Strings(names)
	return names
}

func (s *ModelSummariesSuite) TestModelsForUserAdmin(c *tc.C) {
	s.Setup4Models(c)
	names := s.modelNamesForUser(c, s.Model.Owner().Name(), true)
	// Admin always gets to see all models
	c.Check(names, tc.DeepEquals, []string{"shared", "testmodel", "user1model", "user2model", "user3model"})
}

func (s *ModelSummariesSuite) TestModelsForSuperuserWithoutAll(c *tc.C) {
	s.Setup4Models(c)
	summaries, err := s.State.ModelSummariesForUser(s.Model.Owner(), false)
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, len(summaries))
	for i, summary := range summaries {
		names[i] = summary.Name
	}
	sort.Strings(names)
	c.Check(names, tc.DeepEquals, []string{"shared", "testmodel"})
}

func (s *ModelSummariesSuite) TestModelsForSuperuserWithAll(c *tc.C) {
	s.Setup4Models(c)
	summaries, err := s.State.ModelSummariesForUser(s.Model.Owner(), true)
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, len(summaries))
	access := make(map[string]string)
	isController := make(map[string]bool)
	for i, summary := range summaries {
		names[i] = summary.Name
		access[summary.Name] = string(summary.Access)
		isController[summary.Name] = summary.IsController
	}
	sort.Strings(names)
	c.Check(names, tc.DeepEquals, []string{"shared", "testmodel", "user1model", "user2model", "user3model"})
	c.Check(access, tc.DeepEquals, map[string]string{
		"shared":     "admin",
		"testmodel":  "admin",
		"user1model": "",
		"user2model": "",
		"user3model": "",
	})
	c.Check(isController, tc.DeepEquals, map[string]bool{
		"shared":     false,
		"testmodel":  true,
		"user1model": false,
		"user2model": false,
		"user3model": false,
	})
}

func (s *ModelSummariesSuite) TestModelsForUser1(c *tc.C) {
	// User1 is only added to the model they own and the shared model as write
	s.Setup4Models(c)
	names := s.modelNamesForUser(c, "user1write", false)
	c.Check(names, tc.DeepEquals, []string{"shared", "user1model"})
}

func (s *ModelSummariesSuite) TestModelsForUser2(c *tc.C) {
	// User2 is only added to the model they own and the shared model as read
	s.Setup4Models(c)
	names := s.modelNamesForUser(c, "user2read", false)
	c.Check(names, tc.DeepEquals, []string{"shared", "user2model"})
}

func (s *ModelSummariesSuite) TestModelsForUser3(c *tc.C) {
	// User2 is only added to the model they own and the shared model as admin
	s.Setup4Models(c)
	names := s.modelNamesForUser(c, "user3admin", false)
	c.Check(names, tc.DeepEquals, []string{"shared", "user3model"})
}

// NOTE: (jam 2017-12-11) We probably only ever stripped Importing models because there details might not be complete.
// We probably actually want to include importing models, and just handle when they don't have complete data.
func (s *ModelSummariesSuite) TestModelsForIgnoresImportingModels(c *tc.C) {
	s.Setup4Models(c)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "importing",
		"uuid": utils.MustNewUUID().String(),
		"type": state.ModelTypeIAAS,
	})
	_, stImporting, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   names.NewUserTag("user1write"),
		MigrationMode:           state.MigrationModeImporting,
		EnvironVersion:          s.Model.EnvironVersion(),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	defer stImporting.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Since the new model is importing, when we do the list we shouldn't see it.
	names := s.modelNamesForUser(c, "user3admin", false)
	c.Check(names, tc.DeepEquals, []string{"shared", "user3model"})
	// Superuser doesn't see importing models, either
	names = s.modelNamesForUser(c, s.Model.Owner().Name(), true)
	c.Check(names, tc.DeepEquals, []string{"shared", "testmodel", "user1model", "user2model", "user3model"})
}

func (s *ModelSummariesSuite) TestContainsConfigInformation(c *tc.C) {
	s.Setup4Models(c)
	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	// We don't guarantee the order of the summaries, but the data for each model should match the same
	// information you would get if you instantiate the model directly
	summaryA := summaries[0]
	model, ph, err := s.StatePool.GetModel(summaryA.UUID)
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	conf, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaryA.ProviderType, tc.Equals, conf.Type())
	version, ok := conf.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Check(summaryA.AgentVersion, tc.NotNil)
	c.Check(*summaryA.AgentVersion, tc.Equals, version)
}

func (s *ModelSummariesSuite) TestContainsProviderType(c *tc.C) {
	s.Setup4Models(c)
	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	// We don't guarantee the order of the summaries, but both should have the same ProviderType
	summaryA := summaries[0]
	model, ph, err := s.StatePool.GetModel(summaryA.UUID)
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	conf, err := model.Config()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaryA.ProviderType, tc.Equals, conf.Type())
}

func (s *ModelSummariesSuite) TestContainsModelStatus(c *tc.C) {
	modelNameToUUID := s.Setup4Models(c)
	expectedStatus := map[string]status.StatusInfo{
		"shared": {
			Status:  status.Available,
			Message: "human message",
		},
		"user1model": {
			Status:  status.Busy,
			Message: "human message",
		},
	}
	shared, ph, err := s.StatePool.GetModel(modelNameToUUID["shared"])
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	err = shared.SetStatus(expectedStatus["shared"])
	c.Assert(err, tc.ErrorIsNil)
	user1, ph, err := s.StatePool.GetModel(modelNameToUUID["user1model"])
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	err = user1.SetStatus(expectedStatus["user1model"])
	c.Assert(err, tc.ErrorIsNil)
	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	statuses := make(map[string]status.StatusInfo)
	for _, summary := range summaries {
		// We nil the time, because we don't want to compare it, we nil the Data map to avoid comparing an
		// empty map to a nil map
		st := summary.Status
		st.Since = nil
		st.Data = nil
		statuses[summary.Name] = st
	}
	c.Check(statuses, tc.DeepEquals, expectedStatus)
}

func (s *ModelSummariesSuite) TestContainsModelStatusSuspended(c *tc.C) {
	modelNameToUUID := s.Setup4Models(c)
	expectedStatus := map[string]status.StatusInfo{
		"shared": {
			Status:  status.Suspended,
			Message: "suspended since cloud credential is not valid",
			Data:    map[string]interface{}{"reason": "test"},
		},
		"user1model": {
			Status:  status.Busy,
			Message: "human message",
			Data:    map[string]interface{}{},
		},
	}
	shared, err := s.StatePool.Get(modelNameToUUID["shared"])
	defer shared.Release()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(shared.InvalidateModelCredential("test"), tc.ErrorIsNil)

	user1, ph, err := s.StatePool.GetModel(modelNameToUUID["user1model"])
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	err = user1.SetStatus(expectedStatus["user1model"])
	c.Assert(err, tc.ErrorIsNil)
	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	statuses := make(map[string]status.StatusInfo)
	for _, summary := range summaries {
		// We nil the time, because we don't want to compare it, we nil the Data map to avoid comparing an
		// empty map to a nil map
		st := summary.Status
		st.Since = nil
		statuses[summary.Name] = st
	}
	c.Check(statuses, tc.DeepEquals, expectedStatus)
}

func (s *ModelSummariesSuite) TestContainsAccessInformation(c *tc.C) {
	modelNameToUUID := s.Setup4Models(c)
	shared, ph, err := s.StatePool.GetModel(modelNameToUUID["shared"])
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	err = shared.UpdateLastModelConnection(names.NewUserTag("auser"))
	s.Clock.Advance(time.Hour)
	c.Assert(err, tc.ErrorIsNil)
	timeShared := s.Clock.Now().Round(time.Second).UTC()
	err = shared.UpdateLastModelConnection(names.NewUserTag("user1write"))
	c.Assert(err, tc.ErrorIsNil)
	s.Clock.Advance(time.Hour) // give a different time for user2 accessing the shared model
	err = shared.UpdateLastModelConnection(names.NewUserTag("user2read"))
	c.Assert(err, tc.ErrorIsNil)
	user1, ph, err := s.StatePool.GetModel(modelNameToUUID["user1model"])
	defer ph.Release()
	c.Assert(err, tc.ErrorIsNil)
	s.Clock.Advance(time.Hour)
	timeUser1 := s.Clock.Now().Round(time.Second).UTC()
	err = user1.UpdateLastModelConnection(names.NewUserTag("user1write"))
	c.Assert(err, tc.ErrorIsNil)

	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	times := make(map[string]time.Time)
	access := make(map[string]permission.Access)
	for _, summary := range summaries {
		c.Assert(summary.UserLastConnection, tc.NotNil, tc.Commentf("nil time for %v", summary.Name))
		times[summary.Name] = summary.UserLastConnection.UTC()
		access[summary.Name] = summary.Access
	}
	c.Check(times, tc.DeepEquals, map[string]time.Time{
		"shared":     timeShared,
		"user1model": timeUser1,
	})
	c.Check(access, tc.DeepEquals, map[string]permission.Access{
		"shared":     permission.WriteAccess,
		"user1model": permission.AdminAccess,
	})
}

func (s *ModelSummariesSuite) TestContainsMachineInformation(c *tc.C) {
	modelNameToUUID := s.Setup4Models(c)
	shared, err := s.StatePool.Get(modelNameToUUID["shared"])
	defer shared.Release()
	c.Assert(err, tc.ErrorIsNil)
	onecore := uint64(1)
	twocores := uint64(2)
	threecores := uint64(3)
	m0, err := shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m0.Life(), tc.Equals, state.Alive)
	err = m0.SetInstanceInfo("i-12345", "", "nonce", &instance.HardwareCharacteristics{
		CpuCores: &onecore,
	}, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	m1, err := shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = m1.SetInstanceInfo("i-45678", "", "nonce", &instance.HardwareCharacteristics{
		CpuCores: &twocores,
	}, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	m2, err := shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = m2.SetInstanceInfo("i-78901", "", "nonce", &instance.HardwareCharacteristics{
		CpuCores: &threecores,
	}, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	// No instance
	_, err = shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	// Dying instance, should not count to Cores or Machine count
	mDying, err := shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = mDying.SetInstanceInfo("i-78901", "", "nonce", &instance.HardwareCharacteristics{
		CpuCores: &threecores,
	}, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = mDying.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// Instance data, but no core count
	m4, err := shared.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	arch := arch.DefaultArchitecture
	err = m4.SetInstanceInfo("i-78901", "", "nonce", &instance.HardwareCharacteristics{
		Arch: &arch,
	}, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag("user1write"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(summaries, tc.HasLen, 2)
	summaryMap := make(map[string]*state.ModelSummary)
	for i := range summaries {
		summaryMap[summaries[i].Name] = &summaries[i]
	}
	sharedSummary := summaryMap["shared"]
	c.Assert(sharedSummary, tc.NotNil)
	c.Check(sharedSummary.MachineCount, tc.Equals, int64(5))
	c.Check(sharedSummary.CoreCount, tc.Equals, int64(1+2+3))
	userSummary := summaryMap["user1model"]
	c.Assert(userSummary, tc.NotNil)
	c.Check(userSummary.MachineCount, tc.Equals, int64(0))
	c.Check(userSummary.CoreCount, tc.Equals, int64(0))
}

func (s *ModelSummariesSuite) TestContainsMigrationInformation(c *tc.C) {
	//modelNameToUUID := s.Setup4Models(c)
	// TODO: Figure out how to create a multiple-attempt migration information, and assert that we expose the right info
}

func (s *ModelSummariesSuite) namedSummariesForUser(c *tc.C, user string) map[string]*state.ModelSummary {
	summaries, err := s.State.ModelSummariesForUser(names.NewUserTag(user), false)
	c.Assert(err, tc.ErrorIsNil)
	summaryMap := make(map[string]*state.ModelSummary, len(summaries))
	for i := range summaries {
		summaryMap[summaries[i].Name] = &summaries[i]
	}
	return summaryMap
}

func (s *ModelSummariesSuite) TestModelsWithNoSettings(c *tc.C) {
	modelNameToUUID := s.Setup4Models(c)
	m2uuid := modelNameToUUID["user2model"]
	// Mark the model as dying, and move to start tearing it down
	model, ph, err := s.StatePool.GetModel(m2uuid)
	c.Assert(err, tc.ErrorIsNil)
	defer ph.Release()
	err = model.SetStatus(status.StatusInfo{
		Status:  status.Available,
		Message: "running",
	})
	c.Assert(err, tc.ErrorIsNil)

	summaryMap := s.namedSummariesForUser(c, "user2read")
	// Even though user2model is dying/dead, it should still be in the output.
	c.Check(summaryMap, tc.HasLen, 2)
	userSummary := summaryMap["user2model"]
	c.Assert(userSummary, tc.NotNil)
	c.Check(userSummary.Status.Message, tc.Equals, "running")

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetStatus(status.StatusInfo{
		Status:  status.Destroying,
		Message: "stopping",
	})
	c.Assert(err, tc.ErrorIsNil)

	summaryMap = s.namedSummariesForUser(c, "user2read")
	// Even though user2model is dying/dead, it should still be in the output.
	c.Check(summaryMap, tc.HasLen, 2)
	userSummary = summaryMap["user2model"]
	c.Assert(userSummary, tc.NotNil)
	c.Check(userSummary.Status.Message, tc.Equals, "stopping")

	// Now we start tearing down some of the collections for this model, and see that it still shows up.
	settings := s.Session.DB("juju").C("settings")
	// The settings document for this model
	err = settings.Remove(bson.M{"_id": m2uuid + ":e"})
	c.Assert(err, tc.ErrorIsNil)
	summaryMap = s.namedSummariesForUser(c, "user2read")
	c.Assert(err, tc.ErrorIsNil)
	// Even though user2model is dying/dead, it should still be in the output.
	c.Check(summaryMap, tc.HasLen, 2)
	userSummary = summaryMap["user2model"]
	c.Assert(userSummary, tc.NotNil)
	c.Check(userSummary.Status.Message, tc.Equals, "stopping")
}

func (s *ModelSummariesSuite) TestCAASModel(c *tc.C) {
	s.Setup4Models(c)

	summaryMap := s.namedSummariesForUser(c, "user2read")
	c.Check(summaryMap, tc.HasLen, 2)
	userSummary := summaryMap["user2model"]
	c.Assert(userSummary, tc.NotNil)
	c.Assert(userSummary.MachineCount, tc.Equals, int64(0))
	c.Assert(userSummary.CoreCount, tc.Equals, int64(0))
	c.Assert(userSummary.UnitCount, tc.Equals, int64(1))
}
