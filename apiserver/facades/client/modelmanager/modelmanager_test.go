// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	stdcontext "context"
	"regexp"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	// Register the providers for the field check test
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	_ "github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/dummy"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	jujuversion "github.com/juju/juju/version"
)

func createArgs(owner names.UserTag) params.ModelCreateArgs {
	return params.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: owner.String(),
		Config: map[string]interface{}{
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"controller": false,
		},
	}
}

type modelManagerSuite struct {
	testhelpers.IsolationSuite
	st         *mockState
	ctlrSt     *mockState
	caasSt     *mockState
	caasBroker *mockCaasBroker
	authoriser apiservertesting.FakeAuthorizer
	api        *modelmanager.ModelManagerAPI
	caasApi    *modelmanager.ModelManagerAPI

	callContext context.ProviderCallContext
}

func TestModelManagerSuite(t *tctesting.T) {
	tc.Run(t, &modelManagerSuite{})
}

func (s *modelManagerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	attrs := dummy.SampleConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)

	dummyCloud := cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		Regions: []cloud.Region{
			{Name: "some-region"},
			{Name: "qux"},
		},
	}

	mockK8sCloud := cloud.Cloud{
		Name:      "k8s-cloud",
		Type:      "kubernetes",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}

	controllerModel := &mockModel{
		owner: names.NewUserTag("admin"),
		life:  state.Alive,
		cfg:   cfg,
		status: status.StatusInfo{
			Status: status.Available,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName: "add-model",
			access:   permission.AdminAccess,
		}, {

			userName: "otheruser",
			access:   permission.WriteAccess,
		}},
	}

	s.st = &mockState{
		block: -1,
		cloud: dummyCloud,
		clouds: map[names.CloudTag]cloud.Cloud{
			names.NewCloudTag("some-cloud"): dummyCloud,
		},
		controllerModel: controllerModel,
		model: &mockModel{
			owner: names.NewUserTag("admin"),
			life:  state.Alive,
			tag:   coretesting.ModelTag,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.Available,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   permission.AdminAccess,
			}, {
				userName: "add-model",
				access:   permission.AdminAccess,
			}, {

				userName: "otheruser",
				access:   permission.WriteAccess,
			}},
		},
		cred:        statetesting.NewEmptyCredential(),
		modelConfig: coretesting.ModelConfig(c),
	}
	s.ctlrSt = &mockState{
		model:           s.st.model,
		controllerModel: controllerModel,
		cred:            statetesting.NewEmptyCredential(),
		cloud:           dummyCloud,
		clouds: map[names.CloudTag]cloud.Cloud{
			names.NewCloudTag("some-cloud"): dummyCloud,
		},
		cloudUsers: map[string]permission.Access{},
		cfgDefaults: config.ModelDefaultAttributes{
			"attr": config.AttributeDefaultValues{
				Default:    "",
				Controller: "val",
				Regions: []config.RegionDefaultValue{{
					Name:  "dummy",
					Value: "val++"}}},
			"attr2": config.AttributeDefaultValues{
				Controller: "val3",
				Default:    "val2",
				Regions: []config.RegionDefaultValue{{
					Name:  "left",
					Value: "spam"}}},
		},
	}

	caasCred := state.Credential{}
	caasCred.AuthType = string(cloud.UserPassAuthType)
	s.caasSt = &mockState{
		cloud: mockK8sCloud,
		clouds: map[names.CloudTag]cloud.Cloud{
			names.NewCloudTag("k8s-cloud"): mockK8sCloud,
		},
		controllerModel: controllerModel,
		model: &mockModel{
			owner: names.NewUserTag("admin"),
			life:  state.Alive,
			tag:   coretesting.ModelTag,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.Available,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   permission.AdminAccess,
			}, {
				userName: "add-model",
				access:   permission.AdminAccess,
			}},
		},
		cred:        caasCred,
		modelConfig: coretesting.ModelConfig(c),
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}

	s.callContext = context.NewEmptyCloudCallContext()

	newBroker := func(_ stdcontext.Context, args environs.OpenParams) (caas.Broker, error) {
		s.caasBroker = &mockCaasBroker{namespace: args.Config.Name()}
		return s.caasBroker, nil
	}

	api, err := modelmanager.NewModelManagerAPI(
		s.st, s.ctlrSt, nil, newBroker, common.NewBlockChecker(s.st),
		s.authoriser, s.st.model, s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	caasApi, err := modelmanager.NewModelManagerAPI(
		s.caasSt, s.ctlrSt, nil, newBroker, common.NewBlockChecker(s.caasSt),
		s.authoriser, s.st.model, s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.caasApi = caasApi

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})
	modelmanager.MockSupportedFeatures(fs)
}

func (s *modelManagerSuite) TearDownTest(c *tc.C) {
	modelmanager.ResetSupportedFeaturesGetter()
}

func (s *modelManagerSuite) setAPIUser(c *tc.C, user names.UserTag, authorizerOptions ...apiservertesting.FakeAuthorizerOption) {
	s.authoriser.Tag = user
	for _, option := range authorizerOptions {
		option(&s.authoriser)
	}
	newBroker := func(_ stdcontext.Context, args environs.OpenParams) (caas.Broker, error) {
		return s.caasBroker, nil
	}
	mm, err := modelmanager.NewModelManagerAPI(
		s.st, s.ctlrSt, nil, newBroker, common.NewBlockChecker(s.st),
		s.authoriser, s.st.model, s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = mm
}

func (s *modelManagerSuite) getModelArgs(c *tc.C) state.ModelArgs {
	return getModelArgsFor(c, s.st)
}

func getModelArgsFor(c *tc.C, mockState *mockState) state.ModelArgs {
	for _, v := range mockState.Calls() {
		if v.Args == nil {
			continue
		}
		if newModelArgs, ok := v.Args[0].(state.ModelArgs); ok {
			return newModelArgs
		}
	}
	c.Fatal("failed to find state.ModelArgs")
	panic("unreachable")
}

func (s *modelManagerSuite) TestCreateModelArgs(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-some-cloud_admin_some-credential",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c,
		"ControllerTag",
		"ControllerTag",
		"Cloud",
		"CloudCredential",
		"ComposeNewModelConfig",
		"ControllerConfig",
		"NewModel",
		"SaveProviderSubnets",
		"Close",
		"GetBackend",
		"Model",
		"IsController",
		"LatestMigration",
		"AllMachines",
		"ControllerNodes",
		"HAPrimaryMachine",
	)

	// Check that Model.LastModelConnection is called three times
	// without making the test depend on other calls to Model
	n := 0
	for _, call := range s.st.model.Calls() {
		if call.FuncName == "LastModelConnection" {
			n = n + 1
		}
	}
	c.Assert(n, tc.Equals, 3)

	// We cannot predict the UUID, because it's generated,
	// so we just extract it and ensure that it's not the
	// same as the controller UUID.
	newModelArgs := s.getModelArgs(c)
	uuid := newModelArgs.Config.UUID()
	c.Assert(uuid, tc.Not(tc.Equals), s.st.controllerModel.cfg.UUID())

	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":          "foo",
		"type":          "dummy",
		"uuid":          uuid,
		"agent-version": jujuversion.Current.String(),
		"bar":           "baz",
		"controller":    false,
		"broken":        "",
		"secret":        "pork",
		"something":     "value",
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(newModelArgs.StorageProviderRegistry, tc.NotNil)
	newModelArgs.StorageProviderRegistry = nil

	c.Assert(newModelArgs, tc.DeepEquals, state.ModelArgs{
		Type:        state.ModelTypeIAAS,
		Owner:       names.NewUserTag("admin"),
		CloudName:   "some-cloud",
		CloudRegion: "qux",
		CloudCredential: names.NewCloudCredentialTag(
			"some-cloud/admin/some-credential",
		),
		Config: cfg,
	})
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloud(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudTag:           "cloud-some-cloud",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-some-cloud_admin_some-credential",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, tc.Equals, "some-cloud")
}

func (s *modelManagerSuite) TestModelInfoWithReadAccess(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
	}
	modelInfoAdmin, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)

	_true := true
	expectedModelInfo := modelInfoAdmin
	expectedModelInfo.Users = []params.ModelUserInfo{}
	expectedModelInfo.CloudCredentialValidity = &_true
	expectedModelInfo.Machines = []params.ModelMachineInfo{}
	expectedModelInfo.SecretBackends = []params.SecretBackendResult{}

	alice := names.NewUserTag("alice")
	s.setAPIUser(c, alice, apiservertesting.SetTagWithReadAccess(alice))
	modelInfoReader, err := s.api.ModelInfo(params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewModelTag(modelInfoAdmin.UUID).String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelInfoReader.Results, tc.HasLen, 1)
	c.Assert(modelInfoReader.Results[0].Error, tc.IsNil)
	c.Assert(modelInfoReader.Results[0].Result, tc.DeepEquals, &expectedModelInfo)
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloudNotFound(c *tc.C) {
	s.st.SetErrors(errors.NotFoundf("cloud"))
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		CloudTag: "cloud-some-unknown-cloud",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorMatches, `cloud "some-unknown-cloud" not found, expected one of \["some-cloud"\]`)
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudRegion, tc.Equals, "some-region")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *tc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdminNoDomain(c *tc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin")
}

func (s *modelManagerSuite) testCreateModelDefaultCredentialAdmin(c *tc.C, ownerTag string) {
	s.st.cloud.AuthTypes = []cloud.AuthType{"userpass"}
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: ownerTag,
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, tc.Equals, names.NewCloudCredentialTag(
		"some-cloud/bob/some-credential",
	))
}

func (s *modelManagerSuite) TestCreateModelEmptyCredentialNonAdmin(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, tc.Equals, names.CloudCredentialTag{})
}

func (s *modelManagerSuite) TestCreateModelNoDefaultCredentialNonAdmin(c *tc.C) {
	s.st.cloud.AuthTypes = nil
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorMatches, "no credential specified")
}

func (s *modelManagerSuite) TestCreateModelUnknownCredential(c *tc.C) {
	s.st.SetErrors(nil, errors.NotFoundf("credential"))
	args := params.ModelCreateArgs{
		Name:               "foo",
		OwnerTag:           "user-admin",
		CloudCredentialTag: "cloudcred-some-cloud_admin_bar",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, tc.ErrorMatches, `getting credential: credential not found`)
}

func (s *modelManagerSuite) TestCreateCAASModelArgs(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:               "foo",
		OwnerTag:           "user-admin",
		Config:             map[string]interface{}{},
		CloudTag:           "cloud-k8s-cloud",
		CloudCredentialTag: "cloudcred-k8s-cloud_admin_some-credential",
	}
	_, err := s.caasApi.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)
	s.caasSt.CheckCallNames(c,
		"ControllerTag",
		"ControllerTag",
		"Cloud",
		"CloudCredential",
		"ComposeNewModelConfig",
		"ControllerConfig",
		"NewModel",
		"Close",
		"GetBackend",
		"Model",
		"IsController",
		"LatestMigration",
		"AllMachines",
		"ControllerNodes",
		"HAPrimaryMachine",
	)
	s.caasBroker.CheckCallNames(c, "Create")

	// Check that Model.LastModelConnection is called just twice
	// without making the test depend on other calls to Model
	n := 0
	for _, call := range s.caasSt.model.Calls() {
		if call.FuncName == "LastModelConnection" {
			n = n + 1
		}
	}
	c.Assert(n, tc.Equals, 2)

	// We cannot predict the UUID, because it's generated,
	// so we just extract it and ensure that it's not the
	// same as the controller UUID.
	newModelArgs := getModelArgsFor(c, s.caasSt)
	uuid := newModelArgs.Config.UUID()
	c.Assert(uuid, tc.Not(tc.Equals), s.caasSt.controllerModel.cfg.UUID())

	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":                              "foo",
		"type":                              "kubernetes",
		"uuid":                              uuid,
		"agent-version":                     jujuversion.Current.String(),
		"storage-default-block-source":      "kubernetes",
		"storage-default-filesystem-source": "kubernetes",
		"something":                         "value",
		"operator-storage":                  "",
		"workload-storage":                  "",
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(newModelArgs.StorageProviderRegistry, tc.NotNil)
	newModelArgs.StorageProviderRegistry = nil

	c.Assert(newModelArgs, tc.DeepEquals, state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		Owner:     names.NewUserTag("admin"),
		CloudName: "k8s-cloud",
		CloudCredential: names.NewCloudCredentialTag(
			"k8s-cloud/admin/some-credential",
		),
		Config: cfg,
	})
}

func (s *modelManagerSuite) TestCreateCAASModelNamespaceClash(c *tc.C) {
	args := params.ModelCreateArgs{
		Name:               "existing-ns",
		OwnerTag:           "user-admin",
		Config:             map[string]interface{}{},
		CloudTag:           "cloud-k8s-cloud",
		CloudCredentialTag: "cloudcred-k8s-cloud_admin_some-credential",
	}
	_, err := s.caasApi.CreateModel(args)
	s.caasBroker.CheckCallNames(c, "Create")
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *modelManagerSuite) TestModelDefaults(c *tc.C) {
	results, err := s.api.ModelDefaultsForClouds(params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	expectedValues := map[string]params.ModelDefaults{
		"attr": {
			Controller: "val",
			Default:    "",
			Regions: []params.RegionDefaults{{
				RegionName: "dummy",
				Value:      "val++"}}},
		"attr2": {
			Controller: "val3",
			Default:    "val2",
			Regions: []params.RegionDefaults{{
				RegionName: "left",
				Value:      "spam"}}},
	}
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Config, tc.DeepEquals, expectedValues)
}

func (s *modelManagerSuite) TestSetModelDefaults(c *tc.C) {
	params := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"attr3": "val3",
				"attr4": "val4"},
		}}}
	result, err := s.api.SetModelDefaults(params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
	c.Assert(s.ctlrSt.cfgDefaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {
			Controller: "val",
			Default:    "",
			Regions: []config.RegionDefaultValue{{
				Name:  "dummy",
				Value: "val++"}}},
		"attr2": {
			Controller: "val3",
			Default:    "val2",
			Regions: []config.RegionDefaultValue{{
				Name:  "left",
				Value: "spam"}}},
		"attr3": {Controller: "val3"},
		"attr4": {Controller: "val4"},
	})
}

func (s *modelManagerSuite) blockAllChanges(c *tc.C, msg string) {
	s.st.blockMsg = msg
	s.st.block = state.ChangeBlock
}

func (s *modelManagerSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelManagerSuite) TestBlockChangesSetModelDefaults(c *tc.C) {
	s.blockAllChanges(c, "TestBlockChangesSetModelDefaults")
	_, err := s.api.SetModelDefaults(params.SetModelDefaults{})
	s.assertBlocked(c, err, "TestBlockChangesSetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelDefaults(c *tc.C) {
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr"},
		}}}
	result, err := s.api.UnsetModelDefaults(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
	want := config.ModelDefaultAttributes{
		"attr": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{
				{Name: "dummy", Value: "val++"},
			},
		},
		"attr2": config.AttributeDefaultValues{
			Default:    "val2",
			Controller: "val3",
			Regions: []config.RegionDefaultValue{
				{Name: "left", Value: "spam"},
			},
		},
	}
	c.Assert(s.ctlrSt.cfgDefaults, tc.DeepEquals, want)
}

func (s *modelManagerSuite) TestBlockUnsetModelDefaults(c *tc.C) {
	s.blockAllChanges(c, "TestBlockUnsetModelDefaults")
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"abc"},
		}}}
	_, err := s.api.UnsetModelDefaults(args)
	s.assertBlocked(c, err, "TestBlockUnsetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelDefaultsMissing(c *tc.C) {
	// It's okay to unset a non-existent attribute.
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"not there"},
		}}}
	result, err := s.api.UnsetModelDefaults(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestModelDefaultsAsNormalUser(c *tc.C) {
	charlie := names.NewUserTag("charlie")
	s.setAPIUser(c, charlie)
	got, err := s.api.ModelDefaultsForClouds(params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ModelDefaultsResults{})
}

func (s *modelManagerSuite) TestSetModelDefaultsAsNormalUser(c *tc.C) {
	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.SetModelDefaults(params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"ftp-proxy": "http://charlie",
			}}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access"}}}})

	// Make sure it didn't change.
	s.setAPIUser(c, names.NewUserTag("admin"))
	results, err := s.api.ModelDefaultsForClouds(params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Config["ftp-proxy"].Controller, tc.IsNil)
}

func (s *modelManagerSuite) TestUnsetModelDefaultsAsNormalUser(c *tc.C) {
	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.UnsetModelDefaults(params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr2"}}}})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Make sure it didn't change.
	s.setAPIUser(c, names.NewUserTag("admin"))
	results, err := s.api.ModelDefaultsForClouds(params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Config["attr2"].Controller.(string), tc.Equals, "val3")
}

func (s *modelManagerSuite) TestDumpModel(c *tc.C) {
	results := s.api.DumpModels(params.DumpModelRequest{
		Entities: []params.Entity{{
			Tag: "bad-tag",
		}, {
			Tag: "application-foo",
		}, {
			Tag: s.st.ModelTag().String(),
		}}})

	c.Assert(results.Results, tc.HasLen, 3)
	bad, notApp, good := results.Results[0], results.Results[1], results.Results[2]
	c.Check(bad.Result, tc.Equals, "")
	c.Check(bad.Error.Message, tc.Equals, `"bad-tag" is not a valid tag`)

	c.Check(notApp.Result, tc.Equals, "")
	c.Check(notApp.Error.Message, tc.Equals, `"application-foo" is not a valid model tag`)

	c.Check(good.Error, tc.IsNil)
	c.Check(good.Result, tc.DeepEquals, "model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n")
}

func (s *modelManagerSuite) TestDumpModelMissingModel(c *tc.C) {
	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModels(models)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"ControllerTag", nil},
		{"GetBackend", []interface{}{tag.Id()}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Result, tc.Equals, "")
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.Error.Code, tc.Equals, `not found`)
	c.Check(result.Error.Message, tc.Equals, `id not found`)
}

func (s *modelManagerSuite) TestDumpModelUsers(c *tc.C) {
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModels(models)
		c.Assert(results.Results, tc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, tc.Equals, "")
		c.Assert(result.Error, tc.NotNil)
		c.Check(result.Error.Message, tc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestDumpModelsDB(c *tc.C) {
	results := s.api.DumpModelsDB(params.Entities{[]params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: "application-foo",
	}, {
		Tag: s.st.ModelTag().String(),
	}}})

	c.Assert(results.Results, tc.HasLen, 3)
	bad, notApp, good := results.Results[0], results.Results[1], results.Results[2]
	c.Check(bad.Result, tc.IsNil)
	c.Check(bad.Error.Message, tc.Equals, `"bad-tag" is not a valid tag`)

	c.Check(notApp.Result, tc.IsNil)
	c.Check(notApp.Error.Message, tc.Equals, `"application-foo" is not a valid model tag`)

	c.Check(good.Error, tc.IsNil)
	c.Check(good.Result, tc.DeepEquals, map[string]interface{}{
		"models": "lots of data",
	})
}

func (s *modelManagerSuite) TestDumpModelsDBMissingModel(c *tc.C) {
	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.Entities{[]params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModelsDB(models)

	s.st.CheckCalls(c, []testhelpers.StubCall{
		{"ControllerTag", nil},
		{"ModelTag", nil},
		{"GetBackend", []interface{}{tag.Id()}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Result, tc.IsNil)
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.Error.Code, tc.Equals, `not found`)
	c.Check(result.Error.Message, tc.Equals, `id not found`)
}

func (s *modelManagerSuite) TestDumpModelsDBUsers(c *tc.C) {
	models := params.Entities{[]params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModelsDB(models)
		c.Assert(results.Results, tc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, tc.IsNil)
		c.Assert(result.Error, tc.NotNil)
		c.Check(result.Error.Message, tc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestAddModelCanCreateModel(c *tc.C) {
	addModelUser := names.NewUserTag("add-model")
	s.ctlrSt.cloudUsers[addModelUser.Id()] = permission.AddModelAccess
	s.setAPIUser(c, addModelUser, apiservertesting.SetTagWithAdminAccess(addModelUser))
	_, err := s.api.CreateModel(createArgs(addModelUser))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestAddModelCantCreateModelForSomeoneElse(c *tc.C) {
	addModelUser := names.NewUserTag("add-model")
	s.ctlrSt.cloudUsers[addModelUser.Id()] = permission.AddModelAccess
	s.setAPIUser(c, addModelUser)
	nonAdminUser := names.NewUserTag("non-admin")
	_, err := s.api.CreateModel(createArgs(nonAdminUser))
	c.Assert(err, tc.ErrorMatches, "\"add-model\" permission does not permit creation of models for different owners: permission denied")
}

// modelManagerStateSuite contains end-to-end tests.
// Prefer adding tests to modelManagerSuite above.
type modelManagerStateSuite struct {
	jujutesting.JujuConnSuite
	modelmanager *modelmanager.ModelManagerAPI
	authoriser   apiservertesting.FakeAuthorizer

	callContext context.ProviderCallContext
}

func TestModelManagerStateSuite(t *tctesting.T) {
	tc.Run(t, &modelManagerStateSuite{})
}

func (s *modelManagerStateSuite) SetUpSuite(c *tc.C) {
	coretesting.SkipUnlessControllerOS(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *modelManagerStateSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.callContext = context.NewEmptyCloudCallContext()
	loggo.GetLogger("juju.apiserver.modelmanager").SetLogLevel(loggo.TRACE)
}

func (s *modelManagerStateSuite) setAPIUser(c *tc.C, user names.UserTag, authorizerOptions ...apiservertesting.FakeAuthorizerOption) {
	s.authoriser.Tag = user
	for _, option := range authorizerOptions {
		option(&s.authoriser)
	}
	st := common.NewModelManagerBackend(s.Model, s.StatePool)
	ctlrSt := common.NewModelManagerBackend(s.Model, s.StatePool)
	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), ctlrSt)
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	configGetter := stateenvirons.EnvironConfigGetter{Model: s.Model}
	newEnviron := common.EnvironFuncForModel(model, configGetter)
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
	modelmanager, err := modelmanager.NewModelManagerAPI(
		st, ctlrSt,
		toolsFinder,
		nil,
		common.NewBlockChecker(st),
		s.authoriser,
		s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.modelmanager = modelmanager
}

func (s *modelManagerStateSuite) TestNewAPIAcceptsClient(c *tc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	st := common.NewModelManagerBackend(s.Model, s.StatePool)
	endPoint, err := modelmanager.NewModelManagerAPI(
		st,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(st), anAuthoriser,
		s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endPoint, tc.NotNil)
}

func (s *modelManagerStateSuite) TestNewAPIRefusesNonClient(c *tc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	st := common.NewModelManagerBackend(s.Model, s.StatePool)
	endPoint, err := modelmanager.NewModelManagerAPI(
		st,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(st), anAuthoriser, s.Model,
		s.callContext,
	)
	c.Assert(endPoint, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) createArgsForVersion(c *tc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := createArgs(owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerStateSuite) TestUserCanCreateModel(c *tc.C) {
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	model, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.OwnerTag, tc.Equals, owner.String())
	c.Assert(model.Name, tc.Equals, "test-model")
	c.Assert(model.Type, tc.Equals, "iaas")
}

func (s *modelManagerStateSuite) TestAdminCanCreateModelForSomeoneElse(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	owner := names.NewUserTag("external@remote")

	model, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.OwnerTag, tc.Equals, owner.String())
	c.Assert(model.Name, tc.Equals, "test-model")
	c.Assert(model.Type, tc.Equals, "iaas")
	// Make sure that the environment created does actually have the correct
	// owner, and that owner is actually allowed to use the environment.
	newState, err := s.StatePool.Get(model.UUID)
	c.Assert(err, tc.ErrorIsNil)
	defer newState.Release()

	newModel, err := newState.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newModel.Owner(), tc.Equals, owner)
	_, err = newState.UserAccess(owner, newModel.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *tc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSelf(c *tc.C) {
	owner := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, owner)
	_, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestCreateModelValidatesConfig(c *tc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := createArgs(admin)
	args.Config["controller"] = "maybe"
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, tc.ErrorMatches,
		"failed to create config: provider config preparation failed: controller: expected bool, got string\\(\"maybe\"\\)",
	)
}

func (s *modelManagerStateSuite) TestCreateModelBadConfig(c *tc.C) {
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	for i, test := range []struct {
		key      string
		value    interface{}
		errMatch string
	}{
		{
			key:      "uuid",
			value:    "anything",
			errMatch: `failed to create config: uuid is generated, you cannot specify one`,
		},
	} {
		c.Logf("%d: %s", i, test.key)
		args := createArgs(owner)
		args.Config[test.key] = test.value
		_, err := s.modelmanager.CreateModel(args)
		c.Assert(err, tc.ErrorMatches, test.errMatch)

	}
}

func (s *modelManagerStateSuite) TestCreateModelSameAgentVersion(c *tc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestCreateModelBadAgentVersion(c *tc.C) {
	err := s.BackingState.SetModelAgentVersion(coretesting.FakeVersionNumber, nil, false)
	c.Assert(err, tc.ErrorIsNil)

	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)

	bigger := coretesting.FakeVersionNumber
	bigger.Minor += 1

	smaller := coretesting.FakeVersionNumber
	smaller.Minor -= 1

	for i, test := range []struct {
		value    interface{}
		errMatch string
	}{
		{
			value:    42,
			errMatch: `failed to create config: agent-version must be a string but has type 'int'`,
		}, {
			value:    "not a number",
			errMatch: `failed to create config: invalid version \"not a number\"`,
		}, {
			value:    bigger.String(),
			errMatch: "failed to create config: agent-version .* cannot be greater than the controller .*",
		}, {
			value:    smaller.String(),
			errMatch: "failed to create config: no agent binaries found for version .*",
		},
	} {
		c.Logf("test %d", i)
		args := s.createArgsForVersion(c, admin, test.value)
		_, err := s.modelmanager.CreateModel(args)
		c.Check(err, tc.ErrorMatches, test.errMatch)
	}
}

func (s *modelManagerStateSuite) checkModelMatches(c *tc.C, model params.Model, expected *state.Model) {
	c.Check(model.Name, tc.Equals, expected.Name())
	c.Check(model.UUID, tc.Equals, expected.UUID())
	c.Check(model.OwnerTag, tc.Equals, expected.Owner().String())
}

func (s *modelManagerStateSuite) TestListModelsAdminSelf(c *tc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{Tag: user.String()})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserModels, tc.HasLen, 1)
	expected, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.checkModelMatches(c, result.UserModels[0].Model, expected)
}

func (s *modelManagerStateSuite) TestListModelsAdminListsOther(c *tc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("admin")
	result, err := s.modelmanager.ListModels(params.Entity{Tag: other.String()})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserModels, tc.HasLen, 1)
}

func (s *modelManagerStateSuite) TestListModelsDenied(c *tc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.modelmanager.ListModels(params.Entity{Tag: other.String()})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestAdminModelManager(c *tc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsTrue)
}

func (s *modelManagerStateSuite) TestNonAdminModelManager(c *tc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsFalse)
}

func (s *modelManagerStateSuite) TestDestroyOwnModel(c *tc.C) {
	// TODO(perrito666) this test is not valid until we have
	// proper controller permission since the only users that
	// can create models are controller admins.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorIsNil)

	st, err := s.StatePool.Get(m.UUID)
	c.Assert(err, tc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	backend := common.NewModelManagerBackend(model, s.StatePool)
	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		backend,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(backend), s.authoriser,
		s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)

	force := true
	timeout := time.Minute
	results, err := s.modelmanager.DestroyModels(params.DestroyModelsParams{
		Models: []params.DestroyModelParams{{
			ModelTag: "model-" + m.UUID,
			Force:    &force,
			Timeout:  &timeout,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	model, err = st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Not(tc.Equals), state.Alive)
	gotTimeout := model.DestroyTimeout()
	c.Assert(gotTimeout, tc.NotNil)
	c.Assert(*gotTimeout, tc.Equals, timeout)
	gotForce := model.ForceDestroyed()
	c.Assert(gotForce, tc.IsTrue)
}

func (s *modelManagerStateSuite) TestAdminDestroysOtherModel(c *tc.C) {
	// TODO(perrito666) Both users are admins in this case, this tesst is of dubious
	// usefulness until proper controller permissions are in place.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorIsNil)

	st, err := s.StatePool.Get(m.UUID)
	c.Assert(err, tc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	s.authoriser.Tag = s.AdminUserTag(c)
	backend := common.NewModelManagerBackend(model, s.StatePool)
	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		backend,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(backend), s.authoriser,
		s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.modelmanager.DestroyModels(params.DestroyModelsParams{
		Models: []params.DestroyModelParams{{
			ModelTag: "model-" + m.UUID,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)

	s.authoriser.Tag = owner
	model, err = st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Not(tc.Equals), state.Alive)
}

func (s *modelManagerStateSuite) TestDestroyModelErrors(c *tc.C) {
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(createArgs(owner))
	c.Assert(err, tc.ErrorIsNil)

	st, err := s.StatePool.Get(m.UUID)
	c.Assert(err, tc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	backend := common.NewModelManagerBackend(model, s.StatePool)
	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		backend,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(backend), s.authoriser, s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)

	user := names.NewUserTag("other@remote")
	s.setAPIUser(c, user)

	results, err := s.modelmanager.DestroyModels(params.DestroyModelsParams{
		Models: []params.DestroyModelParams{
			{ModelTag: "model-" + m.UUID},
			{ModelTag: "model-9f484882-2f18-4fd2-967d-db9663db7bea"},
			{ModelTag: "machine-42"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{{
		// we don't have admin access to the model
		&params.Error{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		},
	}, {
		&params.Error{
			Message: `model "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
			Code:    params.CodeNotFound,
		},
	}, {
		&params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})

	s.setAPIUser(c, owner)
	model, err = st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Alive)
}

func (s *modelManagerStateSuite) modifyAccess(c *tc.C, user names.UserTag, action params.ModelAction, access params.UserAccessPermission, model names.ModelTag) error {
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.String(),
			Action:   action,
			Access:   access,
			ModelTag: model.String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	if err != nil {
		return err
	}
	return result.OneError()
}

func (s *modelManagerStateSuite) grant(c *tc.C, user names.UserTag, access params.UserAccessPermission, model names.ModelTag) error {
	return s.modifyAccess(c, user, params.GrantModelAccess, access, model)
}

func (s *modelManagerStateSuite) revoke(c *tc.C, user names.UserTag, access params.UserAccessPermission, model names.ModelTag) error {
	return s.modifyAccess(c, user, params.RevokeModelAccess, access, model)
}

func (s *modelManagerStateSuite) TestGrantMissingUserFails(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	user := names.NewLocalUserTag("foobar")
	err = s.grant(c, user, params.ModelReadAccess, m.ModelTag())
	expectedErr := `could not grant model access: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(err, tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestGrantMissingModelFails(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)
	model := names.NewModelTag("17e4bd2d-3e08-4f3d-b945-087be7ebdce4")
	err := s.grant(c, user.UserTag, params.ModelReadAccess, model)
	expectedErr := `.*model "17e4bd2d-3e08-4f3d-b945-087be7ebdce4" not found`
	c.Assert(err, tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestRevokeAdminLeavesReadAccess(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: permission.WriteAccess})

	err := s.revoke(c, user.UserTag, params.ModelWriteAccess, user.Object.(names.ModelTag))
	c.Assert(err, tc.IsNil)

	modelUser, err := s.State.UserAccess(user.UserTag, user.Object)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
}

func (s *modelManagerStateSuite) TestRevokeReadRemovesModelUser(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)

	err := s.revoke(c, user.UserTag, params.ModelReadAccess, user.Object.(names.ModelTag))
	c.Assert(err, tc.IsNil)

	_, err = s.State.UserAccess(user.UserTag, user.Object)
	c.Assert(errors.IsNotFound(err), tc.IsTrue)
}

func (s *modelManagerStateSuite) TestRevokeModelMissingUser(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	user := names.NewUserTag("bob")
	err = s.revoke(c, user, params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorMatches, `could not revoke model access: model user "bob" does not exist`)

	_, err = st.UserAccess(user, m.ModelTag())
	c.Assert(errors.IsNotFound(err), tc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantOnlyGreaterAccess(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, user.UserTag(), params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, user.UserTag(), params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorMatches, `user already has "read" access or greater`)
}

func (s *modelManagerStateSuite) assertNewUser(c *tc.C, modelUser permission.UserAccess, userTag, creatorTag names.UserTag) {
	c.Assert(modelUser.UserTag, tc.Equals, userTag)
	c.Assert(modelUser.CreatedBy, tc.Equals, creatorTag)
	_, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, tc.Satisfies, state.IsNeverConnectedError)
}

func (s *modelManagerStateSuite) assertModelAccess(c *tc.C, st *state.State) {
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.modelmanager.ModelInfo(params.Entities{Entities: []params.Entity{{Tag: m.ModelTag().String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *modelManagerStateSuite) TestGrantModelAddLocalUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, user.UserTag(), params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	modelUser, err := st.UserAccess(user.UserTag(), m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	s.assertNewUser(c, modelUser, user.UserTag(), apiUser)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
	s.setAPIUser(c, user.UserTag(), apiservertesting.SetTagWithReadAccess(user.UserTag()))
	s.assertModelAccess(c, st)
}

func (s *modelManagerStateSuite) TestGrantModelAddRemoteUser(c *tc.C) {
	userTag := names.NewUserTag("foobar@ubuntuone")
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, userTag, params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	modelUser, err := st.UserAccess(userTag, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	s.assertNewUser(c, modelUser, userTag, apiUser)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
	s.setAPIUser(c, userTag, apiservertesting.SetTagWithReadAccess(userTag))
	s.assertModelAccess(c, st)
}

func (s *modelManagerStateSuite) TestGrantModelAddAdminUser(c *tc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, user.UserTag(), params.ModelWriteAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	modelUser, err := st.UserAccess(user.UserTag(), m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	s.assertNewUser(c, modelUser, user.UserTag(), apiUser)
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)
	s.setAPIUser(c, user.UserTag(), apiservertesting.SetTagWithWriteAccess(user.UserTag()))
	s.assertModelAccess(c, st)
}

func (s *modelManagerStateSuite) TestGrantModelIncreaseAccess(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	stFactory := factory.NewFactory(st, s.StatePool)
	user := stFactory.MakeModelUser(c, &factory.ModelUserParams{Access: permission.ReadAccess})

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, user.UserTag, params.ModelWriteAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	modelUser, err := st.UserAccess(user.UserTag, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.Access, tc.Equals, permission.WriteAccess)
}

func (s *modelManagerStateSuite) TestGrantToModelNoAccess(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	other := names.NewUserTag("other@remote")
	err = s.grant(c, other, params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestGrantToModelReadAccess(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	stFactory := factory.NewFactory(st, s.StatePool)
	stFactory.MakeModelUser(c, &factory.ModelUserParams{
		User: apiUser.Id(), Access: permission.ReadAccess})

	other := names.NewUserTag("other@remote")
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, other, params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestGrantToModelWriteAccess(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	apiUser := names.NewUserTag("admin@remote")
	s.setAPIUser(c, apiUser)
	stFactory := factory.NewFactory(st, s.StatePool)
	stFactory.MakeModelUser(c, &factory.ModelUserParams{
		User: apiUser.Id(), Access: permission.AdminAccess})

	other := names.NewUserTag("other@remote")
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = s.grant(c, other, params.ModelReadAccess, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)

	modelUser, err := st.UserAccess(other, m.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	s.assertNewUser(c, modelUser, other, apiUser)
	c.Assert(modelUser.Access, tc.Equals, permission.ReadAccess)
}

func (s *modelManagerStateSuite) TestGrantModelInvalidUserTag(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify model access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyModelAccessRequest{
			Changes: []params.ModifyModelAccess{{
				ModelTag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
				UserTag:  testParam.tag,
				Action:   params.GrantModelAccess,
				Access:   params.ModelReadAccess,
			}}}

		result, err := s.modelmanager.ModifyModelAccess(args)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
	}
}

func (s *modelManagerStateSuite) TestModifyModelAccessEmptyArgs(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `could not modify model access: "" model access not valid`
	c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModifyModelAccessInvalidAction(c *tc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	var dance params.ModelAction = "dance"
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  "user-user",
			Action:   dance,
			Access:   params.ModelReadAccess,
			ModelTag: s.Model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModelInfoForMigratedModel(c *tc.C) {
	user := names.NewUserTag("admin")

	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: user,
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, tc.ErrorIsNil)

	// Migrate the model and delete it from the state
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: user,
		TargetInfo: migration.TargetInfo{
			ControllerTag:   names.NewControllerTag(utils.MustNewUUID().String()),
			ControllerAlias: "target",
			Addrs:           []string{"1.2.3.4:5555"},
			CACert:          coretesting.CACert,
			AuthTag:         names.NewUserTag("user2"),
			Password:        "secret",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), tc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), tc.ErrorIsNil)

	anAuthoriser := s.authoriser
	anAuthoriser.Tag = user
	st := common.NewUserAwareModelManagerBackend(model, s.StatePool, user)
	endPoint, err := modelmanager.NewModelManagerAPI(
		st,
		common.NewModelManagerBackend(s.Model, s.StatePool),
		nil, nil, common.NewBlockChecker(st), anAuthoriser,
		s.Model,
		s.callContext,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endPoint, tc.NotNil)

	res, err := endPoint.ModelInfo(
		params.Entities{
			Entities: []params.Entity{
				{Tag: model.ModelTag().String()},
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	resErr0 := errors.Cause(res.Results[0].Error)
	c.Assert(params.IsRedirect(resErr0), tc.Equals, true)

	pErr, ok := resErr0.(*params.Error)
	c.Assert(ok, tc.Equals, true)

	var info params.RedirectErrorInfo
	c.Assert(pErr.UnmarshalInfo(&info), tc.ErrorIsNil)

	nhp := params.HostPort{
		Address: params.Address{
			Value: "1.2.3.4",
			Type:  string(network.IPv4Address),
			Scope: string(network.ScopePublic),
		},
		Port: 5555,
	}
	c.Assert(info.Servers, tc.DeepEquals, [][]params.HostPort{{nhp}})
	c.Assert(info.CACert, tc.Equals, coretesting.CACert)
	c.Assert(info.ControllerAlias, tc.Equals, "target")
}

func (s *modelManagerSuite) TestModelStatus(c *tc.C) {
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ModelStatus(params.Entities{[]params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ModelStatus(params.Entities{[]params.Entity{{
		Tag: s.st.ModelTag().String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[1].Error, tc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we return successfully if no errors.
	results, err = s.api.ModelStatus(params.Entities{[]params.Entity{{
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *tc.C) {
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredentialBulkUninterrupted(c *tc.C) {
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: "bad-model-tag"},
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `"bad-model-tag" is not a valid tag`)
	c.Assert(results.Results[1].Error, tc.IsNil)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String()},
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: "bad-credential-tag"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[1].Error, tc.ErrorMatches, `"bad-credential-tag" is not a valid tag`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorisedUser(c *tc.C) {
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	results, err := s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestChangeModelCredentialGetModelFail(c *tc.C) {
	s.st.SetErrors(errors.New("getting model"))
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()

	results, err := s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `getting model`)
	s.st.CheckCallNames(c, "ControllerTag", "ModelTag", "GetBlockForType", "ControllerTag", "GetModel")
}

func (s *modelManagerSuite) TestChangeModelCredentialNotUpdated(c *tc.C) {
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return false, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(params.ChangeModelCredentialsParams{
		[]params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `model deadbeef-0bad-400d-8000-4b1d0d06f00d already uses credential foo/bob/bar`)
}

type fakeProvider struct {
	environs.CloudEnvironProvider
}

func (*fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (*fakeProvider) PrepareForCreateEnvironment(controllerUUID string, cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func init() {
	environs.RegisterProvider("fake", &fakeProvider{})
}
