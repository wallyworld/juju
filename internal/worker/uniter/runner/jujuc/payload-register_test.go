// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type registerSuite struct {
	testhelpers.IsolationSuite
}

func TestRegisterSuite(t *tctesting.T) {
	tc.Run(t, &registerSuite{})
}

func (s *registerSuite) TestInitNilArgs(c *tc.C) {
	cmd := jujuc.PayloadRegisterCmd{}
	err := cmd.Init(nil)
	c.Assert(err, tc.NotNil)
}

func (s *registerSuite) TestInitTooFewArgs(c *tc.C) {
	cmd := jujuc.PayloadRegisterCmd{}
	err := cmd.Init([]string{"foo", "bar"})
	c.Assert(err, tc.NotNil)
}

func (s *registerSuite) TestRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	payload := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		ID:     "id",
		Status: payloads.StateRunning,
		Labels: []string{"tag1", "tag2"},
		Unit:   "a-application/0",
	}
	hctx.EXPECT().TrackPayload(payload).Return(nil)
	hctx.EXPECT().FlushPayloads()

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "class", "id", "tag1", "tag2"})
	c.Assert(code, tc.Equals, 0)
}

func (s *registerSuite) TestRunUnknownClass(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "badclass", "id", "tag1", "tag2"})
	c.Assert(code, tc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `ERROR payload "badclass" not found in metadata.yaml`+"\n")
}

func (s *registerSuite) TestRunUnknownType(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"badtype", "class", "id", "tag1", "tag2"})
	c.Assert(code, tc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `ERROR incorrect type "badtype" for payload "class", expected "type"`+"\n")
}

func (s *registerSuite) TestRunError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	payload := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		ID:     "id",
		Status: payloads.StateRunning,
		Labels: []string{"tag1", "tag2"},
		Unit:   "a-application/0",
	}
	hctx.EXPECT().TrackPayload(payload).Return(errors.New("boom"))

	com, err := jujuc.NewCommand(hctx, "payload-register")
	c.Assert(err, tc.ErrorIsNil)
	ctx := setupMetadata(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"type", "class", "id", "tag1", "tag2"})
	c.Assert(code, tc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `ERROR boom`+"\n")
}

func setupMetadata(c *tc.C) *cmd.Context {
	dir := c.MkDir()
	path := filepath.Join(dir, "metadata.yaml")
	err := os.WriteFile(path, []byte(metadataContents), 0660)
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	ctx.Dir = dir
	return ctx
}

const metadataContents = `name: ducksay
summary: Testing charm payload management
maintainer: juju@canonical.com <Juju>
description: |
  Testing payloads
subordinate: false
payloads:
  class:
    type: type
    lifecycle: ["restart"]
`
