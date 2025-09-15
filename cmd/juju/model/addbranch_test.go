// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type addBranchSuite struct {
	generationBaseSuite
}

func TestAddBranchSuite(t *tctesting.T) {
	tc.Run(t, &addBranchSuite{})
}

func (s *addBranchSuite) TestInit(c *tc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *addBranchSuite) TestInitNoName(c *tc.C) {
	err := s.runInit()
	c.Assert(err, tc.ErrorMatches, "expected a branch name")
}

func (s *addBranchSuite) TestInitInvalidName(c *tc.C) {
	err := s.runInit(coremodel.GenerationMaster)
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
}

func (s *addBranchSuite) TestRunCommand(c *tc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Created branch \""+s.branchName+"\" and set active\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details.ActiveBranch, tc.Equals, s.branchName)
}

func (s *addBranchSuite) TestRunCommandFail(c *tc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, tc.ErrorMatches, "fail")
}

func (s *addBranchSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewAddBranchCommandForTest(nil, s.store), args)
}

func (s *addBranchSuite) runCommand(c *tc.C, api model.AddBranchCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewAddBranchCommandForTest(api, s.store), s.branchName)
}

func setUpMocks(c *tc.C) (*gomock.Controller, *mocks.MockAddBranchCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockAddBranchCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
