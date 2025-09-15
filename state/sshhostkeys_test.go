// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type SSHHostKeysSuite struct {
	ConnSuite
	machineTag names.MachineTag
}

func TestSSHHostKeysSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SSHHostKeysSuite{})
}

func (s *SSHHostKeysSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.machineTag = s.Factory.MakeMachine(c, nil).MachineTag()
}

func (s *SSHHostKeysSuite) TestGetWithNoKeys(c *tc.C) {
	checkKeysNotFound(c, s.State, s.machineTag)
}

func (s *SSHHostKeysSuite) TestSetGet(c *tc.C) {
	for i := 0; i < 3; i++ {
		keys := state.SSHHostKeys{fmt.Sprintf("rsa foo %d", i), "dsa bar"}
		err := s.State.SetSSHHostKeys(s.machineTag, keys)
		c.Assert(err, tc.ErrorIsNil)
		checkGet(c, s.State, s.machineTag, keys)
	}
}

func (s *SSHHostKeysSuite) TestModelIsolation(c *tc.C) {
	stA := s.State
	tagA := s.machineTag
	keysA := state.SSHHostKeys{"rsaA", "dsaA"}
	c.Assert(stA.SetSSHHostKeys(tagA, keysA), tc.ErrorIsNil)

	stB := s.Factory.MakeModel(c, nil)
	defer stB.Close()
	factoryB := factory.NewFactory(stB, s.StatePool)
	tagB := factoryB.MakeMachine(c, nil).MachineTag()
	keysB := state.SSHHostKeys{"rsaB", "dsaB"}
	c.Assert(stB.SetSSHHostKeys(tagB, keysB), tc.ErrorIsNil)

	checkGet(c, stA, tagA, keysA)
	checkGet(c, stB, tagB, keysB)
}

func checkKeysNotFound(c *tc.C, st *state.State, tag names.MachineTag) {
	_, err := st.GetSSHHostKeys(tag)
	c.Check(errors.IsNotFound(err), tc.IsTrue)
	c.Check(err, tc.ErrorMatches, "keys not found")
}

func checkGet(c *tc.C, st *state.State, tag names.MachineTag, expected state.SSHHostKeys) {
	keysGot, err := st.GetSSHHostKeys(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keysGot, tc.DeepEquals, expected)
}
