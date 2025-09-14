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

type unitScopeSuite struct {
	testhelpers.IsolationSuite
}

func TestUnitScopeSuite(t *tctesting.T) {
	tc.Run(t, &unitScopeSuite{})
}

func (s *unitScopeSuite) TestGetIdentValue(c *tc.C) {
	tests := []struct {
		Field    string
		UnitInfo *params.UnitInfo
		Expected query.Box
	}{{
		Field:    "name",
		UnitInfo: &params.UnitInfo{Name: "model name"},
		Expected: query.NewString("model name"),
	}, {
		Field:    "application",
		UnitInfo: &params.UnitInfo{Application: "app-name"},
		Expected: query.NewString("app-name"),
	}, {
		Field:    "base",
		UnitInfo: &params.UnitInfo{Base: "ubuntu@22.04"},
		Expected: query.NewString("ubuntu@22.04"),
	}, {
		Field:    "charm-url",
		UnitInfo: &params.UnitInfo{CharmURL: "charm-url-value"},
		Expected: query.NewString("charm-url-value"),
	}, {
		Field:    "life",
		UnitInfo: &params.UnitInfo{Life: life.Alive},
		Expected: query.NewString("alive"),
	}, {
		Field:    "public-address",
		UnitInfo: &params.UnitInfo{PublicAddress: "public-address-1"},
		Expected: query.NewString("public-address-1"),
	}, {
		Field:    "private-address",
		UnitInfo: &params.UnitInfo{PrivateAddress: "private-address-1"},
		Expected: query.NewString("private-address-1"),
	}, {
		Field:    "machine-id",
		UnitInfo: &params.UnitInfo{MachineId: "machine-id-1"},
		Expected: query.NewString("machine-id-1"),
	}, {
		Field:    "principal",
		UnitInfo: &params.UnitInfo{Principal: "principal-1"},
		Expected: query.NewString("principal-1"),
	}, {
		Field:    "subordinate",
		UnitInfo: &params.UnitInfo{Subordinate: true},
		Expected: query.NewBool(true),
	}, {
		Field: "workload-status",
		UnitInfo: &params.UnitInfo{WorkloadStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}, {
		Field: "workload-message",
		UnitInfo: &params.UnitInfo{WorkloadStatus: params.StatusInfo{
			Message: "unit is ready",
		}},
		Expected: query.NewString("unit is ready"),
	}, {
		Field: "agent-status",
		UnitInfo: &params.UnitInfo{AgentStatus: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := UnitScope{
			ctx:      MakeScopeContext(),
			UnitInfo: test.UnitInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.Expected)
	}
}

func (s *unitScopeSuite) TestGetIdentValueError(c *tc.C) {
	tests := []struct {
		Field    string
		UnitInfo *params.UnitInfo
		Err      string
	}{{
		Field:    "bad",
		UnitInfo: &params.UnitInfo{},
		Err:      `"bad" on UnitInfo.*`,
	}, {
		Field:    "application",
		UnitInfo: nil,
		Err:      "internal error: UnitInfo is missing",
	}}
	for _, test := range tests {
		scope := UnitScope{
			ctx:      MakeScopeContext(),
			UnitInfo: test.UnitInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, tc.ErrorMatches, test.Err)
		c.Assert(result, tc.IsNil)
	}
}
