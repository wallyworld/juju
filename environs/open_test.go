// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdcontext "context"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/provider/dummy"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
	jujuversion "github.com/juju/juju/version"
)

type OpenSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

func TestOpenSuite(t *tctesting.T) {
	tc.Run(t, &OpenSuite{})
}

func (s *OpenSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *OpenSuite) TearDownTest(c *tc.C) {
	dummy.Reset(c)
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *OpenSuite) TestNewDummyEnviron(c *tc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, tc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	cache := jujuclient.NewMemStore()
	controllerCfg := testing.FakeControllerConfig()
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, cache, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   cfg.Name(),
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	env := bootstrapEnviron.(environs.Environ)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, cfg.AgentStream(), cfg.AgentStream())
	err = bootstrap.Bootstrap(ctx, env, context.NewEmptyCloudCallContext(), bootstrap.BootstrapParams{
		ControllerConfig:        controllerCfg,
		AdminSecret:             "admin-secret",
		CAPrivateKey:            testing.CAKey,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)

	// New controller should have been added to collection.
	foundController, err := cache.ControllerByName(cfg.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, tc.DeepEquals, controllerCfg.ControllerUUID())
}

func (s *OpenSuite) TestUpdateEnvInfo(c *tc.C) {
	store := jujuclient.NewMemStore()
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	uuid := utils.MustNewUUID().String()
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "dummy",
		"name": "admin-model",
		"uuid": uuid,
	})
	c.Assert(err, tc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()
	_, err = bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)

	foundController, err := store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundController.ControllerUUID, tc.Not(tc.Equals), "")
	c.Assert(foundController.CACert, tc.Not(tc.Equals), "")
	foundModel, err := store.ModelByName("controller-name", "admin/admin-model")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundModel, tc.DeepEquals, &jujuclient.ModelDetails{
		ModelUUID: cfg.UUID(),
		ModelType: model.IAAS,
	})
}

func (*OpenSuite) TestNewUnknownEnviron(c *tc.C) {
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: "wondercloud",
		},
	})
	c.Assert(err, tc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, tc.IsNil)
}

func (*OpenSuite) TestNewKubernetes(c *tc.C) {
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type: k8sconstants.CAASProviderType,
		},
	})
	c.Assert(err, tc.ErrorMatches, "cloud environ provider kubernetes.kubernetesEnvironProvider not valid")
	c.Assert(env, tc.IsNil)
}

func (*OpenSuite) TestNew(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"controller": false,
			"name":       "erewhemos",
		},
	))
	c.Assert(err, tc.ErrorIsNil)
	e, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  dummy.SampleCloudSpec(),
		Config: cfg,
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = e.ControllerInstances(context.NewEmptyCloudCallContext(), "uuid")
	c.Assert(err, tc.ErrorMatches, "model is not prepared")
}

func (*OpenSuite) TestDestroy(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"name": "erewhemos",
		},
	))
	c.Assert(err, tc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	// Prepare the environment and sanity-check that
	// the config storage info has been made.
	controllerCfg := testing.FakeControllerConfig()
	ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
	bootstrapEnviron, err := bootstrap.PrepareController(false, ctx, store, bootstrap.PrepareParams{
		ControllerConfig: controllerCfg,
		ControllerName:   "controller-name",
		ModelConfig:      cfg.AllAttrs(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, tc.ErrorIsNil)
	e := bootstrapEnviron.(environs.Environ)
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIsNil)

	callCtx := context.NewEmptyCloudCallContext()
	err = environs.Destroy("controller-name", e, callCtx, store)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the environment has actually been destroyed
	// and that the controller details been removed too.
	_, err = e.ControllerInstances(callCtx, controllerCfg.ControllerUUID())
	c.Assert(err, tc.ErrorMatches, "model is not prepared")
	_, err = store.ControllerByName("controller-name")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (*OpenSuite) TestDestroyNotFound(c *tc.C) {
	var env destroyControllerEnv
	store := jujuclient.NewMemStore()
	err := environs.Destroy("fnord", &env, context.NewEmptyCloudCallContext(), store)
	c.Assert(err, tc.ErrorIsNil)
	env.CheckCallNames(c) // no controller details, no call
}

type destroyControllerEnv struct {
	environs.Environ
	testhelpers.Stub
}

func (e *destroyControllerEnv) DestroyController(ctx context.ProviderCallContext, uuid string) error {
	e.MethodCall(e, "DestroyController", ctx, uuid)
	return e.NextErr()
}
