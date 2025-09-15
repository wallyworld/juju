// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type ControllerNodeSuite struct {
	ConnSuite
}

func TestControllerNodeSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ControllerNodeSuite{})
}

func (s *ControllerNodeSuite) TestAddControllerNode(c *tc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.IsManager(), tc.IsTrue)
	c.Assert(node.Tag().String(), tc.Equals, "controller-0")
	c.Assert(node.Life(), tc.Equals, state.Alive)
	node0, err := s.State.ControllerNode("0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node, tc.DeepEquals, node0)

	// Check id increments.
	node1, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node1.Id(), tc.Equals, "1")
}

func (s *ControllerNodeSuite) TestSetPassword(c *tc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	testSetPassword(c, func() (state.Authenticator, error) {
		return node, nil
	})
}

func (s *ControllerNodeSuite) TestSetMongoPassword(c *tc.C) {
	_, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	testSetMongoPassword(c, func(st *state.State, id string) (mongoPasswordSetter, error) {
		return st.ControllerNode("0")
	}, s.State.ControllerTag(), s.modelTag, s.Session)
}

func (s *ControllerNodeSuite) TestAgentTools(c *tc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	testAgentTools(c, node, "controller "+node.Id())
}
