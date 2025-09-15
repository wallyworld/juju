// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasmodeloperator"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloudconfig/podcfg"
	coretesting "github.com/juju/juju/internal/testing"
	statetesting "github.com/juju/juju/state/testing"
)

type ModelOperatorSuite struct {
	coretesting.BaseSuite

	authorizer *apiservertesting.FakeAuthorizer
	api        *caasmodeloperator.API
	resources  *common.Resources
	state      *mockState
}

func TestModelOperatorSuite(t *tctesting.T) {
	tc.Run(t, &ModelOperatorSuite{})
}

func (m *ModelOperatorSuite) SetUpTest(c *tc.C) {
	m.BaseSuite.SetUpTest(c)

	m.resources = common.NewResources()

	m.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewModelTag("model-deadbeef-0bad-400d-8000-4b1d0d06f00d"),
		Controller: true,
	}

	m.state = newMockState()
	m.state.operatorRepo = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}`[1:]

	c.Logf("m.state.1operatorRepo %q", m.state.operatorRepo)

	api, err := caasmodeloperator.NewAPI(m.authorizer, m.resources, m.state, m.state)
	c.Assert(err, tc.ErrorIsNil)

	m.api = api
}

func (m *ModelOperatorSuite) TestProvisioningInfo(c *tc.C) {
	info, err := m.api.ModelOperatorProvisioningInfo()
	c.Assert(err, tc.ErrorIsNil)

	controllerConf, err := m.state.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	imagePath, err := podcfg.GetJujuOCIImagePathFromControllerCfg(controllerConf, info.Version)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imagePath, tc.Equals, info.ImageDetails.RegistryPath)

	c.Assert(info.ImageDetails.Auth, tc.Equals, `xxxxx==`)
	c.Assert(info.ImageDetails.Repository, tc.Equals, `test-account`)

	model, err := m.state.Model()
	c.Assert(err, tc.ErrorIsNil)

	modelConfig, err := model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)

	vers, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)

	c.Assert(vers, tc.DeepEquals, info.Version)
}

func (m *ModelOperatorSuite) TestWatchProvisioningInfo(c *tc.C) {
	controllerConfigChanged := make(chan struct{}, 1)
	modelConfigChanged := make(chan struct{}, 1)
	apiHostPortsForAgentsChanged := make(chan struct{}, 1)
	m.state.controllerConfigWatcher = statetesting.NewMockNotifyWatcher(controllerConfigChanged)
	m.state.apiHostPortsForAgentsWatcher = statetesting.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)
	m.state.model.modelConfigChanged = statetesting.NewMockNotifyWatcher(modelConfigChanged)

	controllerConfigChanged <- struct{}{}
	apiHostPortsForAgentsChanged <- struct{}{}
	modelConfigChanged <- struct{}{}

	results, err := m.api.WatchModelOperatorProvisioningInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Error, tc.IsNil)
	res := m.resources.Get("1")
	c.Assert(res, tc.FitsTypeOf, (*common.MultiNotifyWatcher)(nil))
}
