// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"context"
	"reflect"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/environ"
)

type TrackerSuite struct {
	coretesting.BaseSuite
}

func TestTrackerSuite(t *tctesting.T) {
	tc.Run(t, &TrackerSuite{})
}

func (s *TrackerSuite) validConfig(configAPI environ.ConfigAPI) environ.Config {
	if configAPI == nil {
		configAPI = &runContext{}
	}
	return environ.Config{
		ConfigAPI:      configAPI,
		NewEnvironFunc: newMockEnviron,
		Logger:         loggo.GetLogger("test"),
	}
}

func (s *TrackerSuite) TestValidateObserver(c *tc.C) {
	config := s.validConfig(nil)
	config.ConfigAPI = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, "nil ConfigAPI not valid")
	})
}

func (s *TrackerSuite) TestValidateNewEnvironFunc(c *tc.C) {
	config := s.validConfig(nil)
	config.NewEnvironFunc = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, "nil NewEnvironFunc not valid")
	})
}

func (s *TrackerSuite) TestValidateLogger(c *tc.C) {
	config := s.validConfig(nil)
	config.Logger = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, "nil Logger not valid")
	})
}

func (s *TrackerSuite) testValidate(c *tc.C, config environ.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	tracker, err := environ.NewTracker(config)
	c.Check(tracker, tc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestModelConfigFails(c *tc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, errors.New("no you"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Check(err, tc.ErrorMatches, "retrieving model config: no you")
		c.Check(tracker, tc.IsNil)
		context.CheckCallNames(c, "ControllerConfig", "ModelConfig")
	})
}

func (s *TrackerSuite) TestModelConfigInvalid(c *tc.C) {
	fix := &fixture{}
	fix.Run(c, func(runContext *runContext) {
		config := s.validConfig(runContext)
		config.NewEnvironFunc = func(context.Context, environs.OpenParams) (environs.Environ, error) {
			return nil, errors.NotValidf("config")
		}
		tracker, err := environ.NewTracker(config)
		c.Check(err, tc.ErrorMatches,
			`creating environ for model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): config not valid`)
		c.Check(tracker, tc.IsNil)
		runContext.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec")
	})
}

func (s *TrackerSuite) TestModelConfigValid(c *tc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "this-particular-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Assert(err, tc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotEnviron := tracker.Environ()
		c.Assert(gotEnviron, tc.NotNil)
		c.Check(gotEnviron.Config().Name(), tc.Equals, "this-particular-name")
	})
}

func (s *TrackerSuite) TestCloudSpec(c *tc.C) {
	cloudSpec := environscloudspec.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{initialSpec: cloudSpec}
	fix.Run(c, func(runContext *runContext) {
		config := s.validConfig(runContext)
		config.NewEnvironFunc = func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
			c.Assert(args.Cloud, tc.DeepEquals, cloudSpec)
			return nil, errors.NotValidf("cloud spec")
		}
		tracker, err := environ.NewTracker(config)
		c.Check(err, tc.ErrorMatches,
			`creating environ for model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): cloud spec not valid`)
		c.Check(tracker, tc.IsNil)
		runContext.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec")
	})
}

func (s *TrackerSuite) TestWatchFails(c *tc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, errors.New("grrk splat"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Assert(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		err = workertest.CheckKilled(c, tracker)
		c.Check(err, tc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): watching environ config: grrk splat`)
		context.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec", "WatchForModelConfigChanges")
	})
}

func (s *TrackerSuite) TestModelConfigWatchCloses(c *tc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Assert(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, tc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): environ config watch closed`)
		context.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *TrackerSuite) TestCloudSpecWatchCloses(c *tc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Assert(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseCloudSpecNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, tc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): cloud watch closed`)
		context.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigFails(c *tc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, nil, nil, errors.New("blam ouch"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Check(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, tc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): reading model config: blam ouch`)
		context.CheckCallNames(c, "ControllerConfig", "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges", "ModelConfig")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigIncompatible(c *tc.C) {
	fix := &fixture{}
	fix.Run(c, func(runContext *runContext) {
		config := s.validConfig(runContext)
		config.NewEnvironFunc = func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
			env := &mockEnviron{cfg: args.Config}
			env.SetErrors(nil, errors.New("SetConfig is broken"))
			return env, nil
		}
		tracker, err := environ.NewTracker(config)
		c.Check(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		runContext.SendModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, tc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): updating environ config: SetConfig is broken`)
		runContext.CheckCallNames(c,
			"ControllerConfig", "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges", "ModelConfig")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigUpdates(c *tc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "original-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Check(err, tc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetConfig(c, coretesting.Attrs{
			"name": "updated-name",
		})
		gotEnviron := tracker.Environ()
		c.Assert(gotEnviron.Config().Name(), tc.Equals, "original-name")

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendModelConfigNotify()
		for {
			select {
			case <-attempt:
				name := gotEnviron.Config().Name()
				if name == "original-name" {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(name, tc.Equals, "updated-name")
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
	})
}

func (s *TrackerSuite) TestWatchedCloudSpecUpdates(c *tc.C) {
	fix := &fixture{
		initialSpec: environscloudspec.CloudSpec{Name: "cloud", Type: "lxd"},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Check(err, tc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetCloudSpec(c, environscloudspec.CloudSpec{Name: "lxd", Type: "lxd", Endpoint: "http://api"})
		gotEnviron := tracker.Environ().(*mockEnviron)
		c.Assert(gotEnviron.CloudSpec(), tc.DeepEquals, fix.initialSpec)

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendCloudSpecNotify()
		for {
			select {
			case <-attempt:
				ep := gotEnviron.CloudSpec().Endpoint
				if ep == "" {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(ep, tc.Equals, "http://api")
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
	})
}

func (s *TrackerSuite) TestWatchedCloudSpecCredentialsUpdates(c *tc.C) {
	original := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "user",
			"password": "secret",
		},
	)
	differentContent := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "user",
			"password": "not-secret-anymore",
		},
	)
	fix := &fixture{
		initialSpec: environscloudspec.CloudSpec{Name: "cloud", Type: "lxd", Credential: &original},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(s.validConfig(context))
		c.Check(err, tc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetCloudSpec(c, environscloudspec.CloudSpec{Name: "lxd", Type: "lxd", Credential: &differentContent})
		gotEnviron := tracker.Environ().(*mockEnviron)
		c.Assert(gotEnviron.CloudSpec(), tc.DeepEquals, fix.initialSpec)

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendCloudSpecNotify()
		for {
			select {
			case <-attempt:
				ep := gotEnviron.CloudSpec().Credential
				if reflect.DeepEqual(ep, &original) {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(reflect.DeepEqual(ep, &differentContent), tc.IsTrue)
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
	})
}
