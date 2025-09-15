// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/block"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// BlockHelper helps manage blocks for apiserver tests.
// It provides easy access to switch blocks on
// as well as test whether operations are blocked or not.
type BlockHelper struct {
	apiState api.Connection
	client   *block.Client
}

// NewBlockHelper creates a block switch used in testing
// to manage desired juju blocks.
func NewBlockHelper(st api.Connection) BlockHelper {
	return BlockHelper{
		apiState: st,
		client:   block.NewClient(st),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s BlockHelper) on(c *tc.C, blockType model.BlockType, msg string) {
	c.Assert(s.client.SwitchBlockOn(fmt.Sprintf("%v", blockType), msg), tc.IsNil)
}

// BlockAllChanges blocks all operations that could change the model.
func (s BlockHelper) BlockAllChanges(c *tc.C, msg string) {
	s.on(c, model.BlockChange, msg)
}

// BlockRemoveObject blocks all operations that remove
// machines, services, units or relations.
func (s BlockHelper) BlockRemoveObject(c *tc.C, msg string) {
	s.on(c, model.BlockRemove, msg)
}

func (s BlockHelper) Close() {
	s.client.Close()
	s.apiState.Close()
}

// BlockDestroyModel blocks destroy-model.
func (s BlockHelper) BlockDestroyModel(c *tc.C, msg string) {
	s.on(c, model.BlockDestroy, msg)
}

// AssertBlocked checks if given error is
// related to switched block.
func (s BlockHelper) AssertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}
