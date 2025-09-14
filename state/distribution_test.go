// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type InstanceDistributorSuite struct {
	ConnSuite
	distributor mockInstanceDistributor
	wordpress   *state.Application
	machines    []*state.Machine
	hwChar      *instance.HardwareCharacteristics
}

func TestInstanceDistributorSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &InstanceDistributorSuite{})
}

type mockInstanceDistributor struct {
	candidates        []instance.Id
	distributionGroup []instance.Id
	result            []instance.Id
	err               error
}

func (p *mockInstanceDistributor) DistributeInstances(
	ctx context.ProviderCallContext, candidates, distributionGroup []instance.Id, limitZones []string,
) ([]instance.Id, error) {
	p.candidates = candidates
	p.distributionGroup = distributionGroup
	result := p.result
	if result == nil {
		result = candidates
	}
	return result, p.err
}

func (s *InstanceDistributorSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	s.distributor = mockInstanceDistributor{}
	s.policy.GetInstanceDistributor = func() (context.Distributor, error) {
		return &s.distributor, nil
	}

	a := arch.DefaultArchitecture
	s.hwChar = &instance.HardwareCharacteristics{
		Arch: &a,
	}

	s.wordpress = s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	s.machines = make([]*state.Machine, 3)
	for i := range s.machines {
		m, err := s.State.AddOneMachine(state.MachineTemplate{
			Base:        state.UbuntuBase("12.10"),
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: constraints.MustParse("arch=amd64"),
		})
		c.Assert(err, tc.ErrorIsNil)

		hwChar := *s.hwChar
		if i <= 1 {
			az := "az1"
			hwChar.AvailabilityZone = &az
		}

		instId := instance.Id(fmt.Sprintf("i-blah-%d", i))
		err = m.SetProvisioned(instId, "", "fake-nonce", &hwChar)
		c.Assert(err, tc.ErrorIsNil)

		s.machines[i] = m
	}
}

func (s *InstanceDistributorSuite) setupScenario(c *tc.C) {
	// Assign a unit so we have a non-empty distribution group, and
	// provision all instances so we have candidates.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machines[0])
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstances(c *tc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.distributor.candidates, tc.SameContents, []instance.Id{"i-blah-1", "i-blah-2"})
	c.Assert(s.distributor.distributionGroup, tc.SameContents, []instance.Id{"i-blah-0"})
	s.distributor.result = []instance.Id{}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesInvalidInstances(c *tc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	s.distributor.result = []instance.Id{"notthere"}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: invalid instance returned: notthere`)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoEmptyMachines(c *tc.C) {
	for range s.machines {
		// Assign a unit so we have a non-empty distribution group.
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		_, err = unit.AssignToCleanMachine()
		c.Assert(err, tc.ErrorIsNil)
	}

	// InstanceDistributor is not called if there are no empty instances.
	s.distributor.err = fmt.Errorf("no assignment for you")
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, eligibleMachinesInUse)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesErrors(c *tc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that assignment fails when DistributeInstances returns an error.
	s.distributor.err = fmt.Errorf("no assignment for you")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, ".*no assignment for you")
	_, err = unit.AssignToCleanEmptyMachine()
	c.Assert(err, tc.ErrorMatches, ".*no assignment for you")
	// If the policy's InstanceDistributor method fails, that will be returned first.
	s.policy.GetInstanceDistributor = func() (context.Distributor, error) {
		return nil, fmt.Errorf("incapable of InstanceDistributor")
	}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, ".*incapable of InstanceDistributor")
}

func (s *InstanceDistributorSuite) TestDistributeInstancesDistributionGroup(c *tc.C) {
	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit0.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)

	// Distribution group is not empty, because the machine assigned.
	unit1, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit1.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesEmptyDistributionGroup(c *tc.C) {
	s.distributor.err = fmt.Errorf("no assignment for you")

	// InstanceDistributor is not called if the distribution group is empty.
	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit0.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesEmptyDistributionGroupAfterAssignWithNonProvision(c *tc.C) {
	s.distributor.err = fmt.Errorf("no assignment for you")

	// InstanceDistributor is not called if the distribution group is empty.
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Jobs:                    []state.MachineJob{state.JobHostUnits},
		Constraints:             constraints.MustParse("arch=amd64"),
		HardwareCharacteristics: *s.hwChar,
	})
	c.Assert(err, tc.ErrorIsNil)

	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit0.AssignToMachine(m)
	c.Assert(err, tc.ErrorIsNil)

	// Distribution group is still empty, because the machine assigned to has
	// not been provisioned.
	unit1, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit1.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestInstanceDistributorUnimplemented(c *tc.C) {
	s.setupScenario(c)

	var distributorErr error
	s.policy.GetInstanceDistributor = func() (context.Distributor, error) {
		return nil, distributorErr
	}
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: policy returned nil instance distributor without an error`)

	distributorErr = errors.NotImplementedf("InstanceDistributor")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoPolicy(c *tc.C) {
	s.policy.GetInstanceDistributor = func() (context.Distributor, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesWithZoneConstraints(c *tc.C) {
	err := s.wordpress.SetConstraints(constraints.MustParse("zones=az1"))
	c.Assert(err, tc.ErrorIsNil)

	// Initial unit, assigned to machine 0, to get a distribution group.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit.AssignToMachine(s.machines[0])
	c.Assert(err, tc.ErrorIsNil)

	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Only machine 1 is empty, and in the desired AZ.
	s.distributor.result = []instance.Id{"i-blah-1"}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, tc.ErrorIsNil)

	// Machine 2 filtered by zone constraint.
	c.Check(s.distributor.candidates, tc.SameContents, []instance.Id{"i-blah-1"})
	c.Check(s.distributor.distributionGroup, tc.SameContents, []instance.Id{"i-blah-0"})
}

type ApplicationMachinesSuite struct {
	ConnSuite
	wordpress *state.Application
	mysql     *state.Application
	machines  []*state.Machine
}

func TestApplicationMachinesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ApplicationMachinesSuite{})
}

func (s *ApplicationMachinesSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	s.wordpress = s.AddTestingApplication(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
	)
	s.mysql = s.AddTestingApplication(
		c,
		"mysql",
		s.AddTestingCharm(c, "mysql"),
	)

	s.machines = make([]*state.Machine, 5)
	for i := range s.machines {
		var err error
		s.machines[i], err = s.State.AddOneMachine(state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	for _, i := range []int{0, 1, 4} {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, tc.ErrorIsNil)
	}
	for _, i := range []int{2, 3} {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *ApplicationMachinesSuite) TestApplicationMachines(c *tc.C) {
	machines, err := state.ApplicationMachines(s.State, "mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.DeepEquals, []string{"2", "3"})

	machines, err = state.ApplicationMachines(s.State, "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.DeepEquals, []string{"0", "1", "4"})

	machines, err = state.ApplicationMachines(s.State, "fred")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(machines), tc.Equals, 0)
}
