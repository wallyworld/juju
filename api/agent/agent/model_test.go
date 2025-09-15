// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/agent"
	apitesting "github.com/juju/juju/api/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type modelSuite struct {
	jujutesting.JujuConnSuite
	*apitesting.ModelWatcherTests
}

func TestModelSuite(t *tctesting.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)

	agentAPI, err := agent.NewState(stateAPI)
	c.Assert(agentAPI, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)

	s.ModelWatcherTests = apitesting.NewModelWatcherTests(
		agentAPI, s.BackingState, s.Model,
	)
}
