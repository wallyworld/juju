// Copyright 2019 Canonical Ltd.
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

type abortSuite struct {
	generationBaseSuite
}

func TestAbortSuite(t *tctesting.T) {
	tc.Run(t, &abortSuite{})
}

func (s *abortSuite) TestInit(c *tc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *abortSuite) TestInitNoName(c *tc.C) {
	err := s.runInit()
	c.Assert(err, tc.ErrorMatches, "expected a branch name")
}

func (s *abortSuite) TestInitInvalidName(c *tc.C) {
	err := s.runInit(coremodel.GenerationMaster)
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
}

func (s *abortSuite) TestRunCommand(c *tc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)
	api.EXPECT().AbortBranch(s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Aborting all changes in \""+s.branchName+"\" and closing branch.\n"+
		"Active branch set to \"master\"\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details.ActiveBranch, tc.Equals, "master")
}

func (s *abortSuite) TestRunCommandFail(c *tc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)
	api.EXPECT().AbortBranch(s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, tc.ErrorMatches, "fail")
}

func (s *abortSuite) TestRunCommandFailHasActiveBranch(c *tc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(false, nil)

	_, err := s.runCommand(c, api)
	c.Assert(err, tc.ErrorMatches, "this model has no active branch \""+s.branchName+"\"")
}

func (s *abortSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewAbortCommandForTest(nil, s.store), args)
}

func (s *abortSuite) runCommand(c *tc.C, api model.AbortCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewAbortCommandForTest(api, s.store), s.branchName)
}

func setUpAbortMocks(c *tc.C) (*gomock.Controller, *mocks.MockAbortCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockAbortCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
