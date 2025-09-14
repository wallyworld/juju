// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

type helpersSuite struct {
}

func TestHelpersSuite(t *tctesting.T) {
	tc.Run(t, &helpersSuite{})
}

func (helpersSuite) TestPayload2api(c *tc.C) {
	apiPayload := Payload2api(payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payloads.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})

	c.Check(apiPayload, tc.DeepEquals, params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payloads.StateRunning,
		Labels:  []string{"a-tag"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	})
}

func (helpersSuite) TestAPI2Payload(c *tc.C) {
	pl, err := API2Payload(params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payloads.StateRunning,
		Labels:  []string{"a-tag"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(pl, tc.DeepEquals, payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payloads.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})
}
