// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/undertaker"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
)

// TODO(fwereade) 2016-03-17 lp:1558668
// this is not a feature test; much of it is redundant, and other
// bits should be tested elsewhere.
type undertakerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *undertakerSuite) TestPermDenied(c *tc.C) {
	nonManagerMachine, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	for _, conn := range []api.Connection{
		nonManagerMachine,
		s.APIState,
	} {
		undertakerClient, err := undertaker.NewClient(conn, apiwatcher.NewNotifyWatcher)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(undertakerClient, tc.NotNil)

		_, err = undertakerClient.ModelInfo()
		c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
			Message: "permission denied",
			Code:    "unauthorized access",
		})
	}
}

func (s *undertakerSuite) TestStateEnvironInfo(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient, tc.NotNil)

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	c.Assert(result.Error, tc.IsNil)
	info := result.Result
	c.Assert(info.UUID, tc.Equals, coretesting.ModelTag.Id())
	c.Assert(info.Name, tc.Equals, "controller")
	c.Assert(info.GlobalName, tc.Equals, "user-admin/controller")
	c.Assert(info.IsSystem, tc.IsTrue)
	c.Assert(info.Life, tc.Equals, life.Alive)
}

func (s *undertakerSuite) TestStateProcessDyingEnviron(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient, tc.NotNil)

	err = undertakerClient.ProcessDyingModel()
	c.Assert(err, tc.ErrorMatches, "model is not dying")

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	err = undertakerClient.ProcessDyingModel()
	c.Assert(err, tc.ErrorMatches, `model not empty, found 1 machine \(model not empty\)`)
}

func (s *undertakerSuite) TestStateRemoveEnvironFails(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	undertakerClient, err := undertaker.NewClient(st, apiwatcher.NewNotifyWatcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient, tc.NotNil)
	c.Assert(undertakerClient.RemoveModel(), tc.ErrorMatches, "can't remove model: model still alive")
}

func (s *undertakerSuite) TestHostedEnvironInfo(c *tc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	result, err := undertakerClient.ModelInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	c.Assert(result.Error, tc.IsNil)
	envInfo := result.Result
	c.Assert(envInfo.UUID, tc.Equals, otherSt.ModelUUID())
	c.Assert(envInfo.Name, tc.Equals, "hosted-env")
	c.Assert(envInfo.GlobalName, tc.Equals, "user-admin/hosted-env")
	c.Assert(envInfo.IsSystem, tc.IsFalse)
	c.Assert(envInfo.Life, tc.Equals, life.Alive)
}

func (s *undertakerSuite) TestHostedProcessDyingEnviron(c *tc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	err := undertakerClient.ProcessDyingModel()
	c.Assert(err, tc.ErrorMatches, "model is not dying")

	factory.NewFactory(otherSt, s.StatePool).MakeApplication(c, nil)
	model, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	err = otherSt.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient.ProcessDyingModel(), tc.ErrorIsNil)

	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
}

func (s *undertakerSuite) TestWatchModelResources(c *tc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	w, err := undertakerClient.WatchModelResources()
	c.Assert(err, tc.ErrorIsNil)
	defer w.Kill()
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
	wc.AssertStops()
}

func (s *undertakerSuite) TestHostedRemoveEnviron(c *tc.C) {
	undertakerClient, otherSt := s.hostedAPI(c)
	defer otherSt.Close()

	// Aborts on alive environ.
	err := undertakerClient.RemoveModel()
	c.Assert(err, tc.ErrorMatches, "can't remove model: model still alive")

	factory.NewFactory(otherSt, s.StatePool).MakeApplication(c, nil)
	model, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)

	err = otherSt.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient.ProcessDyingModel(), tc.ErrorIsNil)

	c.Assert(undertakerClient.RemoveModel(), tc.ErrorIsNil)
	c.Assert(otherSt.EnsureModelRemoved(), tc.ErrorIsNil)
}

func (s *undertakerSuite) hostedAPI(c *tc.C) (*undertaker.Client, *state.State) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "hosted-env"})

	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:     []state.MachineJob{state.JobManageModel},
		Password: password,
		Nonce:    "fake_nonce",
	})

	// Connect to hosted environ from controller.
	info := s.APIInfo(c)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = names.NewModelTag(otherState.ModelUUID())

	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, tc.ErrorIsNil)

	undertakerClient, err := undertaker.NewClient(otherAPIState, apiwatcher.NewNotifyWatcher)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(undertakerClient, tc.NotNil)

	return undertakerClient, otherState
}

var fakeSecretDeleter = func(uri *secrets.URI, revision int) error {
	return nil
}
