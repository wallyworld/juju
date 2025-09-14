// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type PayloadUnregisterSuite struct {
	testhelpers.IsolationSuite
}

func TestPayloadUnregisterSuite(t *tctesting.T) {
	tc.Run(t, &PayloadUnregisterSuite{})
}

func (s *PayloadUnregisterSuite) TestRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().UntrackPayload("class", "id").Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, "payload-unregister")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id"})
	c.Assert(code, tc.Equals, 0)
}
