// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration/mocks"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/rpc/params"
)

type modelGenerationSuite struct {
	modelUUID     string
	newBranchName string
	apiUser       string

	api *modelgeneration.API

	mockState      *mocks.MockState
	mockModel      *mocks.MockModel
	mockGen        *mocks.MockGeneration
	mockModelCache *mocks.MockModelCache
}

func TestModelGenerationSuite(t *tctesting.T) {
	tc.Run(t, &modelGenerationSuite{})
}

func (s *modelGenerationSuite) SetUpSuite(c *tc.C) {
	s.modelUUID = "deadbeef-abcd-4fd2-967d-db9663db7bea"
	s.newBranchName = "new-branch"
	s.apiUser = "test-user"
}

func (s *modelGenerationSuite) TearDownTest(c *tc.C) {
	s.api = nil
}

// TODO (hml) 17-jan-2019
// Add more explicit permissions tests once that requirement is ironed out.

func (s *modelGenerationSuite) TestAddBranchInvalidNameError(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()

	arg := params.BranchArg{BranchName: model.GenerationMaster}
	result, err := s.api.AddBranch(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.Error.Message, tc.Matches, ".* not valid")
}

func (s *modelGenerationSuite) TestAddBranchSuccess(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectAddBranch()

	result, err := s.api.AddBranch(s.newBranchArg())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchEntityTypeError(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectAssignUnits("ghost", 0)
	s.expectAssignUnit("mysql/0")
	s.expectBranch()

	arg := params.BranchTrackArg{
		BranchName: s.newBranchName,
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
			{Tag: names.NewMachineTag("7").String()},
		},
	}
	result, err := s.api.TrackBranch(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: "expected names.UnitTag or names.ApplicationTag, got names.MachineTag"}},
	})
}

func (s *modelGenerationSuite) TestTrackBranchSuccess(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectAssignUnits("ghost", 0)
	s.expectAssignUnit("mysql/0")
	s.expectBranch()

	arg := params.BranchTrackArg{
		BranchName: s.newBranchName,
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
		},
	}
	result, err := s.api.TrackBranch(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	})
}

func (s *modelGenerationSuite) TestTrackBranchWithTooManyNumUnits(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()

	arg := params.BranchTrackArg{
		BranchName: s.newBranchName,
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
		},
		NumUnits: 1,
	}
	result, err := s.api.TrackBranch(arg)
	c.Assert(err, tc.ErrorMatches, "number of units and unit IDs can not be specified at the same time")
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult(nil))
}

func (s *modelGenerationSuite) TestCommitBranchSuccess(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectCommit()
	s.expectBranch()

	result, err := s.api.CommitBranch(s.newBranchArg())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.IntResult{Result: 3, Error: nil})
}

func (s *modelGenerationSuite) TestAbortBranchSuccess(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectAbort()
	s.expectBranch()

	result, err := s.api.AbortBranch(s.newBranchArg())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *modelGenerationSuite) TestHasActiveBranchTrue(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectHasActiveBranch(nil)

	result, err := s.api.HasActiveBranch(s.newBranchArg())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.Result, tc.IsTrue)
}

func (s *modelGenerationSuite) TestHasActiveBranchFalse(c *tc.C) {
	defer s.setupModelGenerationAPI(c).Finish()
	s.expectHasActiveBranch(errors.NotFoundf(s.newBranchName))

	result, err := s.api.HasActiveBranch(s.newBranchArg())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Check(result.Result, tc.IsFalse)
}

func (s *modelGenerationSuite) TestBranchInfoDetailed(c *tc.C) {
	s.testBranchInfo(c, nil, true)
}

func (s *modelGenerationSuite) TestBranchInfoSummary(c *tc.C) {
	s.testBranchInfo(c, []string{s.newBranchName}, false)
}

func (s *modelGenerationSuite) testBranchInfo(c *tc.C, branchNames []string, detailed bool) {
	ctrl := s.setupModelGenerationAPI(c)
	defer ctrl.Finish()

	units := []string{"redis/0", "redis/1", "redis/2"}

	s.expectConfig()
	s.expectBranchName()
	s.expectAssignedUnits(units[:2])
	s.expectCreated()
	s.expectCreatedBy()

	// Flex the code path based on whether we are getting all branches
	// or a sub-set.
	if len(branchNames) > 0 {
		s.expectBranch()
	} else {
		s.expectBranches()
	}

	s.setupMockApp(ctrl, units)

	result, err := s.api.BranchInfo(params.BranchInfoArgs{
		BranchNames: branchNames,
		Detailed:    detailed,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.Generations, tc.HasLen, 1)

	gen := result.Generations[0]
	c.Assert(gen.BranchName, tc.Equals, s.newBranchName)
	c.Assert(gen.Created, tc.Equals, int64(666))
	c.Assert(gen.CreatedBy, tc.Equals, s.apiUser)
	c.Assert(gen.Applications, tc.HasLen, 1)

	genApp := gen.Applications[0]
	c.Check(genApp.ApplicationName, tc.Equals, "redis")
	c.Check(genApp.UnitProgress, tc.Equals, "2/3")
	c.Check(genApp.ConfigChanges, tc.DeepEquals, map[string]interface{}{
		"password":  "added-pass",
		"databases": 16,
		"port":      8000,
	})

	// Unit lists are only populated when detailed is true.
	if detailed {
		c.Check(genApp.UnitsTracking, tc.SameContents, units[:2])
		c.Check(genApp.UnitsPending, tc.SameContents, units[2:])
	} else {
		c.Check(genApp.UnitsTracking, tc.IsNil)
		c.Check(genApp.UnitsPending, tc.IsNil)
	}
}

func (s *modelGenerationSuite) setupModelGenerationAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockGen = mocks.NewMockGeneration(ctrl)

	s.mockState = mocks.NewMockState(ctrl)
	s.mockState.EXPECT().ControllerTag().Return(names.NewControllerTag(s.modelUUID))

	s.mockModel = mocks.NewMockModel(ctrl)

	mockAuthorizer := facademocks.NewMockAuthorizer(ctrl)
	aExp := mockAuthorizer.EXPECT()
	aExp.HasPermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	aExp.GetAuthTag().Return(names.NewUserTag("test-user"))
	aExp.AuthClient().Return(true)

	s.mockModelCache = mocks.NewMockModelCache(ctrl)

	var err error
	s.api, err = modelgeneration.NewModelGenerationAPI(s.mockState, mockAuthorizer, s.mockModel, s.mockModelCache)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *modelGenerationSuite) newBranchArg() params.BranchArg {
	return params.BranchArg{BranchName: s.newBranchName}
}

func (s *modelGenerationSuite) expectAddBranch() {
	s.mockModel.EXPECT().AddBranch(s.newBranchName, s.apiUser).Return(nil)
}

func (s *modelGenerationSuite) expectBranches() {
	s.mockModel.EXPECT().Branches().Return([]modelgeneration.Generation{s.mockGen}, nil)
}

func (s *modelGenerationSuite) expectBranch() {
	s.mockModel.EXPECT().Branch(s.newBranchName).Return(s.mockGen, nil)
}

func (s *modelGenerationSuite) expectHasActiveBranch(err error) {
	s.mockModelCache.EXPECT().Branch(s.newBranchName).Return(cache.Branch{}, err)
}

func (s *modelGenerationSuite) expectAssignUnits(appName string, numUnits int) {
	s.mockGen.EXPECT().AssignUnits(appName, numUnits).Return(nil)
}

func (s *modelGenerationSuite) expectAssignUnit(unitName string) {
	s.mockGen.EXPECT().AssignUnit(unitName).Return(nil)
}

func (s *modelGenerationSuite) expectAbort() {
	s.mockGen.EXPECT().Abort(s.apiUser).Return(nil)
}

func (s *modelGenerationSuite) expectCommit() {
	s.mockGen.EXPECT().Commit(s.apiUser).Return(3, nil)
}

func (s *modelGenerationSuite) expectAssignedUnits(units []string) {
	s.mockGen.EXPECT().AssignedUnits().Return(map[string][]string{"redis": units})
}

func (s *modelGenerationSuite) expectBranchName() {
	s.mockGen.EXPECT().BranchName().Return(s.newBranchName)
}

func (s *modelGenerationSuite) expectCreated() {
	s.mockGen.EXPECT().Created().Return(int64(666))
}

func (s *modelGenerationSuite) expectCreatedBy() {
	s.mockGen.EXPECT().CreatedBy().Return(s.apiUser)
}

func (s *modelGenerationSuite) expectConfig() {
	s.mockGen.EXPECT().Config().Return(map[string]settings.ItemChanges{"redis": {
		settings.MakeAddition("password", "added-pass"),
		settings.MakeDeletion("databases", 100),
		settings.MakeModification("port", 7000, 8000),
	}})
}

func (s *modelGenerationSuite) setupMockApp(ctrl *gomock.Controller, units []string) {
	mockApp := mocks.NewMockApplication(ctrl)
	mockApp.EXPECT().DefaultCharmConfig().Return(map[string]interface{}{
		"databases": 16,
		"password":  "",
	}, nil)
	mockApp.EXPECT().UnitNames().Return(units, nil)

	s.mockState.EXPECT().Application("redis").Return(mockApp, nil)
}
