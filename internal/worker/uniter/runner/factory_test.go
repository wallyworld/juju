// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api/agent/uniter"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/state"
)

type FactorySuite struct {
	ContextSuite
}

func TestFactorySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &FactorySuite{})
}

func (s *FactorySuite) AssertPaths(c *tc.C, rnr runner.Runner) {
	c.Assert(runner.RunnerPaths(rnr), tc.DeepEquals, s.paths)
}

func (s *FactorySuite) TestNewCommandRunnerNoRelation(c *tc.C) {
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: -1})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerRelationIdDoesNotExist(c *tc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: 12, ForceRemoteUnit: value,
		})
		c.Check(err, tc.ErrorMatches, `unknown relation id: 12`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInvalid(c *tc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: 0, RemoteUnitName: "blah", ForceRemoteUnit: value,
		})
		c.Check(err, tc.ErrorMatches, `invalid remote unit: blah`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInappropriate(c *tc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: -1, RemoteUnitName: "blah/123", ForceRemoteUnit: value,
		})
		c.Check(err, tc.ErrorMatches, `remote unit provided without a relation: blah/123`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerEmptyRelation(c *tc.C) {
	_, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 1})
	c.Check(err, tc.ErrorMatches, `cannot infer remote unit in empty relation 1`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitAmbiguous(c *tc.C) {
	s.membership[1] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 1})
	c.Check(err, tc.ErrorMatches, `ambiguous remote unit; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitMissing(c *tc.C) {
	s.membership[0] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123",
	})
	c.Check(err, tc.ErrorMatches, `unknown remote unit blah/123; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerForceNoRemoteUnit(c *tc.C) {
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerForceRemoteUnitMissing(c *tc.C) {
	_, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, tc.IsNil)
}

func (s *FactorySuite) TestNewCommandRunnerInferRemoteUnit(c *tc.C) {
	s.membership[0] = []string{"foo/2"}
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 0})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunner(c *tc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadHook(c *tc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{})
	c.Assert(rnr, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `unknown hook kind ""`)
}

func (s *FactorySuite) TestNewHookRunnerWithStorage(c *tc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, sCons)
	s.machine = nil // allocate a new machine
	unit := s.AddUnit(c, application)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)
	storageTag := storageAttachments[0].StorageInstance()

	volume, err := sb.StorageInstanceVolume(storageTag)
	c.Assert(err, tc.ErrorIsNil)
	volumeTag := volume.VolumeTag()
	machineTag := s.machine.MachineTag()

	err = sb.SetVolumeInfo(
		volumeTag, state.VolumeInfo{
			VolumeId: "vol-123",
			Size:     456,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "sdb",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := uniter.NewFromConnection(st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.uniter, tc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            uniter,
		Unit:             apiUnit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, tc.ErrorIsNil)
	factory, err := runner.NewFactory(
		s.paths,
		contextFactory,
		runner.NewRunner,
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	rnr, err := factory.NewHookRunner(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	c.Assert(ctx, tc.NotNil)
	c.Assert(ctx.UnitName(), tc.Equals, "storage-block/0")
}

func (s *FactorySuite) TestNewHookRunnerWithRelation(c *tc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadRelation(c *tc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 12345,
	})
	c.Assert(rnr, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `unknown relation id: 12345`)
}

func (s *FactorySuite) TestNewActionRunnerGood(c *tc.C) {
	s.SetCharm(c, "dummy")
	for i, test := range []struct {
		actionName string
		payload    map[string]interface{}
	}{
		{
			actionName: "snapshot",
			payload: map[string]interface{}{
				"outfile": "/some/file.bz2",
			},
		},
		{
			// juju-exec should work as a predefined action even if
			// it's not part of the charm
			actionName: "juju-exec",
			payload: map[string]interface{}{
				"command": "foo",
				"timeout": 0.0,
			},
		},
	} {
		c.Logf("test %d", i)
		operationID, err := s.model.EnqueueOperation("a test", 1)
		c.Assert(err, tc.ErrorIsNil)
		action, err := s.model.EnqueueAction(operationID, s.unit.Tag(), test.actionName, test.payload, true, "group", nil)
		c.Assert(err, tc.ErrorIsNil)
		uniterAction := uniter.NewAction(
			action.Id(),
			action.Name(),
			action.Parameters(),
			action.Parallel(),
			action.ExecutionGroup(),
		)
		rnr, err := s.factory.NewActionRunner(uniterAction, nil)
		c.Assert(err, tc.ErrorIsNil)
		s.AssertPaths(c, rnr)
		ctx := rnr.Context()
		data, err := ctx.ActionData()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(data, tc.DeepEquals, &context.ActionData{
			Name:       test.actionName,
			Tag:        action.ActionTag(),
			Params:     test.payload,
			ResultsMap: map[string]interface{}{},
		})
		vars, err := ctx.HookVars(s.paths, false, context.NewRemoteEnvironmenter(
			func() []string { return []string{} },
			func(k string) string {
				switch k {
				case "PATH", "Path":
					return "pathy"
				}
				return ""
			},
			func(k string) (string, bool) {
				switch k {
				case "PATH", "Path":
					return "pathy", true
				}
				return "", false
			},
		))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(len(vars) > 0, tc.IsTrue, tc.Commentf("expected HookVars but found none"))
		combined := strings.Join(vars, "|")
		c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_NAME=`+test.actionName+`(\|.*|$)`)
		c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_UUID=`+action.Id()+`(\|.*|$)`)
		c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_TAG=`+action.Tag().String()+`(\|.*|$)`)
	}
}

func (s *FactorySuite) TestNewActionRunnerBadName(c *tc.C) {
	s.SetCharm(c, "dummy")
	action := uniter.NewAction("666", "no-such-action", nil, false, "")
	rnr, err := s.factory.NewActionRunner(action, nil)
	c.Check(rnr, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "cannot run \"no-such-action\" action: not defined")
	c.Check(err, tc.Satisfies, charmrunner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerBadParams(c *tc.C) {
	s.SetCharm(c, "dummy")
	action := uniter.NewAction("666", "snapshot", map[string]interface{}{
		"outfile": 123,
	}, true, "group")
	rnr, err := s.factory.NewActionRunner(action, nil)
	c.Check(rnr, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "cannot run \"snapshot\" action: .*")
	c.Check(err, tc.Satisfies, charmrunner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerWithCancel(c *tc.C) {
	s.SetCharm(c, "dummy")
	actionName := "snapshot"
	payload := map[string]interface{}{
		"outfile": "/some/file.bz2",
	}
	cancel := make(chan struct{})
	operationID, err := s.model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	action, err := s.model.EnqueueAction(operationID, s.unit.Tag(), actionName, payload, true, "group", nil)
	c.Assert(err, tc.ErrorIsNil)
	uniterAction := uniter.NewAction(
		action.Id(),
		action.Name(),
		action.Parameters(),
		action.Parallel(),
		action.ExecutionGroup(),
	)
	rnr, err := s.factory.NewActionRunner(uniterAction, cancel)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	data, err := ctx.ActionData()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, &context.ActionData{
		Name:       actionName,
		Tag:        action.ActionTag(),
		Params:     payload,
		ResultsMap: map[string]interface{}{},
		Cancel:     cancel,
	})
	vars, err := ctx.HookVars(s.paths, false, context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(k string) string {
			switch k {
			case "PATH", "Path":
				return "pathy"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "PATH", "Path":
				return "pathy", true
			}
			return "", false
		},
	))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(vars) > 0, tc.IsTrue, tc.Commentf("expected HookVars but found none"))
	combined := strings.Join(vars, "|")
	c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_NAME=`+actionName+`(\|.*|$)`)
	c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_UUID=`+action.Id()+`(\|.*|$)`)
	c.Assert(combined, tc.Matches, `(^|.*\|)JUJU_ACTION_TAG=`+action.Tag().String()+`(\|.*|$)`)
}
