// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	coretesting.BaseSuite
	secrets *mockSecrets
}

func TestUndertakerSuite(t *tctesting.T) {
	tc.Run(t, &undertakerSuite{})
}

func (s *undertakerSuite) setupStateAndAPI(c *tc.C, isSystem bool, modelName string, secretsConfigError error) (*mockState, *undertaker.UndertakerAPI) {
	machineNo := "1"
	if isSystem {
		machineNo = "0"
	}

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag(machineNo),
		Controller: true,
	}

	modelCfg, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.IsNil)
	st := newMockState(names.NewUserTag("admin"), modelName, isSystem, *modelCfg)
	s.secrets = &mockSecrets{}
	s.PatchValue(&undertaker.GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.secrets, nil })

	secretBackendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": {
					ModelUUID: "9d3d3b19-2b0c-4a3f-acde-0b1645586a72",
					BackendConfig: provider.BackendConfig{
						BackendType: "some-backend",
					},
				},
			},
		}, secretsConfigError
	}

	api, err := undertaker.NewUndertaker(st, nil, authorizer, secretBackendConfigGetter, nil)
	c.Assert(err, tc.ErrorIsNil)
	return st, api
}

func (s *undertakerSuite) TestNoPerms(c *tc.C) {
	modelCfg, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.IsNil)
	for _, authorizer := range []apiservertesting.FakeAuthorizer{{
		Tag: names.NewMachineTag("0"),
	}, {
		Tag: names.NewUserTag("bob"),
	}} {
		st := newMockState(names.NewUserTag("admin"), "admin", true, *modelCfg)
		_, err := undertaker.NewUndertaker(
			st,
			nil,
			authorizer,
			func() (*provider.ModelBackendConfigInfo, error) {
				return nil, errors.NotImplemented
			},
			nil,
		)
		c.Assert(err, tc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestModelInfo(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)
	st, api := s.setupStateAndAPI(c, true, "admin", nil)
	for _, test := range []struct {
		st        *mockState
		api       *undertaker.UndertakerAPI
		isSystem  bool
		modelName string
	}{
		{otherSt, hostedAPI, false, "hostedmodel"},
		{st, api, true, "admin"},
	} {
		test.st.model.life = state.Dying
		test.st.model.forced = true
		minute := time.Minute
		test.st.model.timeout = &minute

		result, err := test.api.ModelInfo()
		c.Assert(err, tc.ErrorIsNil)

		info := result.Result
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.Error, tc.IsNil)

		c.Assert(info.UUID, tc.Equals, test.st.model.UUID())
		c.Assert(info.GlobalName, tc.Equals, "user-admin/"+test.modelName)
		c.Assert(info.Name, tc.Equals, test.modelName)
		c.Assert(info.IsSystem, tc.Equals, test.isSystem)
		c.Assert(info.Life, tc.Equals, life.Dying)
		c.Assert(info.ForceDestroyed, tc.Equals, true)
		c.Assert(info.DestroyTimeout, tc.NotNil)
		c.Assert(*info.DestroyTimeout, tc.Equals, time.Minute)
		c.Assert(info.ControllerUUID, tc.Equals, test.st.controllerUUID)
	}
}

func (s *undertakerSuite) TestProcessDyingModel(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)
	model, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, tc.ErrorMatches, "model is not dying")
	c.Assert(model.Life(), tc.Equals, state.Alive)

	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, tc.IsNil)
	c.Assert(model.Life(), tc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveModel(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)
	_, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, tc.ErrorMatches, "model not dying or dead")
}

func (s *undertakerSuite) TestRemoveDyingModel(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)

	// Set model to dying
	otherSt.model.life = state.Dying

	c.Assert(hostedAPI.RemoveModel(), tc.ErrorIsNil)
}

func (s *undertakerSuite) TestDeadRemoveModel(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)

	// Set model to dead
	otherSt.model.life = state.Dying
	err := hostedAPI.ProcessDyingModel()
	c.Assert(err, tc.IsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(otherSt.removed, tc.IsTrue)
}

func (s *undertakerSuite) TestRemoveModelSecrets(c *tc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)

	err := hostedAPI.RemoveModelSecrets()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.secrets.cleanedUUID, tc.Equals, otherSt.model.uuid)
}

func (s *undertakerSuite) TestRemoveModelSecretsConfigNotFound(c *tc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", errors.NotFound)

	err := hostedAPI.RemoveModelSecrets()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.secrets.cleanedUUID, tc.Equals, "")
}

func (s *undertakerSuite) TestModelConfig(c *tc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)

	cfg, err := hostedAPI.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.NotNil)
}

func (s *undertakerSuite) TestSetStatus(c *tc.C) {
	mock, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)

	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			mock.model.Tag().String(), status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(mock.model.status, tc.Equals, status.Destroying)
	c.Assert(mock.model.statusInfo, tc.Equals, "woop")
	c.Assert(mock.model.statusData, tc.DeepEquals, map[string]interface{}{"da": "ta"})
}

func (s *undertakerSuite) TestSetStatusControllerPermissions(c *tc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, true, "hostedmodel", nil)
	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			"model-6ada782f-bcd4-454b-a6da-d1793fbcb35e", status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, ".*not found")
}

func (s *undertakerSuite) TestSetStatusNonControllerPermissions(c *tc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel", nil)
	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			"model-6ada782f-bcd4-454b-a6da-d1793fbcb35e", status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "permission denied")
}
