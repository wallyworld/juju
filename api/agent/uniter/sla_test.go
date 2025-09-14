// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type slaSuite struct {
	coretesting.BaseSuite
}

func TestSlaSuite(t *tctesting.T) {
	tc.Run(t, &slaSuite{})
}

func (s *slaSuite) TestSLALevel(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SLALevel")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "essential",
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	level, err := client.SLALevel()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(level, tc.Equals, "essential")
}
