// Copyright Canonical Ltd. 2013
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/dummy"
)

type InitializeSuite struct {
	mgotesting.MgoSuite
	testing.BaseSuite
	Pool  *state.StatePool
	State *state.State
	Model *state.Model
}

func TestInitializeSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &InitializeSuite{})
}

func (s *InitializeSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *InitializeSuite) TearDownSuite(c *tc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *InitializeSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *InitializeSuite) openState(c *tc.C, modelTag names.ModelTag) {
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      testing.ControllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       s.Session,
	})
	c.Assert(err, tc.ErrorIsNil)
	st, err := pool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	s.Pool = pool
	s.State = st

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.Model = m
}

func (s *InitializeSuite) TearDownTest(c *tc.C) {
	if s.Pool != nil {
		err := s.Pool.Close()
		c.Check(err, tc.ErrorIsNil)
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	owner := names.NewLocalUserTag("initialize-admin")

	userPassCredentialTag := names.NewCloudCredentialTag(
		"dummy/" + owner.Id() + "/some-credential",
	)
	emptyCredentialTag := names.NewCloudCredentialTag(
		"dummy/" + owner.Id() + "/empty-credential",
	)
	userpassCredential := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "alice",
			"password": "hunter2",
		},
	)
	userpassCredential.Label = userPassCredentialTag.Name()
	expectedUserpassCredential := statetesting.CloudCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "alice",
			"password": "hunter2",
		},
	)
	expectedUserpassCredential.DocID = "dummy#initialize-admin#some-credential"
	expectedUserpassCredential.Owner = "initialize-admin"
	expectedUserpassCredential.Cloud = "dummy"
	expectedUserpassCredential.Name = "some-credential"

	emptyCredential := cloud.NewEmptyCredential()
	emptyCredential.Label = emptyCredentialTag.Name()
	expectedEmptyCredential := statetesting.NewEmptyCredential()
	expectedEmptyCredential.DocID = "dummy#initialize-admin#empty-credential"
	expectedEmptyCredential.Owner = "initialize-admin"
	expectedEmptyCredential.Cloud = "dummy"
	expectedEmptyCredential.Name = "empty-credential"

	cloudCredentialsIn := map[names.CloudCredentialTag]cloud.Credential{
		userPassCredentialTag: userpassCredential,
		emptyCredentialTag:    emptyCredential,
	}
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			Owner:                   owner,
			Config:                  cfg,
			CloudName:               "dummy",
			CloudRegion:             "dummy-region",
			CloudCredential:         userPassCredentialTag,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name: "dummy",
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.EmptyAuthType, cloud.UserPassAuthType,
			},
			Regions: []cloud.Region{{Name: "dummy-region"}},
		},
		CloudCredentials: cloudCredentialsIn,
		MongoSession:     s.Session,
		AdminPassword:    "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	st, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	cfg, err = s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	expected := cfg.AllAttrs()
	for k, v := range config.ConfigDefaults() {
		if _, ok := expected[k]; !ok {
			expected[k] = v
		}
	}
	c.Assert(cfg.AllAttrs(), tc.DeepEquals, expected)
	// Check that the model has been created.
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Tag(), tc.Equals, modelTag)
	c.Assert(model.CloudRegion(), tc.Equals, "dummy-region")
	// Check that the owner has been created.
	c.Assert(model.Owner(), tc.Equals, owner)
	// Check that the owner can be retrieved by the tag.
	entity, err := s.State.FindEntity(model.Owner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Tag(), tc.Equals, owner)
	// Check that the owner has an ModelUser created for the bootstrapped model.
	modelUser, err := s.State.UserAccess(model.Owner(), model.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUser.UserTag, tc.Equals, owner)
	c.Assert(modelUser.Object, tc.Equals, model.Tag())

	// Check that the model can be found through the tag.
	entity, err = s.State.FindEntity(modelTag)
	c.Assert(err, tc.ErrorIsNil)
	cons, err := s.State.ModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&cons, tc.Satisfies, constraints.IsEmpty)

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	info, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &state.ControllerInfo{ModelTag: modelTag, CloudName: "dummy"})

	// Check that the model's cloud and credential names are as
	// expected, and the owner's cloud credentials are initialised.
	c.Assert(model.CloudName(), tc.Equals, "dummy")
	credentialTag, ok := model.CloudCredentialTag()
	c.Assert(ok, tc.IsTrue)
	c.Assert(credentialTag, tc.Equals, userPassCredentialTag)
	cred, credentialSet, err := model.CloudCredential()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentialSet, tc.IsTrue)
	stateCred, err := s.State.CloudCredential(credentialTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred, tc.DeepEquals, stateCred)
	cloudCredentials, err := s.State.CloudCredentials(model.Owner(), "dummy")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudCredentials, tc.DeepEquals, map[string]state.Credential{
		"dummy/initialize-admin/some-credential":  expectedUserpassCredential,
		"dummy/initialize-admin/empty-credential": expectedEmptyCredential,
	})

	// Check that the cloud owner has admin access.
	access, err := s.State.GetCloudAccess("dummy", owner)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)

	// Check that the cloud's model count is initially 1.
	cl, err := s.State.Cloud("dummy")
	c.Assert(err, tc.ErrorIsNil)

	refCount, err := state.CloudModelRefCount(s.State, cl.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 1)

	// Check that the alpha space is created.
	_, err = s.State.SpaceByName(network.AlphaSpaceName)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the bakery config is created.
	bakeryConfig := s.State.NewBakeryConfig()
	_, err = bakeryConfig.GetLocalUsersKey()
	c.Assert(err, tc.ErrorIsNil)
	_, err = bakeryConfig.GetLocalUsersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)
	_, err = bakeryConfig.GetExternalUsersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)
	_, err = bakeryConfig.GetOffersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)

	// Check ssh server hostkey was inserted.
	key, err := s.State.SSHServerHostKey()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.Equals, testing.SSHServerHostKey)
}

func (s *InitializeSuite) TestInitializeWithInvalidCredentialType(c *tc.C) {
	owner := names.NewLocalUserTag("initialize-admin")
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	credentialTag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/borken")
	_, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  modelCfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name: "dummy",
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.AccessKeyAuthType, cloud.OAuth1AuthType,
			},
		},
		CloudCredentials: map[names.CloudCredentialTag]cloud.Credential{
			credentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	})
	c.Assert(err, tc.ErrorMatches,
		`validating initialization args: validating credential "dummy/initialize-admin/borken" for cloud "dummy": supported auth-types \["access-key" "oauth1"\], "userpass" not supported`,
	)
}

func (s *InitializeSuite) TestInitializeWithControllerInheritedConfig(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	initial := cfg.AllAttrs()
	controllerInheritedConfigIn := map[string]interface{}{
		"charmhub-url": initial["charmhub-url"],
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	st, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	controllerInheritedConfig, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("dummy"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerInheritedConfig.Map(), tc.DeepEquals, controllerInheritedConfigIn)

	expected := cfg.AllAttrs()
	for k, v := range config.ConfigDefaults() {
		if _, ok := expected[k]; !ok {
			expected[k] = v
		}
	}
	// Config as read from state has resources tags coerced to a map.
	expected["resource-tags"] = map[string]string{}
	cfg, err = s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), tc.DeepEquals, expected)
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *tc.C) {
	cfg := testing.ModelConfig(c)
	owner := names.NewLocalUserTag("initialize-admin")

	controllerCfg := testing.FakeControllerConfig()

	args := state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}
	ctlr, err := state.Initialize(args)
	c.Assert(err, tc.ErrorIsNil)
	err = ctlr.Close()
	c.Check(err, tc.ErrorIsNil)

	ctlr, err = state.Initialize(args)
	c.Check(err, tc.ErrorMatches, "already initialized")
	c.Check(ctlr, tc.IsNil)
}

func (s *InitializeSuite) TestModelConfigWithAdminSecret(c *tc.C) {
	update := map[string]interface{}{"admin-secret": "foo"}
	remove := []string{}
	s.testBadModelConfig(c, update, remove, "admin-secret should never be written to the state")
}

func (s *InitializeSuite) TestModelConfigWithCAPrivateKey(c *tc.C) {
	update := map[string]interface{}{"ca-private-key": "foo"}
	remove := []string{}
	s.testBadModelConfig(c, update, remove, "ca-private-key should never be written to the state")
}

func (s *InitializeSuite) TestModelConfigWithoutAgentVersion(c *tc.C) {
	update := map[string]interface{}{}
	remove := []string{"agent-version"}
	s.testBadModelConfig(c, update, remove, "agent-version must always be set in state")
}

func (s *InitializeSuite) testBadModelConfig(c *tc.C, update map[string]interface{}, remove []string, expect string) {
	good := testing.CustomModelConfig(c, testing.Attrs{"uuid": testing.ModelTag.Id()})
	bad, err := good.Apply(update)
	c.Assert(err, tc.ErrorIsNil)
	bad, err = bad.Remove(remove)
	c.Assert(err, tc.ErrorIsNil)

	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	args := state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			CloudRegion:             "dummy-region",
			Owner:                   owner,
			Config:                  bad,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions:   []cloud.Region{{Name: "dummy-region"}},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}
	_, err = state.Initialize(args)
	c.Assert(err, tc.ErrorMatches, expect)

	args.ControllerModelArgs.Config = good
	ctlr, err := state.Initialize(args)
	c.Assert(err, tc.ErrorIsNil)
	sysState, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := sysState.ModelUUID()
	ctlr.Close()

	s.openState(c, names.NewModelTag(modelUUID))
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = m.UpdateModelConfig(update, remove)
	c.Assert(err, tc.ErrorMatches, expect)

	// ModelConfig remains inviolate.
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	goodWithDefaults, err := config.New(config.UseDefaults, good.AllAttrs())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), tc.DeepEquals, goodWithDefaults.AllAttrs())
}

func (s *InitializeSuite) TestCloudConfigWithForbiddenValues(c *tc.C) {
	badAttrNames := []string{
		"admin-secret",
		"ca-private-key",
		config.AgentVersionKey,
	}
	for _, attr := range controller.ControllerOnlyConfigAttributes {
		badAttrNames = append(badAttrNames, attr)
	}

	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	args := state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   names.NewLocalUserTag("initialize-admin"),
			Config:                  modelCfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}

	for _, badAttrName := range badAttrNames {
		badAttrs := map[string]interface{}{badAttrName: "foo"}
		args.ControllerInheritedConfig = badAttrs
		_, err := state.Initialize(args)
		c.Assert(err, tc.ErrorMatches, "local cloud config cannot contain .*")
	}
}

func (s *InitializeSuite) TestInitializeWithCloudRegionConfig(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()

	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"a-key": "a-value",
		},
		"b-region": cloud.Attrs{
			"b-key": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	st, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	for k := range regionInheritedConfigIn {
		// Query for config for each region
		regionInheritedConfig, err := s.State.ReadSettings(
			state.GlobalSettingsC,
			"dummy#"+k)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(
			cloud.Attrs(regionInheritedConfig.Map()),
			tc.DeepEquals,
			regionInheritedConfigIn[k])
	}
}

func (s *InitializeSuite) TestInitializeWithCloudRegionMisses(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	controllerInheritedConfigIn := map[string]interface{}{
		"no-proxy": "local",
	}
	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
		"b-region": cloud.Attrs{
			"no-proxy": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	sysState, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := sysState.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	var attrs map[string]interface{}
	rspec := &environscloudspec.CloudRegionSpec{Cloud: "dummy", Region: "c-region"}
	got, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(got["no-proxy"], tc.Equals, "local")
}

func (s *InitializeSuite) TestInitializeWithCloudRegionHits(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()

	controllerInheritedConfigIn := map[string]interface{}{
		"no-proxy": "local",
	}
	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
		"b-region": cloud.Attrs{
			"no-proxy": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	sysState, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := sysState.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	var attrs map[string]interface{}
	for r := range regionInheritedConfigIn {
		rspec := &environscloudspec.CloudRegionSpec{Cloud: "dummy", Region: r}
		got, err := s.State.ComposeNewModelConfig(attrs, rspec)
		c.Check(err, tc.ErrorIsNil)
		c.Assert(got["no-proxy"], tc.Equals, regionInheritedConfigIn[r]["no-proxy"])
	}
}

func (s *InitializeSuite) TestInitializeWithStoragePool(c *tc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()

	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		SupportsFunc: func(storage.StorageKind) bool {
			return false
		},
	}
	registry := storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"dummy": staticProvider,
		},
	}
	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: registry,
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
		StoragePools: map[string]storage.Attrs{
			"spool": {
				"type": "dummy",
				"foo":  "bar",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctlr, tc.NotNil)
	sysState, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := sysState.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), tc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.openState(c, modelTag)

	pm := poolmanager.New(state.NewStateSettings(s.State), registry)
	storageCfg, err := pm.Get("spool")
	c.Assert(err, tc.ErrorIsNil)
	expectedCfg, err := storage.NewConfig("spool", "dummy", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageCfg, tc.DeepEquals, expectedCfg)
}
