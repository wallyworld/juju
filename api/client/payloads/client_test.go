// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/payloads"
	corepayloads "github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct{}

func TestClientSuite(t *tctesting.T) {
	tc.Run(t, &ClientSuite{})
}

func (s *ClientSuite) TestList(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.PayloadListArgs{
		Patterns: []string{"a-tag", "a-application/0"},
	}

	payloadResult := params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  corepayloads.StateRunning,
		Labels:  []string{"label"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	}
	resultParams := params.PayloadListResults{
		Results: []params.Payload{payloadResult},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("List", args, gomock.Any()).SetArg(2, resultParams).Return(nil)
	client := payloads.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListFull("a-tag", "a-application/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []corepayloads.FullPayloadInfo{
		{
			Payload: corepayloads.Payload{
				PayloadClass: charm.PayloadClass{Name: "spam", Type: "docker"},
				ID:           "idspam",
				Status:       corepayloads.StateRunning,
				Labels:       []string{"label"},
				Unit:         "a-application/0",
			},
			Machine: "1",
		},
	})
}
