// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
)

type prunerSuite struct {
}

func TestPrunerSuite(t *tctesting.T) {
	tc.Run(t, &prunerSuite{})
}

func (s *prunerSuite) TestPrune(c *tc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Assert(request, tc.Equals, "Prune")
			c.Assert(a, tc.DeepEquals, params.StatusHistoryPruneArgs{
				MaxHistoryTime: time.Hour,
				MaxHistoryMB:   666,
			})
			c.Assert(result, tc.IsNil)
			called = true
			return nil
		},
	)
	client := NewClient(apiCaller)
	err := client.Prune(time.Hour, 666)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}
