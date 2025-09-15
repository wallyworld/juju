// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/machineundertaker"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (*manifoldSuite) TestMissingCaller(c *tc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  dependency.ErrMissing,
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, tc.IsNil)
	c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*manifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  &fakeAPICaller{},
		"the-environ": dependency.ErrMissing,
	}))
	c.Assert(result, tc.IsNil)
	c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*manifoldSuite) TestAPIError(c *tc.C) {
	manifold := makeManifold(nil, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  &fakeAPICaller{},
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "machine undertaker client requires a model API connection")
}

func (*manifoldSuite) TestWorkerError(c *tc.C) {
	manifold := makeManifold(nil, errors.New("boglodite"))
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  apitesting.APICallerFunc(nil),
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "boglodite")
}

func (*manifoldSuite) TestSuccess(c *tc.C) {
	w := fakeWorker{name: "Boris"}
	manifold := makeManifold(&w, nil)
	result, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"the-caller":  apitesting.APICallerFunc(nil),
		"the-environ": &fakeEnviron{},
	}))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &w)
}

func makeManifold(workerResult worker.Worker, workerError error) dependency.Manifold {
	return machineundertaker.Manifold(machineundertaker.ManifoldConfig{
		APICallerName: "the-caller",
		EnvironName:   "the-environ",
		Logger:        loggo.GetLogger("test"),
		NewWorker: func(machineundertaker.Facade, environs.Environ, common.CredentialAPI, machineundertaker.Logger) (worker.Worker, error) {
			return workerResult, workerError
		},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) {
			return &fakeCredentialAPI{}, nil
		},
	})
}

type fakeAPICaller struct {
	base.APICaller
}

func (c *fakeAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.ModelTag{}, false
}

type fakeWorker struct {
	worker.Worker
	name string
}
