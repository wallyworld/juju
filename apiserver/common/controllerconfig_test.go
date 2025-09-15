// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type controllerConfigSuite struct {
	testing.BaseSuite

	st *mocks.MockControllerConfigState
	cc *common.ControllerConfigAPI
}

func TestControllerConfigSuite(t *tctesting.T) {
	tc.Run(t, &controllerConfigSuite{})
}

func (s *controllerConfigSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockControllerConfigState(ctrl)
	s.cc = common.NewStateControllerConfig(s.st)
	return ctrl
}

func (s *controllerConfigSuite) TearDownTest(c *tc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *controllerConfigSuite) TestControllerConfigSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ControllerConfig().Return(
		map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
			controller.CACertKey:         testing.CACert,
			controller.APIPort:           4321,
			controller.StatePort:         1234,
		},
		nil,
	)

	result, err := s.cc.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(map[string]interface{}(result.Config), tc.DeepEquals, map[string]interface{}{
		"ca-cert":         testing.CACert,
		"controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"state-port":      1234,
		"api-port":        4321,
	})
}

func (s *controllerConfigSuite) TestControllerConfigFetchError(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ControllerConfig().Return(nil, fmt.Errorf("pow"))
	_, err := s.cc.ControllerConfig()
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (s *controllerConfigSuite) expectStateControllerInfo(c *tc.C) {
	s.st.EXPECT().APIHostPortsForAgents().Return([]network.SpaceHostPorts{
		network.NewSpaceHostPorts(17070, "192.168.1.1"),
	}, nil)
	s.st.EXPECT().ControllerConfig().Return(map[string]interface{}{
		controller.CACertKey: testing.CACert,
	}, nil)
}

func (s *controllerConfigSuite) TestControllerInfo(c *tc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ModelExists(testing.ModelTag.Id()).Return(true, nil)
	s.expectStateControllerInfo(c)

	results, err := s.cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: testing.ModelTag.String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, tc.DeepEquals, []string{"192.168.1.1:17070"})
	c.Assert(results.Results[0].CACert, tc.Equals, testing.CACert)
}

type controllerInfoSuite struct {
	jujutesting.JujuConnSuite

	localState *state.State
	localModel *state.Model
}

func TestControllerInfoSuite(t *tctesting.T) {
	tc.Run(t, &controllerInfoSuite{})
}

func (s *controllerInfoSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.localState = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) {
		s.localState.Close()
	})
	model, err := s.localState.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.localModel = model
}

func (s *controllerInfoSuite) TestControllerInfoLocalModel(c *tc.C) {
	cc := common.NewStateControllerConfig(s.State)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: s.localModel.ModelTag().String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	apiAddr, err := systemState.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Addresses, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses[0], tc.Equals, apiAddr[0][0].String())
	c.Assert(results.Results[0].CACert, tc.Equals, testing.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoExternalModel(c *tc.C) {
	ec := state.NewExternalControllers(s.State)
	modelUUID := utils.MustNewUUID().String()
	info := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.1:12345"},
		CACert:        testing.CACert,
	}
	_, err := ec.Save(info, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	cc := common.NewStateControllerConfig(s.State)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(modelUUID).String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, tc.DeepEquals, info.Addrs)
	c.Assert(results.Results[0].CACert, tc.Equals, info.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoMigratedController(c *tc.C) {
	cc := common.NewStateControllerConfig(s.State)
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{})
	model, err := modelState.Model()
	c.Assert(err, tc.ErrorIsNil)

	targetControllerTag := names.NewControllerTag(utils.MustNewUUID().String())
	defer modelState.Close()

	// Migrate the model and delete it from the state
	controllerIP := "1.2.3.4:5555"
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag:   targetControllerTag,
			ControllerAlias: "target",
			Addrs:           []string{controllerIP},
			CACert:          "",
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

	externalControllerInfo, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(model.UUID()).String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(externalControllerInfo.Results), tc.Equals, 1)
	c.Assert(externalControllerInfo.Results[0].Addresses[0], tc.Equals, controllerIP)
}
