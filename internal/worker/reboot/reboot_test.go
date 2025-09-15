// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	apireboot "github.com/juju/juju/api/agent/reboot"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/reboot"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type rebootSuite struct {
	jujutesting.JujuConnSuite

	machine     *state.Machine
	stateAPI    api.Connection
	rebootState apireboot.State

	ct            *state.Machine
	ctRebootState apireboot.State

	clock clock.Clock
}

func TestRebootSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &rebootSuite{})
}

func (s *rebootSuite) SetUpTest(c *tc.C) {
	var err error
	template := state.MachineTemplate{
		Base: state.DefaultLTSBase(),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	s.JujuConnSuite.SetUpTest(c)

	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c)
	s.rebootState, err = apireboot.NewFromConnection(s.stateAPI)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.rebootState, tc.NotNil)

	//Add container
	s.ct, err = s.State.AddMachineInsideMachine(template, s.machine.Id(), instance.KVM)
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = s.ct.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)
	err = s.ct.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Open api as container
	ctState := s.OpenAPIAsMachine(c, s.ct.Tag(), password, "fake_nonce")
	s.ctRebootState, err = apireboot.NewFromConnection(ctState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.ctRebootState, tc.NotNil)

	s.clock = &fakeClock{delay: time.Millisecond}
}

func (s *rebootSuite) TearDownTest(c *tc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *rebootSuite) TestStartStop(c *tc.C) {
	worker, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), &fakemachinelock{}, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), tc.IsNil)
}

func (s *rebootSuite) TestWorkerCatchesRebootEvent(c *tc.C) {
	wrk, err := reboot.NewReboot(s.rebootState, s.AgentConfigForTag(c, s.machine.Tag()), &fakemachinelock{}, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wrk.Wait(), tc.Equals, worker.ErrRebootMachine)
	// The flag is cleared.
	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsFalse)

}

func (s *rebootSuite) TestContainerCatchesParentFlag(c *tc.C) {
	wrk, err := reboot.NewReboot(s.ctRebootState, s.AgentConfigForTag(c, s.ct.Tag()), &fakemachinelock{}, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	err = s.rebootState.RequestReboot()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wrk.Wait(), tc.Equals, worker.ErrShutdownMachine)
}

type fakeClock struct {
	clock.Clock
	delay time.Duration
}

func (f *fakeClock) After(time.Duration) <-chan time.Time {
	return time.After(f.delay)
}

type fakemachinelock struct {
	mu sync.Mutex
}

func (f *fakemachinelock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

func (f *fakemachinelock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}
