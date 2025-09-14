// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelSuite struct {
	coretesting.BaseSuite
}

func TestModelSuite(t *tctesting.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) TestModel(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(id, tc.Equals, "")
		switch request {
		case "CurrentModel":
			c.Assert(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.ModelResult{})
			*(result.(*params.ModelResult)) = params.ModelResult{
				Name: "mary",
				UUID: "deadbeaf",
				Type: "caas",
			}
		default:
			c.Fatalf("unexpected api call %q", request)
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	m, err := client.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, &model.Model{
		Name:      "mary",
		UUID:      "deadbeaf",
		ModelType: model.CAAS,
	})
}
