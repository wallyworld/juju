// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/deployer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type mockLeadershipRevoker struct {
	revoked set.Strings
}

func (s *mockLeadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
	return nil
}

type deployerSuite struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer

	service0     *state.Application
	service1     *state.Application
	machine0     *state.Machine
	machine1     *state.Machine
	principal0   *state.Unit
	principal1   *state.Unit
	subordinate0 *state.Unit

	resources *common.Resources
	deployer  *deployer.DeployerAPI
	revoker   *mockLeadershipRevoker
}

func TestDeployerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &deployerSuite{})
}

func (s *deployerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// The two known machines now contain the following units:
	// machine 0 (not authorized): mysql/1 (principal1)
	// machine 1 (authorized): mysql/0 (principal0), logging/0 (subordinate0)

	var err error
	s.machine0, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.machine1, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	s.service0 = s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))

	s.service1 = s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	s.principal0, err = s.service0.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.principal0.AssignToMachine(s.machine1)
	c.Assert(err, tc.ErrorIsNil)

	s.principal1, err = s.service0.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.principal1.AssignToMachine(s.machine0)
	c.Assert(err, tc.ErrorIsNil)

	relUnit0, err := rel.Unit(s.principal0)
	c.Assert(err, tc.ErrorIsNil)
	err = relUnit0.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.subordinate0, err = s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.revoker = &mockLeadershipRevoker{revoked: set.NewStrings()}
	// Create a deployer API for machine 1.
	deployer, err := deployer.NewDeployerAPI(
		facadetest.Context{
			Auth_:              s.authorizer,
			Resources_:         s.resources,
			State_:             s.State,
			StatePool_:         s.StatePool,
			LeadershipRevoker_: s.revoker,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.deployer = deployer
}

func (s *deployerSuite) TestDeployerFailsWithNonMachineAgentUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = s.AdminUserTag(c)
	aDeployer, err := deployer.NewDeployerAPI(
		facadetest.Context{
			Auth_:              anAuthorizer,
			LeadershipRevoker_: s.revoker,
			Resources_:         s.resources,
			State_:             s.State,
		},
	)
	c.Assert(err, tc.NotNil)
	c.Assert(aDeployer, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *deployerSuite) TestWatchUnits(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.deployer.WatchUnits(args)
	c.Assert(err, tc.ErrorIsNil)
	sort.Strings(result.Results[0].Changes)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Changes: []string{"logging/0", "mysql/0"}, StringsWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), tc.Equals, 1)
	c.Assert(result.Results[0].StringsWatcherId, tc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *deployerSuite) TestSetPasswords(c *tc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "unit-mysql-0", Password: "xxx-12345678901234567890"},
			{Tag: "unit-mysql-1", Password: "yyy-12345678901234567890"},
			{Tag: "unit-logging-0", Password: "zzz-12345678901234567890"},
			{Tag: "unit-fake-42", Password: "abc-12345678901234567890"},
		},
	}
	results, err := s.deployer.SetPasswords(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.principal0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	changed := s.principal0.PasswordValid("xxx-12345678901234567890")
	c.Assert(changed, tc.IsTrue)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	changed = s.subordinate0.PasswordValid("zzz-12345678901234567890")
	c.Assert(changed, tc.IsTrue)

	// Remove the subordinate and make sure it's detected.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	results, err = s.deployer.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "unit-logging-0", Password: "blah-12345678901234567890"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *deployerSuite) TestLife(c *tc.C) {
	err := s.subordinate0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), tc.Equals, state.Dead)
	err = s.principal0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.principal0.Life(), tc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Life(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	result, err = s.deployer.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-logging-0"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *deployerSuite) TestRemove(c *tc.C) {
	c.Assert(s.principal0.Life(), tc.Equals, state.Alive)
	c.Assert(s.subordinate0.Life(), tc.Equals, state.Alive)

	// Try removing alive units - should fail.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Remove(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `cannot remove entity "unit-mysql-0": still alive`}},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: `cannot remove entity "unit-logging-0": still alive`}},
			{apiservertesting.ErrUnauthorized},
		},
	})
	c.Assert(s.revoker.revoked.IsEmpty(), tc.IsTrue)

	err = s.principal0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.principal0.Life(), tc.Equals, state.Alive)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), tc.Equals, state.Alive)

	// Now make the subordinate dead and try again.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), tc.Equals, state.Dead)

	args = params.Entities{
		Entities: []params.Entity{{Tag: "unit-logging-0"}},
	}
	result, err = s.deployer.Remove(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(s.revoker.revoked.Contains("logging/0"), tc.IsTrue)

	err = s.subordinate0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Make sure the subordinate is detected as removed.
	result, err = s.deployer.Remove(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{apiservertesting.ErrUnauthorized}},
	})
}

func (s *deployerSuite) TestConnectionInfo(c *tc.C) {
	err := s.machine0.SetProviderAddresses(network.NewSpaceAddress("0.1.2.3", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)))
	c.Assert(err, tc.ErrorIsNil)

	// Default host port scope is public, so change the cloud-local one
	hostPorts := network.NewSpaceHostPorts(1234, "0.1.2.3", "1.2.3.4")
	hostPorts[1].Scope = network.ScopeCloudLocal

	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts})
	c.Assert(err, tc.ErrorIsNil)

	expected := params.DeployerConnectionValues{
		APIAddresses: []string{"1.2.3.4:1234", "0.1.2.3:1234"},
	}

	result, err := s.deployer.ConnectionInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *deployerSuite) TestSetStatus(c *tc.C) {
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "unit-mysql-0", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "unit-mysql-1", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "unit-fake-42", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
		},
	}
	results, err := s.deployer.SetStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
	sInfo, err := s.principal0.Status()
	c.Assert(err, tc.ErrorIsNil)
	sInfo.Since = nil
	c.Assert(sInfo, tc.DeepEquals, status.StatusInfo{
		Status:  status.Blocked,
		Message: "waiting",
		Data:    map[string]interface{}{"foo": "bar"},
	})
}
