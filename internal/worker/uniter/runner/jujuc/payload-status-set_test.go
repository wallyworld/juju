// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type PayloadStatusSetSuiye struct {
	testhelpers.IsolationSuite
}

func TestPayloadStatusSetSuiye(t *tctesting.T) {
	tc.Run(t, &PayloadStatusSetSuiye{})
}

func (s *PayloadStatusSetSuiye) TestTooFewArgs(c *tc.C) {
	cmd := jujuc.PayloadStatusSetCmd{}
	err := cmd.Init([]string{})
	c.Check(err, tc.ErrorMatches, `missing .*`)

	err = cmd.Init([]string{payloads.StateRunning})
	c.Check(err, tc.ErrorMatches, `missing .*`)
}

func (s *PayloadStatusSetSuiye) TestInvalidStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-status-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id", "created"})
	c.Assert(code, tc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `ERROR status "created" not supported; expected one of ["running", "starting", "stopped", "stopping"]`+"\n")
}

func (s *PayloadStatusSetSuiye) TestStatusSet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().SetPayloadStatus("class", "id", "stopped").Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, "payload-status-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"class", "id", "stopped"})
	c.Assert(code, tc.Equals, 0)
}
