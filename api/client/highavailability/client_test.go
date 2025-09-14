// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/highavailability"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
}

func TestClientSuite(t *tctesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestClientEnableHA(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	emptyCons := constraints.Value{}

	args := params.ControllersSpecs{Specs: []params.ControllersSpec{{
		Constraints:    emptyCons,
		NumControllers: 3,
		Placement:      nil,
	},
	}}
	res := new(params.ControllersChangeResults)
	results := params.ControllersChangeResults{
		Results: []params.ControllersChangeResult{{
			Result: params.ControllersChanges{
				Maintained: []string{"machine-0"},
				Added:      []string{"machine-1", "machine-2"},
				Removed:    []string{},
			}},
		}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("EnableHA", args, res).SetArg(2, results).Return(nil)
	client := highavailability.NewClientFromCaller(mockFacadeCaller)

	result, err := client.EnableHA(3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(result.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(result.Removed, tc.HasLen, 0)
}
