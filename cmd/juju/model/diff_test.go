// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type diffSuite struct {
	generationBaseSuite

	api *mocks.MockDiffCommandAPI
}

func TestDiffSuite(t *tctesting.T) {
	tc.Run(t, &diffSuite{})
}

func (s *diffSuite) TestInitNoBranch(c *tc.C) {
	err := s.runInit()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *diffSuite) TestInitBranchName(c *tc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *diffSuite) TestInitFail(c *tc.C) {
	err := s.runInit("multiple", "branch", "names")
	c.Assert(err, tc.ErrorMatches, "expected at most 1 branch name, got 3 arguments")
}

func (s *diffSuite) TestRunCommandNextGenExists(c *tc.C) {
	defer s.setup(c).Finish()

	result := map[string]coremodel.Generation{
		s.branchName: {
			Created:   "0001-01-01 00:00:00Z",
			CreatedBy: "test-user",
			Applications: []coremodel.GenerationApplication{{
				ApplicationName: "redis",
				UnitProgress:    "1/2",
				UnitDetail: &coremodel.GenerationUnits{
					UnitsTracking: []string{"redis/0"},
					UnitsPending:  []string{"redis/1"},
				},
				ConfigChanges: map[string]interface{}{"databases": 8},
			}},
		},
	}
	s.api.EXPECT().BranchInfo(s.branchName, true, gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
new-branch:
  created: 0001-01-01 00:00:00Z
  created-by: test-user
  applications:
  - application: redis
    progress: 1/2
    units:
      tracking:
      - redis/0
      incomplete:
      - redis/1
    config:
      databases: 8
`[1:])
}

func (s *diffSuite) TestRunCommandAPIError(c *tc.C) {
	defer s.setup(c).Finish()

	s.api.EXPECT().BranchInfo(s.branchName, true, gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.runCommand(c)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *diffSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewDiffCommandForTest(nil, s.store), args)
}

func (s *diffSuite) runCommand(c *tc.C) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewDiffCommandForTest(s.api, s.store), s.branchName, "--all")
}

func (s *diffSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockDiffCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
