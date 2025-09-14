// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdcontext "context"
	"os"
	"path"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/vault"
)

type InterfaceSuite struct {
	HookContextSuite
}

func TestInterfaceSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &InterfaceSuite{})
}

func (s *InterfaceSuite) TestUnitName(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), tc.Equals, "u/0")
}

func (s *InterfaceSuite) TestHookRelation(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	r, err := ctx.HookRelation()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(r, tc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	name, err := ctx.RemoteUnitName()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(name, tc.Equals, "")
}

func (s *InterfaceSuite) TestRemoteApplicationName(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	name, err := ctx.RemoteApplicationName()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(name, tc.Equals, "")
}

func (s *InterfaceSuite) TestWorkloadName(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	name, err := ctx.WorkloadName()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(name, tc.Equals, "")
}

func (s *InterfaceSuite) TestRelationIds(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	relIds, err := ctx.RelationIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relIds, tc.HasLen, 2)
	c.Assert(relIds, tc.SameContents, []int{0, 1})
	r, err := ctx.Relation(0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Name(), tc.Equals, "db")
	c.Assert(r.FakeId(), tc.Equals, "db:0")
	r, err = ctx.Relation(123)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(r, tc.IsNil)
}

func (s *InterfaceSuite) TestRelationIdsExcludesBroken(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	// Broken relations have no member settings.
	context.SetRelationBroken(ctx, 1)
	relIds, err := ctx.RelationIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relIds, tc.HasLen, 1)
	c.Assert(relIds, tc.SameContents, []int{0})
}

func (s *InterfaceSuite) TestRelationContext(c *tc.C) {
	ctx := s.GetContext(c, 1, "", names.StorageTag{})
	r, err := ctx.HookRelation()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Name(), tc.Equals, "db")
	c.Assert(r.FakeId(), tc.Equals, "db:1")
}

func (s *InterfaceSuite) TestRelationContextWithRemoteUnitName(c *tc.C) {
	ctx := s.GetContext(c, 1, "u/123", names.StorageTag{})
	name, err := ctx.RemoteUnitName()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "u/123")
}

func (s *InterfaceSuite) TestAvailabilityZone(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	zone, err := ctx.AvailabilityZone()
	c.Check(err, tc.ErrorIsNil)
	c.Check(zone, tc.Equals, "a-zone")
}

func (s *InterfaceSuite) TestUnitNetworkInfo(c *tc.C) {
	// Only the error case is tested to ensure end-to-end integration, the rest
	// of the cases are tested separately for network-get, api/uniter, and
	// apiserver/uniter, respectively.
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	netInfo, err := ctx.NetworkInfo([]string{"unknown"}, -1)
	c.Check(err, tc.ErrorIsNil)
	c.Check(netInfo, tc.DeepEquals, map[string]params.NetworkInfoResult{
		"unknown": {
			Error: &params.Error{
				Message: `undefined for unit charm: endpoint "unknown" not valid`,
				Code:    params.CodeNotValid,
			},
		},
	},
	)
}

func (s *InterfaceSuite) TestUnitStatus(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	defer context.PatchCachedStatus(ctx.(context.Context), "maintenance", "working", map[string]interface{}{"hello": "world"})()
	status, err := ctx.UnitStatus()
	c.Check(err, tc.ErrorIsNil)
	c.Check(status.Status, tc.Equals, "maintenance")
	c.Check(status.Info, tc.Equals, "working")
	c.Check(status.Data, tc.DeepEquals, map[string]interface{}{"hello": "world"})
}

func (s *InterfaceSuite) TestSetUnitStatus(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, tc.ErrorIsNil)
	unitStatus, err := ctx.UnitStatus()
	c.Check(err, tc.ErrorIsNil)
	c.Check(unitStatus.Status, tc.Equals, "maintenance")
	c.Check(unitStatus.Info, tc.Equals, "doing work")
	c.Check(unitStatus.Data, tc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestSetUnitStatusUpdatesFlag(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	c.Assert(ctx.(context.Context).HasExecutionSetUnitStatus(), tc.IsFalse)
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(ctx.(context.Context).HasExecutionSetUnitStatus(), tc.IsTrue)
}

func (s *InterfaceSuite) TestGetSetWorkloadVersion(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	// No workload version set yet.
	result, err := ctx.UnitWorkloadVersion()
	c.Assert(result, tc.Equals, "")
	c.Assert(err, tc.ErrorIsNil)

	err = ctx.SetUnitWorkloadVersion("Pipey")
	c.Assert(err, tc.ErrorIsNil)

	result, err = ctx.UnitWorkloadVersion()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, "Pipey")
}

func (s *InterfaceSuite) TestUnitStatusCaching(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	unitStatus, err := ctx.UnitStatus()
	c.Check(err, tc.ErrorIsNil)
	c.Check(unitStatus.Status, tc.Equals, "waiting")
	c.Check(unitStatus.Info, tc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, tc.DeepEquals, map[string]interface{}{})

	// Change remote state.
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "it works",
		Since:   &now,
	}
	err = s.unit.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Local view is unchanged.
	unitStatus, err = ctx.UnitStatus()
	c.Check(err, tc.ErrorIsNil)
	c.Check(unitStatus.Status, tc.Equals, "waiting")
	c.Check(unitStatus.Info, tc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, tc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestUnitCaching(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	pr, err := ctx.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pr, tc.Equals, "u-0.testing.invalid")
	pa, err := ctx.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	// Initially the public address is the same as the private address since
	// the "most public" address is chosen.
	c.Assert(pr, tc.Equals, pa)

	// Change remote state.
	err = s.machine.SetProviderAddresses(
		network.NewSpaceAddress("blah.testing.invalid", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Local view is unchanged.
	pr, err = ctx.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pr, tc.Equals, "u-0.testing.invalid")
	pa, err = ctx.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pr, tc.Equals, pa)
}

func (s *InterfaceSuite) TestConfigCaching(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	settings, err := ctx.ConfigSettings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "Something Else"})
	c.Assert(err, tc.ErrorIsNil)

	// Local view is not changed.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *InterfaceSuite) TestGoalState(c *tc.C) {
	timestamp := time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)
	mockUnitSince := func(inUnits application.UnitsGoalState) application.UnitsGoalState {
		outUnits := application.UnitsGoalState{}
		for name, gsStatus := range inUnits {
			c.Assert(gsStatus.Since, tc.NotNil)
			outUnits[name] = application.GoalStateStatus{
				Status: gsStatus.Status,
				Since:  &timestamp,
			}
		}
		return outUnits
	}
	goalStateCheck := application.GoalState{
		Units: application.UnitsGoalState{
			"u/0": application.GoalStateStatus{
				Status: "waiting",
				Since:  &timestamp,
			},
		},
		Relations: map[string]application.UnitsGoalState{
			"db": {
				"db0": application.GoalStateStatus{
					Status: "joining",
					Since:  &timestamp,
				},
				"db1": application.GoalStateStatus{
					Status: "joining",
					Since:  &timestamp,
				},
			},
		},
	}

	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	goalState, err := ctx.GoalState()

	// Mock status Since string
	goalState.Units = mockUnitSince(goalState.Units)
	for relationsNames, relationUnits := range goalState.Relations {
		goalState.Relations[relationsNames] = mockUnitSince(relationUnits)
	}

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(goalState, tc.DeepEquals, &goalStateCheck)
}

// TestNonActionCallsToActionMethodsFail does exactly what its name says:
// it simply makes sure that Action-related calls to HookContexts with a nil
// actionData member error out correctly.
func (s *InterfaceSuite) TestNonActionCallsToActionMethodsFail(c *tc.C) {
	ctx := context.HookContext{}
	_, err := ctx.ActionParams()
	c.Check(err, tc.ErrorMatches, "not running an action")
	err = ctx.SetActionFailed()
	c.Check(err, tc.ErrorMatches, "not running an action")
	err = ctx.SetActionMessage("foo")
	c.Check(err, tc.ErrorMatches, "not running an action")
	err = ctx.LogActionMessage("foo")
	c.Check(err, tc.ErrorMatches, "not running an action")
	err = ctx.UpdateActionResults([]string{"1", "2", "3"}, "value")
	c.Check(err, tc.ErrorMatches, "not running an action")
}

// TestUpdateActionResults demonstrates that UpdateActionResults functions
// as expected.
func (s *InterfaceSuite) TestUpdateActionResults(c *tc.C) {
	tests := []struct {
		initial  map[string]interface{}
		keys     []string
		value    string
		expected map[string]interface{}
	}{{
		initial: map[string]interface{}{},
		keys:    []string{"foo"},
		value:   "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}, {
		initial: map[string]interface{}{
			"foo": "bar",
		},
		keys:  []string{"foo", "bar"},
		value: "baz",
		expected: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
	}, {
		initial: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
		keys:  []string{"foo"},
		value: "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}}

	for i, t := range tests {
		c.Logf("UpdateActionResults test %d: %#v: %#v", i, t.keys, t.value)
		hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "", names.StorageTag{})
		context.WithActionContext(hctx, t.initial, nil)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, tc.ErrorIsNil)
		actionData, err := hctx.ActionData()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(actionData.ResultsMap, tc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *tc.C) {
	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "", names.StorageTag{})
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionFailed()
	c.Assert(err, tc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actionData.Failed, tc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *tc.C) {
	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "", names.StorageTag{})
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, tc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Check(err, tc.ErrorIsNil)
	c.Check(actionData.ResultsMessage, tc.Equals, "because reasons")
}

// TestLogActionMessage ensures LogActionMessage works properly.
func (s *InterfaceSuite) TestLogActionMessage(c *tc.C) {
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	action, err := s.Model.AddAction(s.unit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = action.Begin()
	c.Assert(err, tc.ErrorIsNil)

	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "", names.StorageTag{})
	context.WithActionContext(hctx, nil, nil)
	err = hctx.LogActionMessage("hello world")
	c.Assert(err, tc.ErrorIsNil)
	a, err := s.Model.Action(action.Id())
	c.Assert(err, tc.ErrorIsNil)
	messages := a.Messages()
	c.Assert(messages, tc.HasLen, 1)
	c.Assert(messages[0].Message(), tc.Equals, "hello world")
}

func (s *InterfaceSuite) TestRequestRebootAfterHook(c *tc.C) {
	var killed bool
	p := &mockProcess{func() error {
		killed = true
		return nil
	}}
	ctx := s.GetContext(c, -1, "", names.StorageTag{}).(*context.HookContext)
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(killed, tc.IsFalse)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, tc.Equals, jujuc.RebootAfterHook)
}

func (s *InterfaceSuite) TestRequestRebootNow(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{}).(*context.HookContext)

	var stub testhelpers.Stub
	var p *mockProcess
	p = &mockProcess{func() error {
		// Reboot priority should be set before the process
		// is killed, or else the client waiting for the
		// process to exit will race with the setting of
		// the priority.
		priority := ctx.GetRebootPriority()
		c.Assert(priority, tc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}}
	stub.SetErrors(errors.New("process is already dead"))
	ctx.SetProcess(p)

	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, tc.ErrorIsNil)

	// Everything went well, so priority should still be RebootNow.
	priority := ctx.GetRebootPriority()
	c.Assert(priority, tc.Equals, jujuc.RebootNow)
}

func (s *InterfaceSuite) TestRequestRebootNowTimeout(c *tc.C) {
	ctx := s.GetContext(c, -1, "", names.StorageTag{}).(*context.HookContext)

	var advanced bool
	var p *mockProcess
	p = &mockProcess{func() error {
		// Reboot priority should be set before the process
		// is killed, or else the client waiting for the
		// process to exit will race with the setting of
		// the priority.
		priority := ctx.GetRebootPriority()
		c.Assert(priority, tc.Equals, jujuc.RebootNow)
		if !advanced {
			advanced = true
			s.clock.Advance(time.Hour) // force timeout
		}
		return nil
	}}
	ctx.SetProcess(p)

	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, tc.ErrorMatches, "failed to kill context process 123")

	// RequestReboot failed, so priority should revert to RebootSkip.
	priority := ctx.GetRebootPriority()
	c.Assert(priority, tc.Equals, jujuc.RebootSkip)
}

func (s *InterfaceSuite) TestRequestRebootNowNoProcess(c *tc.C) {
	// A normal hook run or a juju-exec command will record the *os.Process
	// object of the running command, in HookContext. When requesting a
	// reboot with the --now flag, the process is killed and only
	// then will we set the reboot priority. This test basically simulates
	// the case when the process calling juju-reboot is not recorded.
	ctx := &context.HookContext{}
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, tc.ErrorMatches, "no process to kill")
	priority := ctx.GetRebootPriority()
	c.Assert(priority, tc.Equals, jujuc.RebootNow)
}

func (s *InterfaceSuite) TestStorageAddConstraints(c *tc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {
			params.StorageConstraints{},
		},
	}

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
	assertStorageAddInContext(c, ctx, expected)
}

var two = uint64(2)

func (s *InterfaceSuite) TestStorageAddConstraintsSameStorage(c *tc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {
			params.StorageConstraints{},
			params.StorageConstraints{Count: &two},
		},
	}

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
	addStorageToContext(ctx, "data", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func (s *InterfaceSuite) TestStorageAddConstraintsDifferentStorage(c *tc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {params.StorageConstraints{}},
		"diff": {
			params.StorageConstraints{Count: &two}},
	}

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
	addStorageToContext(ctx, "diff", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func addStorageToContext(ctx *context.HookContext,
	name string,
	cons params.StorageConstraints,
) {
	addOne := map[string]params.StorageConstraints{name: cons}
	_ = ctx.AddUnitStorage(addOne)
}

func assertStorageAddInContext(c *tc.C,
	ctx *context.HookContext, expected map[string][]params.StorageConstraints,
) {
	obtained := context.StorageAddConstraints(ctx)
	c.Assert(len(obtained), tc.Equals, len(expected))
	for k, v := range obtained {
		c.Assert(v, tc.SameContents, expected[k])
	}
}

func (s *InterfaceSuite) TestSecretMetadata(c *tc.C) {
	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	uri2 := coresecrets.NewURI()
	s.secretMetadata = map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "description",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Scope:  "relation-mariadb.db#gitlab.db",
					Role:   coresecrets.RoleView,
				},
			},
		},
		uri2.ID: {
			Owner:       names.NewApplicationTag("mariadb"),
			Description: "will be removed",
		},
	}
	ctx := s.GetContext(c, -1, "", names.StorageTag{})
	md, err := ctx.SecretMetadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.DeepEquals, map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "description",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Scope:  "relation-mariadb.db#gitlab.db",
					Role:   coresecrets.RoleView,
				},
			},
		},
		uri2.ID: {
			Owner:       names.NewApplicationTag("mariadb"),
			Description: "will be removed",
		},
	})
	uri3, err := ctx.CreateSecret(&jujuc.SecretCreateArgs{
		OwnerTag: names.NewApplicationTag("foo"),
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Description: ptr("a new one"),
			Value:       coresecrets.NewSecretValue(map[string]string{"foo": "bar"}),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = ctx.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		Description: ptr("another"),
	})
	c.Assert(err, tc.ErrorIsNil)
	ctx.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    ptr("gitlab/1"),
		RelationKey: ptr("mariadb:db gitlab:db"),
		Role:        ptr(coresecrets.RoleView),
	})

	err = ctx.RemoveSecret(uri2, nil)
	c.Assert(err, tc.ErrorIsNil)
	md, err = ctx.SecretMetadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md, tc.DeepEquals, map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "another",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{Target: "unit-gitlab-0", Scope: "relation-mariadb.db#gitlab.db", Role: "view"},
			},
		},
		uri3.ID: {
			Owner:          names.NewApplicationTag("foo"),
			Description:    "a new one",
			LatestRevision: 1,
			LatestChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	})
}

type mockProcess struct {
	kill func() error
}

func (p *mockProcess) Kill() error {
	return p.kill()
}

func (p *mockProcess) Pid() int {
	return 123
}

func TestMockHookContextSuite(t *tctesting.T) {
	tc.Run(t, &mockHookContextSuite{})
}

type mockHookContextSuite struct {
	testhelpers.IsolationSuite
	mockUnit       *mocks.MockHookUnit
	mockLeadership *mocks.MockLeadershipContext
	mockCache      params.UnitStateResult
}

func (s *mockHookContextSuite) TestDeleteCharmStateValue(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.DeleteCharmStateValue("one")
	c.Assert(err, tc.ErrorIsNil)

	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedCache, tc.DeepEquals, s.mockCache.CharmState)
}

func (s *mockHookContextSuite) TestDeleteCacheStateErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.DeleteCharmStateValue("five")
	c.Assert(err, tc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestGetCharmState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedCache, tc.DeepEquals, s.mockCache.CharmState)
}

func (s *mockHookContextSuite) TestGetCharmStateStateErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	_, err := hookContext.GetCharmState()
	c.Assert(err, tc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestGetCharmStateValue(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedVale, err := hookContext.GetCharmStateValue("one")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedVale, tc.Equals, "two")
}

func (s *mockHookContextSuite) TestGetCharmStateValueEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedVale, err := hookContext.GetCharmStateValue("seven")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedVale, tc.Equals, "")
}

func (s *mockHookContextSuite) TestGetCharmStateValueNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedCache, err := hookContext.GetCharmStateValue("five")
	c.Assert(err, tc.ErrorMatches, "\"five\" not found")
	c.Assert(obtainedCache, tc.Equals, "")
}

func (s *mockHookContextSuite) TestGetCharmStateValueStateErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	_, err := hookContext.GetCharmStateValue("key")
	c.Assert(err, tc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestSetCacheQuotaLimits(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	s.testSetCache(c)
}

func (s *mockHookContextSuite) TestSetCache(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	// Test key len limit
	err := hookContext.SetCharmStateValue(
		strings.Repeat("a", quota.MaxCharmStateKeySize+1),
		"lol",
	)
	c.Assert(err, tc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, ".*max allowed key.*")

	// Test value len limit
	err = hookContext.SetCharmStateValue(
		"lol",
		strings.Repeat("a", quota.MaxCharmStateValueSize+1),
	)
	c.Assert(err, tc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, ".*max allowed value.*")
}

func (s *mockHookContextSuite) TestSetCacheEmptyStartState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, nil)

	s.testSetCache(c)
}

func (s *mockHookContextSuite) testSetCache(c *tc.C) {
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, tc.ErrorIsNil)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, tc.ErrorIsNil)
	value, ok := obtainedCache["five"]
	c.Assert(ok, tc.IsTrue)
	c.Assert(value, tc.Equals, "six")
}

func (s *mockHookContextSuite) TestSetCacheStateErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, tc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestFlushWithNonDirtyCache(c *tc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	s.expectStateValues()

	// The following commands are no-ops as they don't mutate the cache.
	err := hookContext.SetCharmStateValue("one", "two") // no-op: KV already present
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.DeleteCharmStateValue("not-there") // no-op: key not present
	c.Assert(err, tc.ErrorIsNil)

	// Flush the context with a success. As the cache is not dirty we do
	// not expect a SetState call.
	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) TestSequentialFlushOfCacheValues(c *tc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	// We expect a single call for the following API endpoints
	s.expectStateValues()
	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				SetUnitState: &params.SetUnitStateArg{
					Tag: "unit-wordpress-0",
					CharmState: &map[string]string{
						"one":   "two",
						"three": "four",
						"lorem": "ipsum",
						"seven": "",
					},
				},
			},
		},
	}).Return(nil)

	// Mutate cache and flush; this should call out to SetState and reset
	// the dirty flag
	err := hookContext.SetCharmStateValue("lorem", "ipsum")
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Flush again; as the cache is not dirty, the SetState call is skipped.
	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) TestOpenPortRange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				OpenPorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	err := hookContext.OpenPortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) TestOpenedPortRanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				OpenPorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	err := hookContext.OpenPortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, tc.ErrorIsNil)

	// OpenedPortRanges() should return the pending requests, see
	// https://bugs.launchpad.net/juju/+bug/2008035
	openedPorts := hookContext.OpenedPortRanges()
	expectedOpenPorts := []network.PortRange{
		// Already present range from NewMockUnitHookContext()
		{
			FromPort: 666,
			ToPort:   888,
			Protocol: "tcp",
		},
		// Newly added but not yet flushed range
		{
			FromPort: 8080,
			ToPort:   8080,
			Protocol: "tcp",
		},
	}
	c.Assert(openedPorts.UniquePortRanges(), tc.DeepEquals, expectedOpenPorts)

	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)

	// After Flush() opened ports should remain the same.
	openedPorts = hookContext.OpenedPortRanges()
	c.Assert(openedPorts.UniquePortRanges(), tc.DeepEquals, expectedOpenPorts)
}

func (s *mockHookContextSuite) TestClosePortRange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				ClosePorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	err := hookContext.ClosePortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.Flush("success", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockUnit = mocks.NewMockHookUnit(ctrl)
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0")).AnyTimes()
	s.mockUnit.EXPECT().ApplicationName().Return("wordpress").AnyTimes()
	s.mockLeadership = mocks.NewMockLeadershipContext(ctrl)
	return ctrl
}

func (s *mockHookContextSuite) expectStateValues() {
	s.mockCache = params.UnitStateResult{
		CharmState: map[string]string{
			"one":   "two",
			"three": "four",
			"seven": "",
		}}
	s.mockUnit.EXPECT().State().Return(s.mockCache, nil)
}

func (s *mockHookContextSuite) TestActionAbort(c *tc.C) {
	tests := []struct {
		Status string
		Failed bool
		Cancel bool
	}{
		{Status: "aborted", Failed: true, Cancel: true},
		{Status: "failed", Failed: true, Cancel: false},
		{Status: "aborted", Failed: false, Cancel: true},
		{Status: "failed", Failed: false, Cancel: false},
	}
	for _, test := range tests {
		mocks := s.setupMocks(c)
		apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(objType, tc.Equals, "Uniter")
			c.Assert(version, tc.Equals, 0)
			c.Assert(id, tc.Equals, "")
			c.Assert(request, tc.Equals, "FinishActions")
			c.Assert(arg, tc.DeepEquals, params.ActionExecutionResults{
				Results: []params.ActionExecutionResult{{
					ActionTag: "action-2",
					Status:    test.Status,
					Message:   "failed yo",
				}}})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			return nil
		})
		st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
		hookContext := context.NewMockUnitHookContextWithState(s.mockUnit, st)
		cancel := make(chan struct{})
		if test.Cancel {
			close(cancel)
		}
		context.WithActionContext(hookContext, nil, cancel)
		if test.Failed {
			err := hookContext.SetActionFailed()
			c.Assert(err, tc.ErrorIsNil)
		}
		actionData, err := hookContext.ActionData()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(actionData.Failed, tc.Equals, test.Failed)
		err = hookContext.Flush("", errors.Errorf("failed yo"))
		c.Assert(err, tc.ErrorIsNil)
		mocks.Finish()
	}
}

func (s *mockHookContextSuite) TestActionFlushError(c *tc.C) {
	mocks := s.setupMocks(c)
	defer mocks.Finish()

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "FinishActions")
		c.Assert(arg, tc.DeepEquals, params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: "action-2",
				Status:    "failed",
				Message:   "committing requested changes failed",
				Results: map[string]interface{}{
					"stderr":      "flush failed",
					"return-code": "1",
				},
			}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{{
			Tag: "unit-wordpress-0",
			OpenPorts: []params.EntityPortRange{{
				Tag:      "unit-wordpress-0",
				Protocol: "tcp",
				FromPort: 666,
				ToPort:   666,
				Endpoint: "ep",
			}},
		}},
	}).Return(errors.New("flush failed"))

	st := uniter.NewState(apiCaller, names.NewUnitTag("wordpress/0"))
	hookContext := context.NewMockUnitHookContextWithState(s.mockUnit, st)
	context.SetEnvironmentHookContextSecret(hookContext, coresecrets.NewURI().String(), nil, nil, nil)

	err := hookContext.OpenPortRange("ep", network.PortRange{Protocol: "tcp", FromPort: 666, ToPort: 666})
	c.Assert(err, tc.ErrorIsNil)
	cancel := make(chan struct{})
	context.WithActionContext(hookContext, nil, cancel)
	err = hookContext.Flush("", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) TestMissingAction(c *tc.C) {
	defer s.setupMocks(c).Finish()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "FinishActions")
		c.Assert(arg, tc.DeepEquals, params.ActionExecutionResults{
			Results: []params.ActionExecutionResult{{
				ActionTag: "action-2",
				Status:    "failed",
				Message:   `action not implemented on unit "wordpress/0"`,
			}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	hookContext := context.NewMockUnitHookContextWithState(s.mockUnit, st)

	context.WithActionContext(hookContext, nil, nil)
	err := hookContext.Flush("action", charmrunner.NewMissingHookError("noaction"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mockHookContextSuite) assertSecretGetFromPendingChanges(c *tc.C,
	refresh, peek bool,
	setPendingSecretChanges func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string),
) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	label := "label"
	data := map[string]string{"foo": "bar"}
	if !refresh && !peek {
		data["foo"] = "existing"
	}
	setPendingSecretChanges(hookContext, uri, label, data)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, mockBackendClient{})

	value, err := hookContext.GetSecret(nil, label, refresh, peek)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, data)
}

func (s *mockHookContextSuite) TestSecretGetFromPendingChangesExisting(c *tc.C) {
	s.assertSecretGetFromPendingChanges(c, false, false,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			hc.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *mockHookContextSuite) TestSecretGetFromPendingCreateChanges(c *tc.C) {
	s.assertSecretGetFromPendingChanges(c, false, true,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			hc.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *mockHookContextSuite) TestAppSecretGetFromPendingCreateChanges(c *tc.C) {
	s.assertSecretGetFromPendingChanges(c, false, true,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := uniter.SecretCreateArg{OwnerTag: names.NewApplicationTag(s.mockUnit.ApplicationName())}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			hc.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *mockHookContextSuite) TestSecretGetFromPendingUpdateChanges(c *tc.C) {
	s.assertSecretGetFromPendingChanges(c, false, true,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := uniter.SecretUpdateArg{}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			hc.SetPendingSecretUpdates(
				map[string]uniter.SecretUpdateArg{uri.ID: arg})
		},
	)
}

type mockBackend struct {
	provider.SecretsBackend
}

func (mockBackend) GetContent(_ stdcontext.Context, revisionId string) (coresecrets.SecretValue, error) {
	if revisionId != "rev-id" {
		return nil, errors.NotFoundf("revision %q", revisionId)
	}
	return coresecrets.NewSecretValue(map[string]string{"foo": "bar"}), nil
}

type mockBackendClient struct {
	secrets.BackendsClient
}

func (mockBackendClient) GetContent(uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, error) {
	return coresecrets.NewSecretValue(map[string]string{"foo": "existing"}), nil
}

func (s *mockHookContextSuite) TestSecretGet(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(cfg.BackendConfig.BackendType, tc.Equals, "vault")
		return mockBackend{}, nil
	})

	uri := coresecrets.NewURI()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "SecretsManager")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetSecretContentInfo")
		c.Assert(arg, tc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-id",
					},
				},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: vault.BackendType,
					},
				},
			}},
		}
		return nil
	})

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	secretsBackend, err := secrets.NewClient(jujuSecretsAPI)
	c.Assert(err, tc.ErrorIsNil)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, jujuSecretsAPI, secretsBackend)

	value, err := hookContext.GetSecret(uri, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *mockHookContextSuite) assertSecretGetOwnedSecretURILookup(
	c *tc.C, patchContext func(*context.HookContext, *coresecrets.URI, string, context.SecretsAccessor, secrets.BackendsClient),
) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "SecretsManager")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetSecretContentInfo")
		c.Assert(arg, tc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Refresh: false,
				Peek:    false,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	secretsBackend, err := secrets.NewClient(jujuSecretsAPI)
	c.Assert(err, tc.ErrorIsNil)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, jujuSecretsAPI, secretsBackend)

	patchContext(hookContext, uri, "label", jujuSecretsAPI, secretsBackend)

	value, err := hookContext.GetSecret(nil, "label", false, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretURILookupFromAppliedCache(c *tc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client context.SecretsAccessor, backend secrets.BackendsClient) {
			context.SetEnvironmentHookContextSecret(
				ctx, uri.String(),
				map[string]jujuc.SecretMetadata{
					uri.ID: {Label: "label", Owner: s.mockUnit.Tag()},
				},
				client, backend)
		},
	)
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretURILookupFromPendingCreate(c *tc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client context.SecretsAccessor, backend secrets.BackendsClient) {
			arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			ctx.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingCreates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	hookContext.SetPendingSecretCreates(
		map[string]uniter.SecretCreateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, false, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretUpdatePendingCreateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	hookContext.SetPendingSecretCreates(
		map[string]uniter.SecretCreateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(uri, "foobar", false, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	arg.Label = ptr("foobar")
	c.Assert(hookContext.PendingSecretCreates(), tc.DeepEquals, map[string]uniter.SecretCreateArg{
		uri.ID: arg,
	})
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretURILookupFromPendingUpdate(c *tc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client context.SecretsAccessor, backend secrets.BackendsClient) {
			arg := uniter.SecretUpdateArg{}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
			arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
			ctx.SetPendingSecretUpdates(
				map[string]uniter.SecretUpdateArg{uri.ID: arg})
		},
	)
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingUpdatesPeek(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, false, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	c.Assert(hookContext.PendingSecretTrackLatest(), tc.HasLen, 0)
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingUpdatesRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, true, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	c.Assert(hookContext.PendingSecretTrackLatest(), tc.DeepEquals, map[string]bool{uri.ID: true})
}

func (s *mockHookContextSuite) TestSecretGetOwnedSecretUpdatePendingLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	arg.Checksum = "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(uri, "foobar", false, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(value.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	arg.Label = ptr("foobar")
	c.Assert(hookContext.PendingSecretUpdates(), tc.DeepEquals, map[string]uniter.SecretUpdateArg{
		uri.ID: arg,
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *mockHookContextSuite) TestSecretCreateApplicationOwner(c *tc.C) {
	s.assertSecretCreate(c, names.NewApplicationTag("mariadb"))
}

func (s *mockHookContextSuite) TestSecretCreateUnitOwner(c *tc.C) {
	s.assertSecretCreate(c, names.NewUnitTag("mariadb/0"))
}

func (s *mockHookContextSuite) assertSecretCreate(c *tc.C, owner names.Tag) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	expiry := time.Now()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "SecretsManager")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "CreateSecretURIs")
		c.Check(arg, tc.DeepEquals, params.CreateSecretURIsArg{
			Count: 1,
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		}
		return nil
	})
	if owner.Kind() == names.ApplicationTagKind {
		s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	}

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	context.SetEnvironmentHookContextSecret(hookContext, "", nil, jujuSecretsAPI, nil)

	uri, err := hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value:        value,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			ExpireTime:   ptr(expiry),
			Description:  ptr("my secret"),
			Label:        ptr("foo"),
		},
		OwnerTag: owner,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uri.String(), tc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(hookContext.PendingSecretCreates(), tc.DeepEquals, map[string]uniter.SecretCreateArg{
		uri.ID: {
			SecretUpsertArg: uniter.SecretUpsertArg{
				URI:          uri,
				Value:        value,
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(expiry),
				Description:  ptr("my secret"),
				Label:        ptr("foo"),
				Checksum:     "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
			},
			OwnerTag: owner,
		}})
}

func (s *mockHookContextSuite) TestSecretCreateDupLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "SecretsManager")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "CreateSecretURIs")
		c.Check(arg, tc.DeepEquals, params.CreateSecretURIsArg{
			Count: 1,
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		}
		return nil
	})
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil).Times(2)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	context.SetEnvironmentHookContextSecret(hookContext, "", nil, jujuSecretsAPI, nil)

	_, err := hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: value,
			Label: ptr("foo"),
		},
		OwnerTag: names.NewApplicationTag("myapp"),
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: value,
			Label: ptr("foo"),
		},
		OwnerTag: names.NewApplicationTag("myapp"),
	})
	c.Assert(err, tc.ErrorMatches, `secret with label "foo" already exists`)
}

func (s *mockHookContextSuite) TestSecretUpdate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil).Times(2)
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			LatestChecksum: "deadbeef",
			Owner:          names.NewApplicationTag("mariadb"),
		},
	}, nil, nil)

	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	err := hookContext.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		Value:        value,                        // will be overwritten by the new value.
		RotatePolicy: ptr(coresecrets.RotateDaily), // will be kept.
		Description:  ptr("my secret"),             // will be overwritten by the new value.
		Label:        ptr("label1"),                // will be overwritten by the new value.
	})
	c.Assert(err, tc.ErrorIsNil)

	// update again, merge with existing.
	newData := map[string]string{"bar": "baz"}
	newValue := coresecrets.NewSecretValue(newData)
	expiry := time.Now()
	err = hookContext.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		ExpireTime:  ptr(expiry),          // will be merged.
		Value:       newValue,             // will be the new value.
		Description: ptr("my new secret"), // will be the new value.
		Label:       ptr("label2"),        // will be the new value.
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretUpdates(), tc.DeepEquals, map[string]uniter.SecretUpdateArg{
		uri.ID: {
			CurrentRevision: 666,
			SecretUpsertArg: uniter.SecretUpsertArg{
				URI:          uri,
				Value:        newValue,
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(expiry),
				Description:  ptr("my new secret"),
				Label:        ptr("label2"),
				Checksum:     "b3aa50894a7e14268a5ab22be352ece5e937f2f2037367e1d7b43a6574969493",
			},
		}})
}

func (s *mockHookContextSuite) TestSecretUpdateSameContent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	expiry := time.Now()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			LatestChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
			Owner:          names.NewApplicationTag("mariadb"),
		},
	}, nil, nil)
	err := hookContext.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		Value:        value,
		RotatePolicy: ptr(coresecrets.RotateDaily),
		ExpireTime:   ptr(expiry),
		Description:  ptr("my secret"),
		Label:        ptr("foo"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretUpdates(), tc.DeepEquals, map[string]uniter.SecretUpdateArg{
		uri.ID: {
			CurrentRevision: 666,
			SecretUpsertArg: uniter.SecretUpsertArg{
				URI:          uri,
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(expiry),
				Description:  ptr("my secret"),
				Label:        ptr("foo"),
			},
		}})
}

func (s *mockHookContextSuite) TestSecretRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)
	err := hookContext.RemoveSecret(uri, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.RemoveSecret(uri2, ptr(666))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRemoves(), tc.DeepEquals, map[string]uniter.SecretDeleteArg{
		uri.ID:  {URI: uri},
		uri2.ID: {URI: uri2, Revision: ptr(666)}})
}

func (s *mockHookContextSuite) TestSecretGrant(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)

	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.GrantSecret(uri2, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{
		uri.ID: {
			relationKey: {
				URI:             uri,
				ApplicationName: &app,
				RelationKey:     &relationKey,
				Role:            coresecrets.RoleView,
			},
		},
		uri2.ID: {
			relationKey: {
				URI:             uri2,
				ApplicationName: &app,
				RelationKey:     &relationKey,
				Role:            coresecrets.RoleView,
			},
		}})
}

func (s *mockHookContextSuite) TestSecretGrantSecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(errors.Is(err, errors.NotFound), tc.IsTrue)
}

func (s *mockHookContextSuite) TestSecretGrantNotLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(false, nil)

	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(errors.Is(err, context.ErrIsNotLeader), tc.IsTrue)
}

func (s *mockHookContextSuite) TestSecretGrantNoOPSBecauseofExactSameApp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "application-gitlab",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	app := "gitlab"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
		Role:            ptr(coresecrets.RoleView),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *mockHookContextSuite) TestSecretGrantNoOPSBecauseofExactSameUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	unit := "gitlab/0"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit,
		RelationKey: &relationKey,
		Role:        ptr(coresecrets.RoleView),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *mockHookContextSuite) TestSecretGrantNoOPSBecauseApplicationLevelGrantedAlready(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "application-gitlab",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	unit := "gitlab/0"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit,
		RelationKey: &relationKey,
		Role:        ptr(coresecrets.RoleView),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *mockHookContextSuite) TestSecretGrantFailedRevokeExistingRecordRequired(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), tc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	app := "gitlab"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
		Role:            ptr(coresecrets.RoleView),
	})
	c.Assert(err, tc.ErrorMatches, `any unit level grants need to be revoked before granting access to the corresponding application`)
}

func (s *mockHookContextSuite) TestSecretRevoke(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil).AnyTimes()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)
	app := "mariadb"
	unit0 := "mariadb/0"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = hookContext.RevokeSecret(uri2, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), tc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

	// No OPS for duplicated revoke.
	err = hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), tc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

	// No OPS for unit level revoke because application level revoke exists already.
	err = hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit0,
		RelationKey: &relationKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), tc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

}

func (s *mockHookContextSuite) TestHookStorage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().StorageAttachment(names.NewStorageTag("data/0"), names.NewUnitTag("wordpress/0")).Return(params.StorageAttachment{
		StorageTag: "data/0",
	}, nil)
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0")).AnyTimes()
	ctx := context.NewMockUnitHookContextWithStateAndStorage("wordpress/0", s.mockUnit, st, names.NewStorageTag("data/0"))

	storage, err := ctx.HookStorage()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storage, tc.NotNil)
	c.Assert(storage.Tag().Id(), tc.Equals, "data/0")
}

func (s *mockHookContextSuite) TestHookCloudSpecMachine(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().CloudSpec().Return(&params.CloudSpec{
		Type: "lxd",
		Credential: &params.CloudCredential{
			AuthType: "authtype",
		},
	}, nil)

	ctx := context.NewMockUnitHookContextWithStateAndModelType(s.mockUnit, st, model.IAAS)

	spec, err := ctx.CloudSpec()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.Type, tc.Equals, "lxd")
	c.Assert(spec.Credential.AuthType, tc.Equals, "authtype")
}

func (s *mockHookContextSuite) TestHookCloudSpecK8s(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().CloudSpec().Return(&params.CloudSpec{
		Type:   "kubernetes",
		Name:   "myk8s",
		Region: "myregion",
		Credential: &params.CloudCredential{
			AuthType: "overridden",
		},
		IsControllerCloud: true,
	}, nil)

	tempDir := c.MkDir()
	caFile := path.Join(tempDir, "ca.crt")
	err := os.WriteFile(caFile, []byte("start\ncadata\nend\n"), 0600)
	c.Assert(err, tc.ErrorIsNil)

	ctx := context.NewMockUnitHookContextWithStateAndModelType(s.mockUnit, st, model.CAAS)
	ctx.SetInClusterConfig(func() (*rest.Config, error) {
		return &rest.Config{
			Host:        "https://localhost:1234",
			BearerToken: "bearertoken",
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: caFile,
			},
		}, nil
	})

	spec, err := ctx.CloudSpec()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec, tc.DeepEquals, &params.CloudSpec{
		Type:     "kubernetes",
		Name:     "myk8s",
		Region:   "myregion",
		Endpoint: "https://localhost:1234",
		Credential: &params.CloudCredential{
			AuthType:   "oauth2",
			Attributes: map[string]string{"Token": "bearertoken"},
		},
		CACertificates:    []string{"start\ncadata\nend\n"},
		IsControllerCloud: true,
	})
}
