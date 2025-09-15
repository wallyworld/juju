// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/juju/juju/core/settings"
)

const (
	branchName      = "test-branch"
	defaultPassword = "default-pass"
	defaultCharmURL = "default-charm-url"
	defaultUnitName = "redis/0"
)

type charmConfigWatcherSuite struct {
	EntitySuite
}

func TestCharmConfigWatcherSuite(t *tctesting.T) {
	tc.Run(t, &charmConfigWatcherSuite{})
}

func (s *charmConfigWatcherSuite) TestTrackingBranchChangedNotified(c *tc.C) {
	// After initializing we expect not miss
	c.Check(testutil.ToFloat64(s.Gauges.CharmConfigHashCacheMiss), tc.Equals, float64(0))

	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	// After initializing the first watcher we expect one change and one miss
	c.Check(testutil.ToFloat64(s.Gauges.CharmConfigHashCacheMiss), tc.Equals, float64(1))

	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a tracked branch change with altered config.
	b := Branch{
		details: BranchChange{
			Name:   branchName,
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	// publish the second change.
	s.Hub.Publish(branchChange, b)

	s.assertOneChange(c, w, map[string]interface{}{"password": "new-pass"}, defaultCharmURL)

	// After the branchChange we expect another change and hence inc again.
	c.Check(testutil.ToFloat64(s.Gauges.CharmConfigHashCacheMiss), tc.Equals, float64(2))
	c.Check(testutil.ToFloat64(s.Gauges.CharmConfigHashCacheHit), tc.Equals, float64(0))

	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestNotTrackingBranchChangedNotNotified(c *tc.C) {
	// This will initialise the watcher without branch info.
	w := s.newWatcher(c, "redis/9", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{}, defaultCharmURL)

	// Publish a branch change with altered config.
	b := Branch{
		details: BranchChange{
			Name:   branchName,
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	s.Hub.Publish(branchChange, b)

	// Nothing should change.
	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestDifferentBranchChangedNotNotified(c *tc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a branch change with a different name to the tracked one.
	b := Branch{
		details: BranchChange{
			Name:   "some-other-branch",
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	s.Hub.Publish(branchChange, b)

	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestTrackingBranchMasterChangedNotified(c *tc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a change to master configuration.
	hc, _ := newHashCache(map[string]interface{}{"databases": 4}, nil, nil)
	s.Hub.Publish(applicationConfigChange, hc)

	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword, "databases": 4}, defaultCharmURL)
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestTrackingBranchCommittedNotNotified(c *tc.C) {
	w := s.newWatcher(c, "redis/0", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a branch removal.
	s.Hub.Publish(modelBranchRemove, branchName)
	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestNotTrackedBranchSeesMasterConfig(c *tc.C) {
	// Watcher is for a unit not tracking the branch.
	w := s.newWatcher(c, "redis/9", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{}, defaultCharmURL)
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestSameUnitDifferentCharmURLYieldsDifferentHash(c *tc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)
	h1 := w.Watcher.(*CharmConfigWatcher).configHash
	w.AssertStops()

	w = s.newWatcher(c, defaultUnitName, "different-charm-url")
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, "different-charm-url")
	h2 := w.Watcher.(*CharmConfigWatcher).configHash
	w.AssertStops()

	c.Check(h1, tc.Not(tc.Equals), h2)
}

func (s *charmConfigWatcherSuite) newWatcher(c *tc.C, unitName string, charmURL string) StringsWatcherC {
	appName, err := names.UnitApplication(unitName)
	c.Assert(err, tc.ErrorIsNil)

	// The topics can be arbitrary here;
	// these tests are isolated from actual cache behaviour.
	cfg := charmConfigWatcherConfig{
		model:                s.newStubModel(),
		unitName:             unitName,
		appName:              appName,
		charmURL:             charmURL,
		appConfigChangeTopic: applicationConfigChange,
		branchChangeTopic:    branchChange,
		branchRemoveTopic:    modelBranchRemove,
		hub:                  s.Hub,
		res:                  s.NewResident(),
	}

	w, err := newCharmConfigWatcher(cfg)
	c.Assert(err, tc.ErrorIsNil)

	// Wrap the watcher and ensure we get the default notification.
	wc := NewStringsWatcherC(c, w)
	return wc
}

// newStub model sets up a cached model containing a redis application
// and a branch with 2 redis units tracking it.
func (s *charmConfigWatcherSuite) newStubModel() *stubCharmConfigModel {
	app := newApplication(nil, s.Gauges, s.Hub, s.NewResident())
	app.setDetails(ApplicationChange{
		Name:   "redis",
		Config: map[string]interface{}{}},
	)

	branch := newBranch(s.Gauges, s.Hub, s.NewResident())
	branch.setDetails(BranchChange{
		Name:          branchName,
		AssignedUnits: map[string][]string{"redis": {"redis/0", "redis/1"}},
		Config:        map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", defaultPassword)}},
	})

	return &stubCharmConfigModel{
		app:      *app,
		branches: map[string]Branch{"0": *branch},
		metrics:  s.Gauges,
	}
}

// assertWatcherConfig unwraps the charm config watcher and ensures that its
// configuration hash matches that of the input configuration map.
func (s *charmConfigWatcherSuite) assertOneChange(
	c *tc.C, wc StringsWatcherC, cfg map[string]interface{}, extra ...string,
) {
	h, err := hashSettings(cfg, extra...)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange([]string{h})
}

type stubCharmConfigModel struct {
	app      Application
	branches map[string]Branch
	metrics  *ControllerGauges
}

func (m *stubCharmConfigModel) Application(name string) (Application, error) {
	if name == m.app.details.Name {
		return m.app, nil
	}
	return Application{}, errors.NotFoundf("application %q", name)
}

func (m *stubCharmConfigModel) Branches() []Branch {
	branches := make([]Branch, len(m.branches))
	i := 0
	for _, b := range m.branches {
		branches[i] = b.copy()
		i += 1
	}
	return branches
}

func (m *stubCharmConfigModel) Metrics() *ControllerGauges {
	return m.metrics
}
