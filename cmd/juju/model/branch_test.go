// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type branchSuite struct {
	generationBaseSuite
}

func TestBranchSuite(t *tctesting.T) {
	tc.Run(t, &branchSuite{})
}

func (s *branchSuite) TestInit(c *tc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *branchSuite) TestInitNone(c *tc.C) {
	err := s.runInit()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *branchSuite) TestInitFail(c *tc.C) {
	err := s.runInit("test", "me")
	c.Assert(err, tc.ErrorMatches, "must specify a branch name to switch to or leave blank")
}

func (s *branchSuite) TestRunCommandMaster(c *tc.C) {
	ctx, err := s.runCommand(c, nil, coremodel.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Active branch set to \"master\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details.ActiveBranch, tc.Equals, coremodel.GenerationMaster)
}

func (s *branchSuite) TestRunCommandBranchExists(c *tc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)

	ctx, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Active branch set to \"new-branch\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details.ActiveBranch, tc.Equals, s.branchName)
}

func (s *branchSuite) TestRunCommandNoBranchError(c *tc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(false, nil)

	_, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, tc.ErrorMatches, `this model has no active branch "`+s.branchName+`"`)
}

func (s *branchSuite) TestRunCommandActiveBranch(c *tc.C) {
	ctx, err := s.runCommand(c, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Active branch is \"master\"\n")
}

func (s *branchSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewBranchCommandForTest(nil, s.store), args)
}

func (s *branchSuite) runCommand(c *tc.C, api model.BranchCommandAPI, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewBranchCommandForTest(api, s.store), args...)
}

func setUpSwitchMocks(c *tc.C) (*gomock.Controller, *mocks.MockBranchCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockBranchCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
