// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/provisioner"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	stub testhelpers.Stub
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) makeManifold() dependency.Manifold {
	fakeNewProvFunc := func(
		*apiprovisioner.State,
		agent.Config,
		provisioner.Logger,
		environs.Environ,
		common.CredentialAPI,
	) (provisioner.Provisioner, error) {
		s.stub.AddCall("NewProvisionerFunc")
		return struct{ provisioner.Provisioner }{}, nil
	}
	return provisioner.Manifold(provisioner.ManifoldConfig{
		AgentName:                    "agent",
		APICallerName:                "api-caller",
		Logger:                       loggo.GetLogger("test"),
		EnvironName:                  "environ",
		NewProvisionerFunc:           fakeNewProvFunc,
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.stub.ResetCalls()
}

func (s *ManifoldSuite) TestManifold(c *tc.C) {
	manifold := s.makeManifold()
	c.Check(manifold.Inputs, tc.SameContents, []string{"agent", "api-caller", "environ"})
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Start, tc.NotNil)
}

func (s *ManifoldSuite) TestMissingAgent(c *tc.C) {
	manifold := s.makeManifold()
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": struct{ base.APICaller }{},
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingAPICaller(c *tc.C) {
	manifold := s.makeManifold()
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": dependency.ErrMissing,
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingEnviron(c *tc.C) {
	manifold := s.makeManifold()
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      struct{ agent.Agent }{},
		"api-caller": struct{ base.APICaller }{},
		"environ":    dependency.ErrMissing,
	}))
	c.Check(w, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStarts(c *tc.C) {
	manifold := s.makeManifold()
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"agent":      new(fakeAgent),
		"api-caller": apitesting.APICallerFunc(nil),
		"environ":    struct{ environs.Environ }{},
	}))
	c.Check(w, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
	s.stub.CheckCallNames(c, "NewProvisionerFunc")
}

type fakeAgent struct {
	agent.Agent
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return nil
}
