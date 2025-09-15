// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/centralhub"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	hub    *pubsub.StructuredHub
	config centralhub.ManifoldConfig
}

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.hub = pubsub.NewStructuredHub(nil)
	s.config = centralhub.ManifoldConfig{
		StateConfigWatcherName: "state-config-watcher",
		Hub:                    s.hub,
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return centralhub.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"state-config-watcher"})
}

func (s *ManifoldSuite) TestStateConfigWatcherMissing(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStateConfigWatcherNotAStateServer(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": false,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMissingHub(c *tc.C) {
	s.config.Hub = nil
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": true,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
}

func (s *ManifoldSuite) TestHubOutput(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state-config-watcher": true,
	})

	manifold := s.manifold()
	worker, err := manifold.Start(context)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(worker, tc.NotNil)
	defer workertest.CleanKill(c, worker)

	var hub *pubsub.StructuredHub
	err = manifold.Output(worker, &hub)
	c.Check(err, tc.ErrorIsNil)
	c.Check(hub, tc.Equals, s.hub)
}
