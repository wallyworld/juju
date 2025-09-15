// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/payload"
	"github.com/juju/juju/internal/testhelpers"
)

func TestFormatterSuite(t *tctesting.T) {
	tc.Run(t, &formatterSuite{})
}

type formatterSuite struct {
	testhelpers.IsolationSuite
}

func (s *formatterSuite) TestFormatPayloadOkay(c *tc.C) {
	pl := payload.NewPayload("spam", "a-application", 1, 0)
	pl.Labels = []string{"a-tag"}
	formatted := payload.FormatPayload(pl)

	c.Check(formatted, tc.DeepEquals, payload.FormattedPayload{
		Unit:    "a-application/0",
		Machine: "1",
		ID:      "idspam",
		Type:    "docker",
		Class:   "spam",
		Labels:  []string{"a-tag"},
		Status:  "running",
	})
}
