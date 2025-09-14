// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/block"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type blockSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	api *block.API
}

func TestBlockSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &blockSuite{})
}

func (s *blockSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}
	s.api, err = block.NewAPI(facadetest.Context{
		State_:     s.State,
		Resources_: common.NewResources(),
		Auth_:      auth,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *blockSuite) TestListBlockNoneExistent(c *tc.C) {
	s.assertBlockList(c, 0)
}

func (s *blockSuite) assertBlockList(c *tc.C, length int) {
	all, err := s.api.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all.Results, tc.HasLen, length)
}

func (s *blockSuite) TestSwitchValidBlockOn(c *tc.C) {
	s.assertSwitchBlockOn(c, state.DestroyBlock.String(), "for TestSwitchValidBlockOn")
}

func (s *blockSuite) assertSwitchBlockOn(c *tc.C, blockType, msg string) {
	on := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	err := s.api.SwitchBlockOn(on)
	c.Assert(err.Error, tc.IsNil)
	s.assertBlockList(c, 1)
}

func (s *blockSuite) TestSwitchInvalidBlockOn(c *tc.C) {
	on := params.BlockSwitchParams{
		Type:    "invalid_block_type",
		Message: "for TestSwitchInvalidBlockOn",
	}

	c.Assert(func() { s.api.SwitchBlockOn(on) }, tc.PanicMatches, ".*unknown block type.*")
}

func (s *blockSuite) TestSwitchBlockOff(c *tc.C) {
	valid := state.DestroyBlock
	s.assertSwitchBlockOn(c, valid.String(), "for TestSwitchBlockOff")

	off := params.BlockSwitchParams{
		Type: valid.String(),
	}
	err := s.api.SwitchBlockOff(off)
	c.Assert(err.Error, tc.IsNil)
	s.assertBlockList(c, 0)
}
