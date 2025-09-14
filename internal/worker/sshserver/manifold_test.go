// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"os"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.SSHJump)
	c.Assert(err, tc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func newManifoldConfig(l loggo.Logger, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:                 l,
		APICallerName:          "api-caller",
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *tc.C) {
	l := loggo.GetLogger("test")
	// Check config as expected.

	cfg := newManifoldConfig(l, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), tc.ErrorIsNil)

	// Entirely missing.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWorker.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing Logger.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Empty APICallerName.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.APICallerName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		APICallerName: "api-caller",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:          loggo.GetLogger("test"),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{
		"api-caller",
	})

	// Start the worker
	w, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
		}),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)
	workertest.CleanKill(c, w)
}

type mockAPICaller struct {
	base.APICaller
}

func (a mockAPICaller) BestFacadeVersion(facade string) int {
	return 0
}

func (s *manifoldSuite) TestManifoldUninstall(c *tc.C) {
	// Unset feature flag
	os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	manifold := Manifold(ManifoldConfig{
		APICallerName: "api-caller",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:          loggo.GetLogger("test"),
	})
	// Start the worker
	_, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
		}),
	)
	c.Assert(err, tc.ErrorIs, dependency.ErrUninstall)

}
