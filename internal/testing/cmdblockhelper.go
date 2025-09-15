// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/block"
)

// CmdBlockHelper is a helper struct used to block commands.
type CmdBlockHelper struct {
	blockClient *block.Client
}

// NewCmdBlockHelper creates a block switch used in testing
// to manage desired juju blocks.
func NewCmdBlockHelper(api base.APICallCloser) CmdBlockHelper {
	return CmdBlockHelper{
		blockClient: block.NewClient(api),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s *CmdBlockHelper) on(c *tc.C, blockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(blockType, msg), tc.IsNil)
}

// BlockAllChanges switches changes block on.
// This prevents all changes to juju environment.
func (s *CmdBlockHelper) BlockAllChanges(c *tc.C, msg string) {
	s.on(c, "BlockChange", msg)
}

// BlockRemoveObject switches remove block on.
// This prevents any object/entity removal on juju environment
func (s *CmdBlockHelper) BlockRemoveObject(c *tc.C, msg string) {
	s.on(c, "BlockRemove", msg)
}

// BlockDestroyModel switches destroy block on.
// This prevents juju environment destruction.
func (s *CmdBlockHelper) BlockDestroyModel(c *tc.C, msg string) {
	s.on(c, "BlockDestroy", msg)
}

func (s *CmdBlockHelper) Close() {
	s.blockClient.Close()
}

// AssertBlocked is going to be removed as soon as all cmd tests mock out API.
// the corect method to call will become AssertOperationWasBlocked.
func (s *CmdBlockHelper) AssertBlocked(c *tc.C, err error, msg string) {
	if err == nil {
		c.Fail()
	}
	c.Assert(err.Error(), tc.Contains, "disabled")
}

func AssertOperationWasBlocked(c *tc.C, err error, msg string) {
	c.Assert(err.Error(), tc.Contains, "disabled", tc.Commentf("%s", errors.Details(err)))
}
