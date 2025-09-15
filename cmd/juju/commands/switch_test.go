// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"os"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type SwitchSimpleSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testhelpers.Stub
	store     *jujuclient.MemStore
	stubStore *jujuclienttesting.StubStore
	onRefresh func()
}

func TestSwitchSimpleSuite(t *tctesting.T) {
	tc.Run(t, &SwitchSimpleSuite{})
}

func (s *SwitchSimpleSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.store = jujuclient.NewMemStore()
	s.stubStore = jujuclienttesting.WrapClientStore(s.store)
	s.onRefresh = nil
}

func (s *SwitchSimpleSuite) refreshModels(store jujuclient.ClientStore, controllerName string) error {
	s.MethodCall(s, "RefreshModels", store, controllerName)
	if s.onRefresh != nil {
		s.onRefresh()
	}
	return s.NextErr()
}

func (s *SwitchSimpleSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	cmd := &switchCommand{
		Store:         s.stubStore,
		RefreshModels: s.refreshModels,
	}
	return cmdtesting.RunCommand(c, modelcmd.WrapBase(cmd), args...)
}

func (s *SwitchSimpleSuite) TestNoArgs(c *tc.C) {
	_, err := s.run(c)
	c.Assert(err, tc.ErrorMatches, common.MissingModelNameError("switch").Error())
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentController(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	ctx, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "a-controller\n")
}

func (s *SwitchSimpleSuite) TestUnknownControllerNameReturnsError(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	_, err := s.run(c, "another-controller:modela")
	c.Assert(err, tc.ErrorMatches, "controller another-controller not found")
}

func (s *SwitchSimpleSuite) TestNoArgsCurrentModel(c *tc.C) {
	s.addController(c, "a-controller")
	s.store.CurrentControllerName = "a-controller"
	s.store.Models["a-controller"] = &jujuclient.ControllerModels{
		Models:       map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
		CurrentModel: "admin/mymodel",
	}
	ctx, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "a-controller:admin/mymodel\n")
}

func (s *SwitchSimpleSuite) TestSwitchWritesCurrentController(c *tc.C) {
	s.addController(c, "a-controller")
	context, err := s.run(c, "a-controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, " -> a-controller (controller)\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"a-controller"}},
		{"CurrentModel", []interface{}{"a-controller"}},
		{"SetCurrentController", []interface{}{"a-controller"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchWithCurrentController(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrent(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	context, err := s.run(c, "new")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerWithCurrentExplicit(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	context, err := s.run(c, "new:")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchSameController(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	context, err := s.run(c, "same")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same (controller) (no change)\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"same"}},
		{"CurrentModel", []interface{}{"same"}},
		{"ControllerByName", []interface{}{"same"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModel(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ctrl (controller) -> ctrl:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"ctrl"}},
		{"CurrentModel", []interface{}{"ctrl"}},
		{"ControllerByName", []interface{}{"mymodel"}},
		{"AccountDetails", []interface{}{"ctrl"}},
		{"SetCurrentModel", []interface{}{"ctrl", "admin/mymodel"}},
	})
	c.Assert(s.store.Models["ctrl"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToModelDifferentController(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
	c.Assert(s.store.Models["new"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerSameNameAsModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.addController(c, "old")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}, "admin/old": {}},
	}
	s.store.Models["old"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/somemodel": {}},
	}
	_, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, "old")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "new:admin/mymodel -> old (controller)\n")
}

func (s *SwitchSimpleSuite) TestSwitchControllerSameNameAsModelExplicitModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "new")
	s.addController(c, "old")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}, "admin/old": {}},
	}
	s.store.Models["old"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/somemodel": {}},
	}
	_, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	context, err := s.run(c, ":old")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "new:admin/mymodel -> new:admin/old\n")
}

func (s *SwitchSimpleSuite) TestSwitchLocalControllerToModelDifferentController(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
	c.Assert(s.store.Models["new"].CurrentModel, tc.Equals, "admin/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchControllerToDifferentControllerCurrentModel(c *tc.C) {
	s.store.CurrentControllerName = "old"
	s.addController(c, "old")
	s.addController(c, "new")
	s.store.Models["new"] = &jujuclient.ControllerModels{
		Models:       map[string]jujuclient.ModelDetails{"admin/mymodel": {}},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "new:mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "old (controller) -> new:admin/mymodel\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"old"}},
		{"CurrentModel", []interface{}{"old"}},
		{"ControllerByName", []interface{}{"new:mymodel"}},
		{"ControllerByName", []interface{}{"new"}},
		{"AccountDetails", []interface{}{"new"}},
		{"SetCurrentModel", []interface{}{"new", "admin/mymodel"}},
		{"SetCurrentController", []interface{}{"new"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchToModelDifferentOwner(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/mymodel":  {},
			"bianca/mymodel": {},
		},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "bianca/mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same:admin/mymodel -> same:bianca/mymodel\n")
	c.Assert(s.store.Models["same"].CurrentModel, tc.Equals, "bianca/mymodel")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownNoCurrentController(c *tc.C) {
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"unknown"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModels(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.onRefresh = func() {
		s.store.Models["ctrl"] = &jujuclient.ControllerModels{
			Models: map[string]jujuclient.ModelDetails{"admin/unknown": {}},
		}
	}
	ctx, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "ctrl (controller) -> ctrl:admin/unknown\n")
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsStillUnknown(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, `"unknown" is not the name of a model or controller`)
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSwitchUnknownCurrentControllerRefreshModelsFails(c *tc.C) {
	s.store.CurrentControllerName = "ctrl"
	s.addController(c, "ctrl")
	s.SetErrors(errors.New("not very refreshing"))
	_, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches, "refreshing models cache: not very refreshing")
	s.CheckCallNames(c, "RefreshModels")
}

func (s *SwitchSimpleSuite) TestSettingWhenModelEnvVarSet(c *tc.C) {
	os.Setenv("JUJU_MODEL", "using-model")
	_, err := s.run(c, "erewhemos-2")
	c.Assert(err, tc.ErrorMatches, `cannot switch when JUJU_MODEL is overriding the model \(set to "using-model"\)`)
}

func (s *SwitchSimpleSuite) TestSettingWhenControllerEnvVarSet(c *tc.C) {
	os.Setenv("JUJU_CONTROLLER", "using-controller")
	_, err := s.run(c, "erewhemos-2")
	c.Assert(err, tc.ErrorMatches, `cannot switch when JUJU_CONTROLLER is overriding the controller \(set to "using-controller"\)`)
}

func (s *SwitchSimpleSuite) TestTooManyParams(c *tc.C) {
	_, err := s.run(c, "foo", "bar")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: ."bar".`)
}

func (s *SwitchSimpleSuite) addController(c *tc.C, name string) {
	s.store.Controllers[name] = jujuclient.ControllerDetails{}
	s.store.Accounts[name] = jujuclient.AccountDetails{
		User: "admin",
	}
}

func (s *SwitchSimpleSuite) TestSwitchCurrentModelInStore(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/mymodel": {},
		},
		CurrentModel: "admin/mymodel",
	}
	context, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "same:admin/mymodel (no change)\n")
	s.stubStore.CheckCalls(c, []testhelpers.StubCall{
		{"CurrentController", nil},
		{"ControllerByName", []interface{}{"same"}},
		{"CurrentModel", []interface{}{"same"}},
		{"ControllerByName", []interface{}{"mymodel"}},
		{"AccountDetails", []interface{}{"same"}},
		{"SetCurrentModel", []interface{}{"same", "admin/mymodel"}},
	})
}

func (s *SwitchSimpleSuite) TestSwitchCurrentModelNoLongerInStore(c *tc.C) {
	s.store.CurrentControllerName = "same"
	s.addController(c, "same")
	s.store.Models["same"] = &jujuclient.ControllerModels{CurrentModel: "admin/mymodel"}
	_, err := s.run(c, "mymodel")
	c.Assert(err, tc.ErrorMatches, `"mymodel" is not the name of a model or controller`)
}
