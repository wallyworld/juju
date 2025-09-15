// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	"reflect"
	"sync"
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/auditconfigupdater"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
)

type updaterSuite struct {
	jujutesting.BaseSuite
}

func TestUpdaterSuite(t *tctesting.T) {
	tc.Run(t, &updaterSuite{})
}

var ding = struct{}{}

func (s *updaterSuite) TestWorker(c *tc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: false,
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(false, false),
	}

	fakeTarget := apitesting.FakeAuditLog{}
	var calls []auditlog.Config
	factory := func(cfg auditlog.Config) auditlog.AuditLog {
		calls = append(calls, cfg)
		return &fakeTarget
	}

	w, err := auditconfigupdater.New(&source, initial, factory)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.Enabled
	})

	c.Assert(newConfig.Enabled, tc.Equals, true)
	c.Assert(newConfig.CaptureAPIArgs, tc.Equals, false)
	c.Assert(newConfig.ExcludeMethods, tc.DeepEquals, set.NewStrings())
	c.Assert(newConfig.Target, tc.Equals, auditlog.AuditLog(&fakeTarget))
	c.Assert(calls, tc.HasLen, 1)
}

func waitForConfig(c *tc.C, w worker.Worker, predicate func(auditlog.Config) bool) auditlog.Config {
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		config := getWorkerConfig(c, w)
		if predicate(config) {
			return config
		}
	}
	c.Fatalf("timed out waiting for matching config")
	return auditlog.Config{}
}

func (s *updaterSuite) TestKeepsLogFileWhenAuditingDisabled(c *tc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: true,
		Target:  &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false),
	}

	// Passing a nil factory means we can be sure it didn't try to
	// create a new logfile.
	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(false, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return !cfg.Enabled
	})

	c.Assert(newConfig.Enabled, tc.Equals, false)
	c.Assert(newConfig.Target, tc.Equals, initial.Target)
}

func (s *updaterSuite) TestKeepsLogFileWhenEnabled(c *tc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: false,
		Target:  &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(false, false),
	}

	// Passing a nil factory means we can be sure it didn't try to
	// create a new logfile.
	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.Enabled
	})

	c.Assert(newConfig.Enabled, tc.Equals, true)
	c.Assert(newConfig.Target, tc.Equals, initial.Target)
}

func (s *updaterSuite) TestChangingExcludeMethod(c *tc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("Pink.Floyd"),
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false, "Pink.Floyd", "Led.Zeppelin"))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return reflect.DeepEqual(cfg.ExcludeMethods, set.NewStrings("Pink.Floyd", "Led.Zeppelin"))
	})

	source.setConfig(makeControllerConfig(true, false, "Led.Zeppelin"))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return reflect.DeepEqual(cfg.ExcludeMethods, set.NewStrings("Led.Zeppelin"))
	})
}

func (s *updaterSuite) TestChangingCaptureArgs(c *tc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled:        true,
		CaptureAPIArgs: false,
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, true))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.CaptureAPIArgs
	})
}

func makeControllerConfig(auditEnabled bool, captureArgs bool, methods ...interface{}) controller.Config {
	result := map[string]interface{}{
		"other-setting":             "something",
		"auditing-enabled":          auditEnabled,
		"audit-log-capture-args":    captureArgs,
		"audit-log-exclude-methods": methods,
	}
	return result
}

func getWorkerConfig(c *tc.C, w worker.Worker) auditlog.Config {
	getter, ok := w.(interface {
		CurrentConfig() auditlog.Config
	})
	if !ok {
		c.Fatalf("worker %T doesn't expose CurrentConfig()", w)
	}
	return getter.CurrentConfig()
}

type configSource struct {
	mu      sync.Mutex
	stub    testhelpers.Stub
	watcher *watchertest.NotifyWatcher
	cfg     controller.Config
}

func (s *configSource) WatchControllerConfig() state.NotifyWatcher {
	s.stub.AddCall("WatchControllerConfig")
	return s.watcher
}

func (s *configSource) ControllerConfig() (controller.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stub.AddCall("ControllerConfig")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.cfg, nil
}

func (s *configSource) setConfig(cfg controller.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}
