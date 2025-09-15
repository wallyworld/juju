// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"
	"io"
	"os"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type ModelCommandSuite struct {
	testhelpers.IsolationSuite
	store *jujuclient.MemStore
}

func (s *ModelCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")

	s.store = jujuclient.NewMemStore()
}

func TestModelCommandSuite(t *tctesting.T) {
	tc.Run(t, &ModelCommandSuite{})
}

var modelCommandModelTests = []struct {
	about            string
	args             []string
	modelEnvVar      string
	expectController string
	expectModel      string
}{{
	about:            "explicit controller and model, long form",
	args:             []string{"--model", "bar:noncurrentbar"},
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "explicit controller and model, short form",
	args:             []string{"-m", "bar:noncurrentbar"},
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "explicit controller and model UUID, long form",
	args:             []string{"--model", "bar:uuidbar2"},
	expectController: "bar",
	expectModel:      "uuidbar2",
}, {
	about:            "explicit controller and model UUID, short form",
	args:             []string{"-m", "bar:uuidbar2"},
	expectController: "bar",
	expectModel:      "uuidbar2",
}, {
	about:            "implicit controller, explicit model, short form",
	args:             []string{"-m", "explicit"},
	expectController: "foo",
	expectModel:      "explicit",
}, {
	about:            "implicit controller, explicit model, long form",
	args:             []string{"--model", "explicit"},
	expectController: "foo",
	expectModel:      "explicit",
}, {
	about:            "implicit controller, explicit model UUID, short form",
	args:             []string{"-m", "uuidfoo3"},
	expectController: "foo",
	expectModel:      "uuidfoo3",
}, {
	about:            "implicit controller, explicit model UUID, long form",
	args:             []string{"--model", "uuidfoo3"},
	expectController: "foo",
	expectModel:      "uuidfoo3",
}, {
	about:            "explicit controller, implicit model",
	args:             []string{"--model", "bar:"},
	expectController: "bar",
	expectModel:      "adminbar/currentbar",
}, {
	about:            "controller and model in env var",
	modelEnvVar:      "bar:noncurrentbar",
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "model only in env var",
	modelEnvVar:      "noncurrentfoo",
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}, {
	about:            "controller only in env var",
	modelEnvVar:      "bar:",
	expectController: "bar",
	expectModel:      "adminbar/currentbar",
}, {
	about:            "explicit overrides model env var",
	modelEnvVar:      "foo:noncurrentbar",
	args:             []string{"-m", "noncurrentfoo"},
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}, {
	about:            "explicit overrides controller & store env var",
	modelEnvVar:      "bar:noncurrentbar",
	args:             []string{"-m", "foo:noncurrentfoo"},
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}}

func (s *ModelCommandSuite) TestModelIdentifier(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.Controllers["bar"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	s.store.Accounts["bar"] = jujuclient.AccountDetails{
		User: "baz", Password: "hunter3",
	}

	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "adminfoo/noncurrentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo2", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "bar/explicit",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo3", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "bar/noncurrentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo4", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "adminbar/currentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar1", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "adminbar/noncurrentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar2", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "baz/noncurrentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar3", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.SetCurrentModel("bar", "adminbar/currentbar")
	c.Assert(err, tc.ErrorIsNil)

	for i, test := range modelCommandModelTests {
		c.Logf("test %d: %v", i, test.about)
		os.Setenv(osenv.JujuModelEnvKey, test.modelEnvVar)
		s.assertRunHasModel(c, test.expectController, test.expectModel, test.args...)
	}
}

func (s *ModelCommandSuite) TestModelType(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, tc.ErrorIsNil)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, tc.ErrorIsNil)
	modelType, err := cmd.ModelType()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelType, tc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestModelGeneration(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS, ActiveBranch: "new-branch"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, tc.ErrorIsNil)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, tc.ErrorIsNil)
	modelGeneration, err := cmd.ActiveBranch()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelGeneration, tc.Equals, "new-branch")

	c.Assert(cmd.SetActiveBranch(model.GenerationMaster), tc.ErrorIsNil)
	modelGeneration, err = cmd.ActiveBranch()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelGeneration, tc.Equals, model.GenerationMaster)
}

func (s *ModelCommandSuite) TestBootstrapContext(c *tc.C) {
	ctx := modelcmd.BootstrapContext(c.Context(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsTrue)
}

func (s *ModelCommandSuite) TestBootstrapContextNoVerify(c *tc.C) {
	ctx := modelcmd.BootstrapContextNoVerify(c.Context(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsFalse)
}

func (s *ModelCommandSuite) TestWrapWithoutFlags(c *tc.C) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd,
		modelcmd.WrapSkipModelFlags,
		modelcmd.WrapSkipModelInit,
	)
	args := []string{"-m", "testmodel"}
	err := cmdtesting.InitCommand(wrapped, args)
	// 1st position is always the flag
	msg := fmt.Sprintf("option provided but not defined: %v", args[0])
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *ModelCommandSuite) TestWrapWithFlagsAndWithoutModelInit(c *tc.C) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd,
		modelcmd.WrapSkipModelInit,
	)
	args := []string{"-m", "testmodel"}
	err := cmdtesting.InitCommand(wrapped, args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestWrapWithModelInit(c *tc.C) {
	modelCmd := new(testCommand)
	wrapped := modelcmd.Wrap(modelCmd,
		modelcmd.WrapSkipModelInit,
	)
	args := []string{}

	_, err := cmdtesting.RunCommand(c, wrapped, args...)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestInnerCommand(c *tc.C) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd)
	c.Assert(modelcmd.InnerCommand(wrapped), tc.Equals, cmd)
}

func (*ModelCommandSuite) TestSplitModelName(c *tc.C) {
	assert := func(in, controller, model string) {
		outController, outModel := modelcmd.SplitModelName(in)
		c.Assert(outController, tc.Equals, controller)
		c.Assert(outModel, tc.Equals, model)
	}
	assert("model", "", "model")
	assert("ctrl:model", "ctrl", "model")
	assert("ctrl:", "ctrl", "")
	assert(":model", "", "model")
}

func (*ModelCommandSuite) TestJoinModelName(c *tc.C) {
	assert := func(controller, model, expect string) {
		out := modelcmd.JoinModelName(controller, model)
		c.Assert(out, tc.Equals, expect)
	}
	assert("ctrl", "", "ctrl:")
	assert("", "model", ":model")
	assert("ctrl", "model", "ctrl:model")
}

// assertRunHasModel asserts that a command, when run with the given arguments,
// ends up with the given controller and model names.
func (s *ModelCommandSuite) assertRunHasModel(c *tc.C, expectControllerName, expectModelName string, args ...string) {
	cmd, err := runTestCommand(c, s.store, args...)
	c.Assert(err, tc.ErrorIsNil)

	controllerName, err := cmd.ControllerName()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerName, tc.Equals, expectControllerName)

	modelName, err := cmd.ModelIdentifier()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelName, tc.Equals, expectModelName)
}

func (s *ModelCommandSuite) TestIAASOnlyCommandIAASModel(c *tc.C) {
	s.setupIAASModel(c)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, tc.ErrorIsNil)

	modelType, err := cmd.ModelType()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelType, tc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestIAASOnlyCommandCAASModel(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.CAAS})
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, tc.ErrorIsNil)

	_, err = runTestCommand(c, s.store)
	c.Assert(err, tc.ErrorMatches, `Juju command "test-command" not supported on container models`)
}

func (s *ModelCommandSuite) TestCAASOnlyCommandIAASModel(c *tc.C) {
	s.setupIAASModel(c)

	_, err := runCaasCommand(c, s.store)
	c.Assert(err, tc.ErrorMatches, `Juju command "caas-command" only supported on k8s container models`)
}

func (s *ModelCommandSuite) TestAllowedCommandCAASModel(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.CAAS})
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, tc.ErrorIsNil)

	cmd, err := runAllowedCAASCommand(c, s.store)
	c.Assert(err, tc.ErrorIsNil)
	modelType, err := cmd.ModelType()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelType, tc.Equals, model.CAAS)
}

func (s *ModelCommandSuite) TestPartialModelUUIDSuccess(c *tc.C) {
	s.setupIAASModel(c)

	cmd, err := runTestCommand(c, s.store, "-m", "uuidfoo")
	c.Assert(err, tc.ErrorIsNil)

	modelType, err := cmd.ModelType()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelType, tc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestPartialModelUUIDTooShortError(c *tc.C) {
	s.setupIAASModel(c)

	_, err := runTestCommand(c, s.store, "-m", "uuidf")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ModelCommandSuite) setupIAASModel(c *tc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, tc.ErrorIsNil)
}

func noOpRefresh(_ jujuclient.ClientStore, _ string) error {
	return nil
}

func runTestCommand(c *tc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(testCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type testCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
}

func (c *testCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "test-command",
	})
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	return nil
}

func runCaasCommand(c *tc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(caasCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type caasCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.CAASOnlyCommand
}

func (c *caasCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "caas-command",
	})
}

func (c *caasCommand) Run(ctx *cmd.Context) error {
	return nil
}

func runAllowedCAASCommand(c *tc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(allowedCAASCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type allowedCAASCommand struct {
	modelcmd.ModelCommandBase
}

func (c *allowedCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "allowed-caas-command",
	})
}

func (c *allowedCAASCommand) Run(ctx *cmd.Context) error {
	return nil
}

func TestMacaroonLoginSuite(t *tctesting.T) {
	tc.Run(t, &macaroonLoginSuite{})
}

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	store          *jujuclient.MemStore
	controllerName string
	modelName      string
	apiOpen        api.OpenFunc
}

const testUser = "testuser@somewhere"

func (s *macaroonLoginSuite) SetUpTest(c *tc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.MacaroonSuite.AddModelUser(c, testUser)
	s.MacaroonSuite.AddControllerUser(c, testUser, permission.LoginAccess)

	s.controllerName = "my-controller"
	s.modelName = testUser + "/my-model"
	modelTag := names.NewModelTag(s.State.ModelUUID())
	apiInfo := s.APIInfo(c)

	s.store = jujuclient.NewMemStore()
	s.store.Controllers[s.controllerName] = jujuclient.ControllerDetails{
		APIEndpoints:   apiInfo.Addrs,
		ControllerUUID: s.State.ControllerUUID(),
		CACert:         apiInfo.CACert,
	}
	s.store.Accounts[s.controllerName] = jujuclient.AccountDetails{
		// External user forces use of macaroons.
		User:      testUser,
		Macaroons: []macaroon.Slice{{}},
	}
	s.store.Models[s.controllerName] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			s.modelName: {ModelUUID: modelTag.Id(), ModelType: model.IAAS},
		},
	}
	s.apiOpen = func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		mac, err := apitesting.NewMacaroon("test")
		c.Assert(err, tc.ErrorIsNil)
		info.Macaroons = []macaroon.Slice{{mac}}
		return api.Open(info, dialOpts)
	}
}

func (s *macaroonLoginSuite) newModelCommandBase() *modelcmd.ModelCommandBase {
	var c modelcmd.ModelCommandBase
	c.SetClientStore(s.store)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, &c)
	modelcmd.SetRunStarted(&c)
	err := c.SetModelIdentifier(s.controllerName+":"+s.modelName, false)
	if err != nil {
		panic(err)
	}
	return &c
}

func (s *macaroonLoginSuite) TestsSuccessfulLogin(c *tc.C) {
	s.DischargerLogin = func() string {
		return testUser
	}

	cmd := s.newModelCommandBase()
	_, err := cmd.NewAPIRoot()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsFailToObtainDischargeLogin(c *tc.C) {
	s.DischargerLogin = func() string {
		return ""
	}

	cmd := s.newModelCommandBase()
	cmd.SetAPIOpen(s.apiOpen)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, tc.ErrorMatches, "cannot get discharge.*", tc.Commentf("%s", errors.Details(err)))
}
