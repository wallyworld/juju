// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"encoding/hex"
	"os"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/fs"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	k8stesting "github.com/juju/juju/caas/kubernetes/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/feature"
	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
)

type ContextFactorySuite struct {
	HookContextSuite
	paths      runnertesting.RealPaths
	factory    context.ContextFactory
	membership map[int][]string
}

func TestContextFactorySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ContextFactorySuite{})
}

func (s *ContextFactorySuite) SetUpTest(c *tc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.Features: []string{feature.RawK8sSpec},
	}

	s.HookContextSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
	s.membership = map[int][]string{
		0: {"r/0"},
		1: {"r/1"},
	}

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            s.uniter,
		Unit:             s.apiUnit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, tc.ErrorIsNil)
	s.factory = contextFactory
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
}

func (s *ContextFactorySuite) setUpCacheMethods(c *tc.C) {
	// The factory's caches are created lazily, so it doesn't have any at all to
	// begin with. Creating and discarding a context lets us call updateCache
	// without panicking. (IMO this is less invasive that making updateCache
	// responsible for creating missing caches etc.)
	_, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ContextFactorySuite) Model(c *tc.C) *state.Model {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	return m
}

func (s *ContextFactorySuite) updateCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) updateAppCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedAppSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) getCache(relId int, unitName string) (params.Settings, bool) {
	return context.CachedSettings(s.factory, relId, unitName)
}

func (s *ContextFactorySuite) getAppCache(relId int, appName string) (params.Settings, bool) {
	return context.CachedAppSettings(s.factory, relId, appName)
}

func (s *ContextFactorySuite) SetCharm(c *tc.C, name string) {
	err := os.RemoveAll(s.paths.GetCharmDir())
	c.Assert(err, tc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.GetCharmDir())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ContextFactorySuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &context.RelationInfo{
			RelationUnit: &relUnitShim{relUnit},
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) testLeadershipContextWiring(c *tc.C, createContext func() *context.HookContext) {
	var stub testhelpers.Stub
	stub.SetErrors(errors.New("bam"))
	restore := context.PatchNewLeadershipContext(
		func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext {
			stub.AddCall("NewLeadershipContext", accessor, tracker, unitName)
			return &StubLeadershipContext{Stub: &stub}
		},
	)
	defer restore()

	ctx := createContext()
	isLeader, err := ctx.IsLeader()
	c.Check(err, tc.ErrorMatches, "bam")
	c.Check(isLeader, tc.IsFalse)

	stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "NewLeadershipContext",
		Args:     []interface{}{s.uniter.LeadershipSettings, &runnertesting.FakeTracker{}, "u/0"},
	}, {
		FuncName: "IsLeader",
	}})

}

func (s *ContextFactorySuite) TestNewHookContextRetrievesSLALevel(c *tc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, tc.ErrorIsNil)

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx.SLALevel(), tc.Equals, "essential")
}

func (s *ContextFactorySuite) TestNewHookContextLeadershipContext(c *tc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, tc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewCommandContextLeadershipContext(c *tc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
		c.Assert(err, tc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewActionContextLeadershipContext(c *tc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		s.SetCharm(c, "dummy")
		operationID, err := s.Model(c).EnqueueOperation("a test", 1)
		c.Assert(err, tc.ErrorIsNil)
		action, err := s.Model(c).EnqueueAction(operationID, s.unit.Tag(), "snapshot", nil, true, "group", nil)
		c.Assert(err, tc.ErrorIsNil)

		actionData := &context.ActionData{
			Name:       action.Name(),
			Tag:        names.NewActionTag(action.Id()),
			Params:     action.Parameters(),
			ResultsMap: map[string]interface{}{},
		}

		ctx, err := s.factory.ActionContext(actionData)
		c.Assert(err, tc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestHookContextID(c *tc.C) {
	hi := hook.Info{
		Kind: hooks.Install,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, tc.ErrorIsNil)

	v := strings.Split(ctx.Id(), "-")
	c.Assert(v, tc.HasLen, 3)

	randomComponent, err := hex.DecodeString(v[2])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(randomComponent, tc.HasLen, 16)
}

func (s *ContextFactorySuite) TestRelationHookContext(c *tc.C) {
	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestRelationBrokenHookContext(c *tc.C) {
	delete(s.membership, 1)
	rel, err := s.State.Relation(1)
	c.Assert(err, tc.ErrorIsNil)
	err = rel.SetSuspended(true, "")
	c.Assert(err, tc.ErrorIsNil)
	err = s.apiRelunits[1].Relation().Refresh()
	c.Assert(err, tc.ErrorIsNil)
	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(context.RelationBroken(ctx, 0), tc.IsFalse)
	c.Assert(context.RelationBroken(ctx, 1), tc.IsTrue)
}

func (s *ContextFactorySuite) TestRelationIsPeerHookContext(c *tc.C) {
	relCh := s.AddTestingCharm(c, "riak")
	app := s.AddTestingApplication(c, "riak", relCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = u.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	st := s.OpenAPIAs(c, u.Tag(), password)
	uniterAPI, err := uniter.NewFromConnection(st)
	c.Assert(err, tc.ErrorIsNil)

	rels, err := app.Relations()
	c.Assert(err, tc.ErrorIsNil)
	var rel *state.Relation
	for _, r := range rels {
		if len(r.Endpoints()) == 1 {
			rel = r
			break
		}
	}
	c.Assert(rel, tc.NotNil)
	ru, err := rel.Unit(u)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"relation-name": "riak"})
	c.Assert(err, tc.ErrorIsNil)
	apiRel, err := uniterAPI.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, tc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(u.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	s.apiRelunits[rel.Id()] = apiRelUnit

	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: rel.Id(),
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(context.RelationBroken(ctx, rel.Id()), tc.IsFalse)
}

// TestWorkloadHookContext verifies that each of the types of workload hook
// generate the correct event context.
func (s *ContextFactorySuite) TestWorkloadHookContext(c *tc.C) {
	infos := []hook.Info{
		{
			Kind:         hooks.PebbleReady,
			WorkloadName: "test",
		},
		{
			Kind:         hooks.PebbleCustomNotice,
			WorkloadName: "test",
			NoticeID:     "123",
			NoticeType:   "custom",
			NoticeKey:    "example.com/bar",
		},
		{
			Kind:         hooks.PebbleCheckFailed,
			WorkloadName: "test",
			CheckName:    "http-check",
		},
		{
			Kind:         hooks.PebbleCheckRecovered,
			WorkloadName: "test",
			CheckName:    "http-check",
		},
	}
	for _, hi := range infos {
		ctx, err := s.factory.HookContext(hi)
		c.Assert(err, tc.ErrorIsNil)
		s.AssertCoreContext(c, ctx)
		s.AssertWorkloadContext(c, ctx, "test")
		s.AssertNotActionContext(c, ctx)
		s.AssertNotRelationContext(c, ctx)
		s.AssertNotStorageContext(c, ctx)
		s.AssertNotSecretContext(c, ctx)
		switch hi.Kind {
		case hooks.PebbleCustomNotice:
			actualNoticeKey, _ := ctx.WorkloadNoticeKey()
			c.Assert(actualNoticeKey, tc.Equals, "example.com/bar")
			actualNoticeType, _ := ctx.WorkloadNoticeType()
			c.Assert(actualNoticeType, tc.Equals, "custom")
		case hooks.PebbleCheckFailed, hooks.PebbleCheckRecovered:
			actualCheckName, _ := ctx.WorkloadCheckName()
			c.Assert(actualCheckName, tc.Equals, "http-check")
		}
	}
}

func (s *ContextFactorySuite) TestNewHookContextWithStorage(c *tc.C) {
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

	err = sb.CreateVolumeAttachmentPlan(machineTag, volumeTag, state.VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceTypeLocal,
		DeviceAttributes: nil,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = sb.SetVolumeAttachmentPlanBlockInfo(machineTag, volumeTag, state.BlockDeviceInfo{
		DeviceName: "sdb",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sdb",
	})
	c.Assert(err, tc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := uniter.NewFromConnection(st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uniter, tc.NotNil)
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
	ctx, err := contextFactory.HookContext(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx.UnitName(), tc.Equals, "storage-block/0")
	c.Assert(ctx.ModelType(), tc.Equals, model.IAAS)
	s.AssertStorageContext(c, ctx, "data/0", storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestSecretHookContext(c *tc.C) {
	hi := hook.Info{
		// Kind can be any secret hook kind.
		// Whatever attributes are set below will
		// be added to the context.
		Kind:           hooks.SecretExpired,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretLabel:    "label",
		SecretRevision: 666,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertSecretContext(c, ctx, hi.SecretURI, hi.SecretLabel, hi.SecretRevision)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

var podSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
    ports:
    - containerPort: 80
      protocol: TCP
    - containerPort: 443
    config:
      attr: foo=bar; fred=blogs
      foo: bar
`[1:]

func (s *ContextFactorySuite) setupPodSpec(c *tc.C) (*state.State, context.ContextFactory, string) {
	st := s.Factory.MakeCAASModel(c, nil)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// We are using the lease.Manager from the apiserver and not st.LeadershipClaimer
	// so unfortunately, we need to hack the acquisition of a lease because of the
	// way this test is set up.
	claimer, err := s.LeaseManager.Claimer("application-leadership", st.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	err = claimer.Claim(app.Tag().Id(), unit.Tag().Id(), time.Minute)
	c.Assert(err, tc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	apiInfo, err := environs.APIInfo(
		environscontext.NewEmptyCloudCallContext(),
		s.ControllerConfig.ControllerUUID(), st.ModelUUID(), coretesting.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, tc.ErrorIsNil)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	uniter, err := uniter.NewFromConnection(apiState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uniter, tc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State: uniter,
		Unit:  apiUnit,
		Tracker: &runnertesting.FakeTracker{
			AllowClaimLeader: true,
		},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, tc.ErrorIsNil)
	return st, contextFactory, unit.ApplicationName()
}

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetPodSpec(c *tc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, tc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, tc.ErrorIsNil)

	err = ctx.SetPodSpec(podSpec)
	c.Assert(err, tc.ErrorIsNil)

	_, err = cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, tc.ErrorIsNil)

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ps, tc.Equals, podSpec)

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rps, tc.Equals, "")
}

var rawK8sSpec = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`[1:]

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetRawK8sSpec(c *tc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, tc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, tc.ErrorIsNil)

	err = ctx.SetRawK8sSpec(rawK8sSpec)
	c.Assert(err, tc.ErrorIsNil)

	_, err = cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, tc.ErrorIsNil)

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rps, tc.Equals, rawK8sSpec)

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ps, tc.Equals, "")
}

func (s *ContextFactorySuite) TestHookContextCAASDeferredSetPodSpecSetRawK8sSpecNotAllowed(c *tc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()
	appTag := names.NewApplicationTag(appName)

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, tc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, tc.ErrorIsNil)

	err = ctx.SetPodSpec(podSpec)
	c.Assert(err, tc.ErrorIsNil)
	_, err = cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.SetRawK8sSpec(rawK8sSpec)
	c.Assert(err, tc.ErrorIsNil)
	_, err = cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorMatches, "k8s spec for application gitlab not found")

	err = ctx.Flush("", nil)
	c.Assert(err, tc.ErrorMatches, `either k8s-spec-set or k8s-raw-set can be run for each application, but not both`)
}

func (s *ContextFactorySuite) TestHookContextCAASNilPodSpecNilRawPodSpecButUpgradeCharmHookRan(c *tc.C) {
	st, cf, appName := s.setupPodSpec(c)
	defer st.Close()

	ctx, err := cf.HookContext(hook.Info{
		Kind: hooks.UpgradeCharm,
	})
	c.Assert(err, tc.ErrorIsNil)

	sm, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cm, err := sm.CAASModel()
	c.Assert(err, tc.ErrorIsNil)

	appTag := names.NewApplicationTag(appName)
	w, err := cm.WatchPodSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange() // initial event.

	// No change for non upgrade-hook.
	err = ctx.Flush("", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = ctx.Flush("upgrade-charm", nil)
	c.Assert(err, tc.ErrorIsNil)
	// both k8s spec and raw k8s spec are nil, but "upgrade-charm" hook will trigger a change to update "upgrade-counter".
	wc.AssertOneChange()

	ps, err := cm.PodSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ps, tc.Equals, "")

	rps, err := cm.RawK8sSpec(appTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rps, tc.Equals, "")

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *ContextFactorySuite) TestNewHookContextCAASModel(c *tc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	apiInfo, err := environs.APIInfo(
		environscontext.NewEmptyCloudCallContext(),
		s.ControllerConfig.ControllerUUID(), st.ModelUUID(), coretesting.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, tc.ErrorIsNil)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	uniter, err := uniter.NewFromConnection(apiState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uniter, tc.NotNil)
	apiUnit, err := uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State: uniter,
		Unit:  apiUnit,
		Tracker: &runnertesting.FakeTracker{
			AllowClaimLeader: true,
		},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := contextFactory.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx.UnitName(), tc.Equals, unit.Name())
	c.Assert(ctx.ModelType(), tc.Equals, model.CAAS)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestActionContext(c *tc.C) {
	s.SetCharm(c, "dummy")
	operationID, err := s.Model(c).EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	action, err := s.Model(c).EnqueueAction(operationID, s.unit.Tag(), "snapshot", nil, true, "group", nil)
	c.Assert(err, tc.ErrorIsNil)

	actionData := &context.ActionData{
		Name:       action.Name(),
		Tag:        names.NewActionTag(action.Id()),
		Params:     action.Parameters(),
		ResultsMap: map[string]interface{}{},
	}

	ctx, err := s.factory.ActionContext(actionData)
	c.Assert(err, tc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContext(c *tc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, tc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContextNoRelation(c *tc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceNoRemoteUnit(c *tc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceRemoteUnitMissing(c *tc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		// TODO(jam): 2019-10-23 Add RemoteApplicationName
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, tc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextInferRemoteUnit(c *tc.C) {
	// TODO(jam): 2019-10-23 Add RemoteApplicationName
	s.membership[0] = []string{"foo/2"}
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: 0})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextPrunesNonMemberCaches(c *tc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.Settings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.Settings{"drop": "me"})

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, tc.ErrorIsNil)

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, tc.IsTrue)
	c.Assert(settings0, tc.DeepEquals, params.Settings{"keep": "me"})

	settings1, found := s.getCache(0, "rel0/1")
	c.Assert(found, tc.IsFalse)
	c.Assert(settings1, tc.IsNil)

	// Check the caches are being used by the context relations.
	relCtx, err := ctx.Relation(0)
	c.Assert(err, tc.ErrorIsNil)

	// Verify that the settings really were cached by trying to look them up.
	// Nothing's really in scope, so the call would fail if they weren't.
	settings0, err = relCtx.ReadSettings("rel0/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings0, tc.DeepEquals, params.Settings{"keep": "me"})

	// Verify that the non-member settings were purged by looking them up and
	// checking for the expected error.
	settings1, err = relCtx.ReadSettings("rel0/1")
	c.Assert(settings1, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *ContextFactorySuite) TestNewHookContextRelationJoinedUpdatesRelationContextAndCaches(c *tc.C) {
	// Write some cached settings for r/0, so we can verify the cache gets cleared.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        1,
		RemoteUnit:        "r/0",
		RemoteApplication: "r",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "r")
	c.Assert(rel.UnitNames(), tc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, tc.IsNil)
	c.Assert(member, tc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCaches(c *tc.C) {
	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})
	s.updateAppCache(1, "r", params.Settings{"frob": "nizzle"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteUnit:        "r/4",
		RemoteApplication: "r",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/4", "r")
	c.Assert(rel.UnitNames(), tc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, tc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, tc.IsNil)
	c.Assert(member, tc.IsTrue)
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, tc.IsNil)
	c.Assert(member, tc.IsFalse)
	cachedApp, found := s.getAppCache(1, "r")
	// TODO(jam): 2019-10-23 This is currently wrong. We are currently pruning
	//  all application settings on every hook invocation. We should only
	//  invalidate it when we run a relation-changed hook for the app
	c.Assert(cachedApp, tc.Not(tc.DeepEquals), params.Settings{"frob": "bar"}, tc.Commentf("application settings should be properly cached"))
	c.Assert(found, tc.IsFalse)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCachesApplication(c *tc.C) {
	// Set values for r/0 and r make sure we don't see r/0 change but we *do* see r wiped.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateAppCache(1, "r", params.Settings{"baz": "quux"})
	cachedApp, found := s.getAppCache(1, "r")
	c.Assert(cachedApp, tc.DeepEquals, params.Settings{"baz": "quux"})
	c.Assert(found, tc.IsTrue)

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteApplication: "r",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "", "r")
	c.Assert(rel.UnitNames(), tc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, tc.IsTrue)
	// It should not be found in the normal cache
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, tc.IsNil)
	c.Assert(member, tc.IsFalse)
	cachedApp, found = s.getAppCache(1, "r")
	c.Assert(cachedApp, tc.IsNil)
	c.Assert(found, tc.IsFalse)
}

func (s *ContextFactorySuite) TestNewHookContextRelationDepartedUpdatesRelationContextAndCaches(c *tc.C) {
	// Update member settings to have actual values, so we can check that
	// the depart for r/0 leaves r/4's cache alone (while discarding r/0's).
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:          hooks.RelationDeparted,
		RelationId:    1,
		RemoteUnit:    "r/0",
		DepartingUnit: "r/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "")
	c.Assert(rel.UnitNames(), tc.DeepEquals, []string{"r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, tc.IsNil)
	c.Assert(member, tc.IsFalse)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, tc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationBrokenRetainsCaches(c *tc.C) {
	// Note that this is bizarre and unrealistic, because we would never usually
	// run relation-broken on a non-empty relation. But verfying that the settings
	// stick around allows us to verify that there's no special handling for that
	// hook -- as there should not be, because the relation caches will be discarded
	// for the *next* hook, which will be constructed with the current set of known
	// relations and ignore everything else.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	rel := s.AssertRelationContext(c, ctx, 1, "", "")
	c.Assert(rel.UnitNames(), tc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, tc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, tc.IsTrue)
}

func (s *ContextFactorySuite) TestReadApplicationSettings(c *tc.C) {
	s.setUpCacheMethods(c)
	// First, try to read the ApplicationSettings but not as the leader, ensure we get an error
	// Make sure this unit is the leader
	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, tc.ErrorIsNil)
	s.membership[0] = []string{"r/0"}
	rel, err := ctx.Relation(0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = rel.ApplicationSettings()
	c.Assert(err, tc.ErrorMatches, "permission denied.*")
	// Now claim leadership and try again
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.State.ModelUUID())
	c.Assert(err, tc.ErrorIsNil)
	err = claimer.Claim(s.unit.ApplicationName(), s.unit.Name(), time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	settings, err := rel.ApplicationSettings()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(settings.Map(), tc.DeepEquals, params.Settings{})
}

type StubLeadershipContext struct {
	context.LeadershipContext
	*testhelpers.Stub
	isLeader bool
}

func (stub *StubLeadershipContext) IsLeader() (bool, error) {
	stub.MethodCall(stub, "IsLeader")
	return stub.isLeader, stub.NextErr()
}
