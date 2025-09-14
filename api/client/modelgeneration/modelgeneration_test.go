// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelgeneration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

type modelGenerationSuite struct {
	fCaller *mocks.MockFacadeCaller

	branchName string
}

func TestModelGenerationSuite(t *tctesting.T) {
	tc.Run(t, &modelGenerationSuite{})
}

func (s *modelGenerationSuite) SetUpTest(c *tc.C) {
	s.branchName = "new-branch"
}

func (s *modelGenerationSuite) TearDownTest(c *tc.C) {
	s.fCaller = nil
}

func (s *modelGenerationSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	caller := mocks.NewMockAPICallCloser(ctrl)
	caller.EXPECT().BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.fCaller.EXPECT().RawAPICaller().Return(caller).AnyTimes()

	return ctrl
}

func (s *modelGenerationSuite) TestAddBranch(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.ErrorResult{}
	arg := params.BranchArg{BranchName: s.branchName}
	s.fCaller.EXPECT().FacadeCall("AddBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.AddBranch(s.branchName)
	c.Assert(err, tc.IsNil)
}

func (s *modelGenerationSuite) TestAbortBranch(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.ErrorResult{}
	arg := params.BranchArg{BranchName: s.branchName}
	s.fCaller.EXPECT().FacadeCall("AbortBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.AbortBranch(s.branchName)
	c.Assert(err, tc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchSuccess(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultsSource := params.ErrorResults{Results: []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	}}
	arg := params.BranchTrackArg{
		BranchName: s.branchName,
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	s.fCaller.EXPECT().FacadeCall("TrackBranch", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.TrackBranch(s.branchName, []string{"mysql/0", "mysql"}, 0)
	c.Assert(err, tc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchError(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.TrackBranch(s.branchName, []string{"mysql/0", "mysql", "machine-3"}, 0)
	c.Assert(err, tc.ErrorMatches, `"machine-3" is not an application or a unit`)
}

func (s *modelGenerationSuite) TestCommitBranch(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.IntResult{Result: 2}
	arg := params.BranchArg{BranchName: s.branchName}
	s.fCaller.EXPECT().FacadeCall("CommitBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	newGenID, err := api.CommitBranch("new-branch")
	c.Assert(err, tc.IsNil)
	c.Check(newGenID, tc.Equals, 2)
}

func (s *modelGenerationSuite) TestHasActiveBranch(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.BoolResult{Result: true}
	arg := params.BranchArg{BranchName: s.branchName}
	s.fCaller.EXPECT().FacadeCall("HasActiveBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	has, err := api.HasActiveBranch(s.branchName)
	c.Assert(err, tc.IsNil)
	c.Check(has, tc.IsTrue)
}

func (s *modelGenerationSuite) TestBranchInfo(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.BranchResults{Generations: []params.Generation{{
		BranchName: "new-branch",
		Created:    time.Time{}.Unix(),
		CreatedBy:  "test-user",
		Applications: []params.GenerationApplication{
			{
				ApplicationName: "redis",
				UnitProgress:    "1/2",
				UnitsTracking:   []string{"redis/0"},
				UnitsPending:    []string{"redis/1"},
				ConfigChanges:   map[string]interface{}{"databases": 8},
			},
		},
	}}}
	arg := params.BranchInfoArgs{
		BranchNames: []string{s.branchName},
		Detailed:    true,
	}

	s.fCaller.EXPECT().FacadeCall("BranchInfo", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)

	formatTime := func(t time.Time) string {
		return t.UTC().Format("2006-01-02 15:04:05")
	}

	apps, err := api.BranchInfo(s.branchName, true, formatTime)
	c.Assert(err, tc.IsNil)
	c.Check(apps, tc.DeepEquals, map[string]model.Generation{
		s.branchName: {
			Created:   "0001-01-01 00:00:00",
			CreatedBy: "test-user",
			Applications: []model.GenerationApplication{{
				ApplicationName: "redis",
				UnitProgress:    "1/2",
				UnitDetail: &model.GenerationUnits{
					UnitsTracking: []string{"redis/0"},
					UnitsPending:  []string{"redis/1"},
				},
				ConfigChanges: map[string]interface{}{"databases": 8},
			}},
		},
	})
}
