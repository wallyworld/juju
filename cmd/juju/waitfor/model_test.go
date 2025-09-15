// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type modelScopeSuite struct {
	testhelpers.IsolationSuite
}

func TestModelScopeSuite(t *tctesting.T) {
	tc.Run(t, &modelScopeSuite{})
}

func (s *modelScopeSuite) TestGetIdentValue(c *tc.C) {
	tests := []struct {
		Field     string
		ModelInfo *params.ModelUpdate
		Expected  query.Box
	}{{
		Field:     "name",
		ModelInfo: &params.ModelUpdate{Name: "model name"},
		Expected:  query.NewString("model name"),
	}, {
		Field:     "life",
		ModelInfo: &params.ModelUpdate{Life: life.Alive},
		Expected:  query.NewString("alive"),
	}, {
		Field:     "is-controller",
		ModelInfo: &params.ModelUpdate{IsController: false},
		Expected:  query.NewBool(false),
	}, {
		Field: "status",
		ModelInfo: &params.ModelUpdate{Status: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := ModelScope{
			ctx:       MakeScopeContext(),
			ModelInfo: test.ModelInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.Expected)
	}
}

func (s *modelScopeSuite) TestGetIdentValueError(c *tc.C) {
	scope := ModelScope{
		ctx:       MakeScopeContext(),
		ModelInfo: &params.ModelUpdate{},
	}
	result, err := scope.GetIdentValue("bad")
	c.Assert(err, tc.ErrorMatches, `.*"bad" on ModelInfo.*`)
	c.Assert(result, tc.IsNil)
}
