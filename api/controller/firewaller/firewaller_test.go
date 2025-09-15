// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// NOTE: This suite is intended for embedding into other suites,
// so common code can be reused. Do not add test cases to it,
// otherwise they'll be run by each other suite that embeds it.
type firewallerSuite struct {
	jujutesting.JujuConnSuite

	st          api.Connection
	machines    []*state.Machine
	application *state.Application
	charm       *state.Charm
	units       []*state.Unit
	relations   []*state.Relation

	firewaller *firewaller.Client
}

func TestFirewallerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &firewallerSuite{})
}

func (s *firewallerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests. The first one is a manager node.
	s.machines = make([]*state.Machine, 3)
	s.units = make([]*state.Unit, 3)

	var err error
	s.machines[0], err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[0].SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[0].SetProvisioned("i-manager", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machines[0].Tag(), password, "fake_nonce")
	c.Assert(s.st, tc.NotNil)

	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	for i := 1; i <= 2; i++ {
		s.machines[i], err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Check(err, tc.ErrorIsNil)
	}
	// Create an application and three units for these machines.
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.application = s.AddTestingApplication(c, "wordpress", s.charm)
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		s.units[i], err = s.application.AddUnit(state.AddUnitParams{})
		c.Check(err, tc.ErrorIsNil)
		err = s.units[i].AssignToMachine(s.machines[i])
		c.Check(err, tc.ErrorIsNil)
	}

	// Create a relation.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)

	s.relations = make([]*state.Relation, 1)
	s.relations[0], err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Create the firewaller API facade.
	firewallerClient, err := firewaller.NewClient(s.st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(firewallerClient, tc.NotNil)
	s.firewaller = firewallerClient
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *firewallerSuite) TestModelFirewallRules(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ModelFirewallRules")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.IngressRulesResult{})
		*(result.(*params.IngressRulesResult)) = params.IngressRulesResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.ModelFirewallRules()
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchModelFirewallRules(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchModelFirewallRules")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchModelFirewallRules()
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchEgressAddressesForRelation(c *tc.C) {
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchEgressAddressesForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchEgressAddressesForRelation(relationTag)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchInressAddressesForRelation(c *tc.C) {
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchIngressAddressesForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchIngressAddressesForRelation(relationTag)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestControllerAPIInfoForModel(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ControllerAPIInfoForModels")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: coretesting.ModelTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.ControllerAPIInfoForModel(coretesting.ModelTag.Id())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestMacaroonForRelation(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "MacaroonForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: names.NewRelationTag("mysql:db wordpress:db").String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.MacaroonResults{})
		*(result.(*params.MacaroonResults)) = params.MacaroonResults{
			Results: []params.MacaroonResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.MacaroonForRelation("mysql:db wordpress:db")
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestSetRelationStatus(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetRelationsStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{Entities: []params.EntityStatusArgs{
			{
				Tag:    names.NewRelationTag("mysql:db wordpress:db").String(),
				Status: "suspended",
				Info:   "a message",
			}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	err = client.SetRelationStatus("mysql:db wordpress:db", relation.Suspended, "a message")
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestAllSpaceInfos(c *tc.C) {
	expSpaceInfos := network.SpaceInfos{
		{
			ID:         "42",
			Name:       "questions-about-the-universe",
			ProviderId: "provider-id-2",
			Subnets: []network.SubnetInfo{
				{
					ID:                "13",
					CIDR:              "1.168.1.0/24",
					ProviderId:        "provider-subnet-id-1",
					ProviderSpaceId:   "provider-space-id-1",
					ProviderNetworkId: "provider-network-id-1",
					VLANTag:           42,
					AvailabilityZones: []string{"az1", "az2"},
					SpaceID:           "42",
					SpaceName:         "questions-about-the-universe",
					FanInfo: &network.FanCIDRs{
						FanLocalUnderlay: "192.168.0.0/16",
						FanOverlay:       "1.0.0.0/8",
					},
					IsPublic: true,
				},
			},
		},
	}

	var callCount int
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Firewaller")
			c.Check(version, tc.Equals, 6)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "SpaceInfos")
			c.Assert(arg, tc.DeepEquals, params.SpaceInfosParams{})
			c.Assert(result, tc.FitsTypeOf, &params.SpaceInfos{})
			*(result.(*params.SpaceInfos)) = params.FromNetworkSpaceInfos(expSpaceInfos)
			callCount++
			return nil
		}),
		BestVersion: 6,
	}

	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	got, err := client.AllSpaceInfos()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(got, tc.DeepEquals, expSpaceInfos)
}
