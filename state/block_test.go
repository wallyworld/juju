// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type blockSuite struct {
	ConnSuite
}

func TestBlockSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &blockSuite{})
}

func (s *blockSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
}

func assertNoModelBlock(c *tc.C, st *state.State) {
	all, err := st.AllBlocks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all, tc.HasLen, 0)
}

func (s *blockSuite) TestNoInitialBlocks(c *tc.C) {
	assertNoModelBlock(c, s.State)
}

func (s *blockSuite) assertNoTypedBlock(c *tc.C, t state.BlockType) {
	one, found, err := s.State.GetBlockForType(t)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsFalse)
	c.Assert(one, tc.IsNil)
}

func (s *blockSuite) assertModelHasBlock(c *tc.C, st *state.State, t state.BlockType, msg string) {
	block, found, err := st.GetBlockForType(t)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	c.Assert(block, tc.NotNil)
	c.Assert(block.Type(), tc.Equals, t)
	tag, err := block.Tag()
	c.Assert(err, tc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag, tc.Equals, m.ModelTag())
	c.Assert(block.Message(), tc.Equals, msg)
}

func (s *blockSuite) switchOnBlock(c *tc.C, t state.BlockType, message ...string) {
	m := strings.Join(message, " ")
	err := s.State.SwitchBlockOn(state.DestroyBlock, m)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *blockSuite) TestSwitchOnBlock(c *tc.C) {
	s.switchOnBlock(c, state.DestroyBlock, "some message")
	s.assertModelHasBlock(c, s.State, state.DestroyBlock, "some message")
}

func (s *blockSuite) TestSwitchOnBlockAlreadyOn(c *tc.C) {
	s.switchOnBlock(c, state.DestroyBlock, "first message")
	s.switchOnBlock(c, state.DestroyBlock, "second message")
	s.assertModelHasBlock(c, s.State, state.DestroyBlock, "second message")
}

func (s *blockSuite) switchOffBlock(c *tc.C, t state.BlockType) {
	err := s.State.SwitchBlockOff(t)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *blockSuite) TestSwitchOffBlockNoBlock(c *tc.C) {
	s.switchOffBlock(c, state.DestroyBlock)
	assertNoModelBlock(c, s.State)
	s.assertNoTypedBlock(c, state.DestroyBlock)
}

func (s *blockSuite) TestSwitchOffBlock(c *tc.C) {
	s.switchOnBlock(c, state.DestroyBlock)
	s.switchOffBlock(c, state.DestroyBlock)
	assertNoModelBlock(c, s.State)
	s.assertNoTypedBlock(c, state.DestroyBlock)
}

func (s *blockSuite) TestNonsenseBlocked(c *tc.C) {
	bType := state.BlockType(42)
	// This could be useful for entity blocks...
	s.switchOnBlock(c, bType)
	s.switchOffBlock(c, bType)
	// but for multiwatcher, it should panic.
	c.Assert(func() { bType.ToParams() }, tc.PanicMatches, ".*unknown block type.*")
}

func (s *blockSuite) TestMultiModelBlocked(c *tc.C) {
	// create another model
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	// switch one block type on
	t := state.ChangeBlock
	msg := "another model tst"
	err := st2.SwitchBlockOn(t, msg)
	c.Assert(err, tc.ErrorIsNil)
	s.assertModelHasBlock(c, st2, t, msg)

	//check correct model has it
	assertNoModelBlock(c, s.State)
	s.assertNoTypedBlock(c, t)
}

func (s *blockSuite) TestAllBlocksForController(c *tc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(blocks), tc.Equals, 2)
}

func (s *blockSuite) TestRemoveAllBlocksForController(c *tc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveAllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(blocks), tc.Equals, 0)
}

func (s *blockSuite) TestRemoveAllBlocksForControllerNoBlocks(c *tc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.RemoveAllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := st2.AllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(blocks), tc.Equals, 0)
}

func (s *blockSuite) TestModelUUID(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	err := st.SwitchBlockOn(state.ChangeBlock, "blocktest")
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := st.AllBlocks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(blocks), tc.Equals, 1)
	c.Assert(blocks[0].ModelUUID(), tc.Equals, st.ModelUUID())
}

func (s *blockSuite) createTestModel(c *tc.C) (*state.Model, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	return model, st
}

func (s *blockSuite) TestConcurrentBlocked(c *tc.C) {
	switchBlockOn := func() {
		msg := ""
		t := state.DestroyBlock
		err := s.State.SwitchBlockOn(t, msg)
		c.Assert(err, tc.ErrorIsNil)
		s.assertModelHasBlock(c, s.State, t, msg)
	}
	defer state.SetBeforeHooks(c, s.State, switchBlockOn).Check()
	msg := "concurrency tst"
	t := state.RemoveBlock
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(err, tc.ErrorIsNil)
	s.assertModelHasBlock(c, s.State, t, msg)
}
