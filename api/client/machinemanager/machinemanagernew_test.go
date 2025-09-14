// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
The existing machinemanager_test.go uses a home grown mocking mechanism.
I wanted to establish the new suffixed file to have a place to start systematically moving those tests to use gomock.
There are two benefits to this

1) We can work piecemeal
2) We don't have to mix two mocking styles (in attempt to preserve one file) when transitioning between mocking styles

The plan is to start moving those old style tests and when finished delete the old file and mv the new file.
*/

package machinemanager_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/machinemanager"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestNewMachineManagerSuite(t *tctesting.T) {
	tc.Run(t, &NewMachineManagerSuite{})
}

type NewMachineManagerSuite struct {
	jujutesting.BaseSuite

	clientFacade *mocks.MockClientFacade
	facade       *mocks.MockFacadeCaller

	tag    names.Tag
	args   params.Entities
	client *machinemanager.Client
}

func (s *NewMachineManagerSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}

	s.BaseSuite.SetUpTest(c)
}

func (s *NewMachineManagerSuite) TestUpgradeSeriesValidate(c *tc.C) {
	defer s.setup(c).Finish()

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{
			{
				Entity:  params.Entity{Tag: names.NewMachineTag(s.tag.String()).String()},
				Channel: "16.04/stable",
			},
		},
	}
	result := params.UpgradeSeriesUnitsResult{
		UnitNames: []string{"ubuntu/0", "ubuntu/1"},
	}
	results := params.UpgradeSeriesUnitsResults{Results: []params.UpgradeSeriesUnitsResult{result}}
	s.facade.EXPECT().FacadeCall("UpgradeSeriesValidate", args, gomock.Any()).SetArg(2, results)

	unitNames, err := s.client.UpgradeSeriesValidate(s.tag.String(), "16.04/stable")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.DeepEquals, result.UnitNames)
}

func (s *NewMachineManagerSuite) TestUpgradeSeriesPrepareAlreadyInProgress(c *tc.C) {
	defer s.setup(c).Finish()

	arg := params.UpdateChannelArg{
		Entity:  params.Entity{Tag: s.tag.String()},
		Channel: "16.04/stable",
		Force:   true,
	}
	resultSource := params.ErrorResult{
		Error: &params.Error{
			Message: "lock already exists",
			Code:    params.CodeAlreadyExists,
		},
	}
	s.facade.EXPECT().FacadeCall("UpgradeSeriesPrepare", arg, gomock.Any()).SetArg(2, resultSource)

	err := s.client.UpgradeSeriesPrepare(s.tag.Id(), "16.04/stable", true)
	c.Assert(errors.IsAlreadyExists(err), tc.IsTrue)
}

func (s *NewMachineManagerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clientFacade = mocks.NewMockClientFacade(ctrl)
	s.facade = mocks.NewMockFacadeCaller(ctrl)

	s.client = machinemanager.ConstructClient(s.clientFacade, s.facade)

	return ctrl
}
