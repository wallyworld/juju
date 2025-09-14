// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/rpc/params"
)

type prunerSuite struct{}

func TestPrunerSuite(t *tctesting.T) {
	tc.Run(t, &prunerSuite{})
}

func (s *prunerSuite) TestPrune(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ActionPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Prune", args, nil).Return(nil)

	client := action.NewPrunerFromCaller(mockFacadeCaller)
	err := client.Prune(time.Hour, 666)
	c.Assert(err, tc.ErrorIsNil)
}
