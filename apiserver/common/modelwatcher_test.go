// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelWatcherSuite struct {
	testing.BaseSuite
}

func TestModelWatcherSuite(t *tctesting.T) {
	tc.Run(t, &modelWatcherSuite{})
}

type fakeModelAccessor struct {
	modelConfig      *config.Config
	modelConfigError error
}

func (*fakeModelAccessor) WatchForModelConfigChanges() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}

func (f *fakeModelAccessor) ModelConfig() (*config.Config, error) {
	if f.modelConfigError != nil {
		return nil, f.modelConfigError
	}
	return f.modelConfig, nil
}

func (s *modelWatcherSuite) TearDownTest(c *tc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *modelWatcherSuite) TestWatchSuccess(c *tc.C) {
	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })
	e := common.NewModelWatcher(
		&fakeModelAccessor{},
		resources,
		nil,
	)
	result, err := e.WatchForModelConfigChanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(resources.Count(), tc.Equals, 1)
}

func (*modelWatcherSuite) TestModelConfigSuccess(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	testingModelConfig := testingEnvConfig(c)
	e := common.NewModelWatcher(
		&fakeModelAccessor{modelConfig: testingModelConfig},
		nil,
		authorizer,
	)
	result, err := e.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	// Make sure we can read the secret attribute (i.e. it's not masked).
	c.Check(result.Config["secret"], tc.Equals, "pork")
	c.Check(map[string]interface{}(result.Config), tc.DeepEquals, testingModelConfig.AllAttrs())
}

func (*modelWatcherSuite) TestModelConfigFetchError(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	e := common.NewModelWatcher(
		&fakeModelAccessor{
			modelConfigError: fmt.Errorf("pow"),
		},
		nil,
		authorizer,
	)
	_, err := e.ModelConfig()
	c.Assert(err, tc.ErrorMatches, "pow")
}

func testingEnvConfig(c *tc.C) *config.Config {
	env, err := bootstrap.PrepareController(
		false,
		modelcmd.BootstrapContext(c.Context(), cmdtesting.Context(c)),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ControllerName:   "dummycontroller",
			ModelConfig:      dummy.SampleConfig(),
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return env.Config()
}
