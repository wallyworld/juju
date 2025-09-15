// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/machine"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type machinerSuite struct {
	commonSuite

	resources *common.Resources
	machiner  *machine.MachinerAPI
}

func TestMachinerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &machinerSuite{})
}

func (s *machinerSuite) SetUpTest(c *tc.C) {
	s.commonSuite.SetUpTest(c)

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	// Create a machiner API for machine 1.
	machiner, err := machine.NewMachinerAPIForState(
		systemState,
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.machiner = machiner
}

func (s *machinerSuite) TestMachinerFailsWithNonMachineAgentUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewUnitTag("ubuntu/1")
	systemState, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	aMachiner, err := machine.NewMachinerAPIForState(
		systemState,
		s.State, s.resources, anAuthorizer)
	c.Assert(err, tc.NotNil)
	c.Assert(aMachiner, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *machinerSuite) TestSetStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machine0.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machine1.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: status.Error.String(), Info: "not really"},
			{Tag: "machine-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
		}}
	result, err := s.machiner.SetStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 0 - no change.
	statusInfo, err := s.machine0.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Started)
	c.Assert(statusInfo.Message, tc.Equals, "blah")
	// ...machine 1 is fine though.
	statusInfo, err = s.machine1.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, status.Error)
	c.Assert(statusInfo.Message, tc.Equals, "not really")
}

func (s *machinerSuite) TestLife(c *tc.C) {
	err := s.machine1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.Life(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestEnsureDead(c *tc.C) {
	c.Assert(s.machine0.Life(), tc.Equals, state.Alive)
	c.Assert(s.machine1.Life(), tc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.machiner.EnsureDead(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine0.Life(), tc.Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)

	// Try it again on a Dead machine; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	}
	result, err = s.machiner.EnsureDead(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Life(), tc.Equals, state.Dead)
}

func (s *machinerSuite) TestSetMachineAddresses(c *tc.C) {
	c.Assert(s.machine0.Addresses(), tc.HasLen, 0)
	c.Assert(s.machine1.Addresses(), tc.HasLen, 0)

	addresses := []network.MachineAddress{
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("8.8.8.8"),
	}
	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromMachineAddresses(addresses...)},
		{Tag: "machine-0", Addresses: params.FromMachineAddresses(addresses...)},
		{Tag: "machine-42", Addresses: params.FromMachineAddresses(addresses...)},
	}}

	result, err := s.machiner.SetMachineAddresses(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := network.NewSpaceAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(s.machine1.MachineAddresses(), tc.DeepEquals, expectedAddresses)
	err = s.machine0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine0.MachineAddresses(), tc.HasLen, 0)
}

func (s *machinerSuite) TestSetEmptyMachineAddresses(c *tc.C) {
	// Set some addresses so we can ensure they are removed.
	addresses := []network.MachineAddress{
		network.NewMachineAddress("127.0.0.1"),
		network.NewMachineAddress("8.8.8.8"),
	}
	args := params.SetMachinesAddresses{MachineAddresses: []params.MachineAddresses{
		{Tag: "machine-1", Addresses: params.FromMachineAddresses(addresses...)},
	}}
	result, err := s.machiner.SetMachineAddresses(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), tc.HasLen, 2)

	args.MachineAddresses[0].Addresses = nil
	result, err = s.machiner.SetMachineAddresses(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.MachineAddresses(), tc.HasLen, 0)
}

func (s *machinerSuite) TestJobs(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}

	result, err := s.machiner.Jobs(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.JobsResults{
		Results: []params.JobsResult{
			{Jobs: []model.MachineJob{model.JobHostUnits}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *machinerSuite) TestWatch(c *tc.C) {
	loggo.GetLogger("juju.state.pool.txnwatcher").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.watcher").SetLogLevel(loggo.TRACE)

	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	// We just set up the machiner, make sure there aren't pending events
	// before we set up the watcher.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	result, err := s.machiner.Watch(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), tc.Equals, 1)
	c.Assert(result.Results[0].NotifyWatcherId, tc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *tc.C) {
	args := params.RecordAgentStartInformationArgs{Args: []params.RecordAgentStartInformationArg{
		{Tag: "machine-1", Hostname: "thundering-herds"},
		{Tag: "machine-0", Hostname: "eldritch-octopii"},
		{Tag: "machine-42", Hostname: "missing-gem"},
	}}

	result, err := s.machiner.RecordAgentStartInformation(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.machine1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine1.Hostname(), tc.Equals, "thundering-herds", tc.Commentf("expected the machine hostname to be updated"))
}
