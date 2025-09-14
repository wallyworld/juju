// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
)

type upgradeSeriesGraphSuite struct {
	testing.BaseSuite
}

func TestUpgradeSeriesGraphSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesGraphSuite{})
}

func (*upgradeSeriesGraphSuite) TestUpgradeSeriesGraphValidate(c *tc.C) {
	graph := model.UpgradeSeriesGraph()
	err := graph.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (*upgradeSeriesGraphSuite) TestValidate(c *tc.C) {
	graph := model.Graph(map[model.UpgradeSeriesStatus][]model.UpgradeSeriesStatus{
		model.UpgradeSeriesNotStarted: {
			model.UpgradeSeriesPrepareStarted,
		},
	})
	err := graph.Validate()
	c.Assert(err, tc.ErrorMatches, `vertex "not started" edge to vertex "prepare started" is not valid`)
}

type upgradeSeriesFSMSuite struct {
	testing.BaseSuite
}

func TestUpgradeSeriesFSMSuite(t *tctesting.T) {
	tc.Run(t, &upgradeSeriesFSMSuite{})
}

func (*upgradeSeriesFSMSuite) TestTransitionTo(c *tc.C) {
	for _, t := range []struct {
		expected model.UpgradeSeriesStatus
		state    model.UpgradeSeriesStatus
		valid    bool
	}{
		{
			expected: model.UpgradeSeriesPrepareStarted,
			state:    model.UpgradeSeriesPrepareStarted,
			valid:    true,
		},
		{
			expected: model.UpgradeSeriesNotStarted,
			state:    model.UpgradeSeriesStatus("GTFO"),
			valid:    false,
		},
	} {
		fsm, err := model.NewUpgradeSeriesFSM(model.UpgradeSeriesGraph(), model.UpgradeSeriesNotStarted)
		c.Assert(err, tc.ErrorIsNil)

		allowed := fsm.TransitionTo(t.state)
		c.Assert(allowed, tc.Equals, t.valid)
		c.Assert(fsm.State(), tc.Equals, t.expected)
	}
}

func (*upgradeSeriesFSMSuite) TestTransitionGraph(c *tc.C) {
	dag := model.UpgradeSeriesGraph()
	for state, vertices := range dag {
		c.Logf("current state %q", state)

		for _, vertex := range vertices {
			fsm, err := model.NewUpgradeSeriesFSM(dag, state)
			c.Assert(err, tc.ErrorIsNil)

			allowed := fsm.TransitionTo(vertex)
			c.Assert(allowed, tc.IsTrue, tc.Commentf("transition %q to %q", fsm.State(), vertex))
		}
	}
}

func (*upgradeSeriesFSMSuite) TestTransitionGraphChildren(c *tc.C) {
	dag := model.UpgradeSeriesGraph()
	for state, vertices := range dag {
		c.Logf("current state %q", state)

		for _, vertex := range vertices {
			fsm, err := model.NewUpgradeSeriesFSM(dag, state)
			c.Assert(err, tc.ErrorIsNil)

			allowed := fsm.TransitionTo(vertex)
			c.Assert(allowed, tc.IsTrue)

			// Can we transition to the child vertex?
			children := dag[vertex]
			if len(children) == 0 {
				continue
			}
			allowed = fsm.TransitionTo(children[0])
			c.Assert(allowed, tc.IsTrue, tc.Commentf("transition %q to %q", fsm.State(), children[0]))
		}
	}
}
