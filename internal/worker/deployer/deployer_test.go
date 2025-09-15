// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"
	"strings"
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/deployer"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type deployerSuite struct {
	jujutesting.JujuConnSuite

	machine       *state.Machine
	stateAPI      api.Connection
	deployerState *apideployer.State
}

func TestDeployerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &deployerSuite{})
}

func (s *deployerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the deployer facade.
	s.deployerState = apideployer.NewState(s.stateAPI)
	c.Assert(s.deployerState, tc.NotNil)
	loggo.GetLogger("test.deployer").SetLogLevel(loggo.TRACE)
}

func (s *deployerSuite) makeDeployerAndContext(c *tc.C) (worker.Worker, deployer.Context) {
	// Create a deployer acting on behalf of the machine.
	ctx := &fakeContext{
		config:   agentConfig(s.machine.Tag(), c.MkDir(), c.MkDir()),
		deployed: set.NewStrings(),
	}
	api := deployer.MakeAPIShim(s.deployerState)
	deployer, err := deployer.NewDeployer(api, loggo.GetLogger("test.deployer"), ctx)
	c.Assert(err, tc.ErrorIsNil)
	return deployer, ctx
}

func (s *deployerSuite) TestDeployRecallRemovePrincipals(c *tc.C) {
	// Create a machine, and a couple of units.
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	u1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)

	// Assign one unit, and wait for it to be deployed.
	err = u0.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name()))

	// Assign another unit, and wait for that to be deployed.
	err = u1.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dying, and check no change.
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u1.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	err = u1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx, u0.Name(), u1.Name()))

	// Cause a unit to become Dead, and check that it is both recalled and
	// removed from state.
	err = u0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isRemoved(s.State, u0.Name()))
	s.waitFor(c, isDeployed(ctx, u1.Name()))

	// Remove the Dying unit from the machine, and check that it is recalled...
	err = u1.UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx))

	// ...and that the deployer, no longer bearing any responsibility for the
	// Dying unit, does nothing further to it.
	err = u1.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(u1.Life(), tc.Equals, state.Dying)
}

func (s *deployerSuite) TestInitialStatusMessages(c *tc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	dep, _ := s.makeDeployerAndContext(c)
	defer stop(c, dep)
	err = u0.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, unitStatus(u0, status.StatusInfo{
		Status:  status.Waiting,
		Message: "installing agent",
	}))
}

func (s *deployerSuite) TestRemoveNonAlivePrincipals(c *tc.C) {
	// Create an application, and a couple of units.
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	u1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Assign the units to the machine, and set them to Dying/Dead.
	err = u0.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	err = u0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = u1.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	// note: this is not a sane state; for the unit to have a status it must
	// have been deployed. But it's instructive to check that the right thing
	// would happen if it were possible to have a dying unit in this situation.
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u1.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	err = u1.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// When the deployer is started, in each case (1) no unit agent is deployed
	// and (2) the non-Alive unit is been removed from state.
	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)
	s.waitFor(c, isRemoved(s.State, u0.Name()))
	s.waitFor(c, isRemoved(s.State, u1.Name()))
	s.waitFor(c, isDeployed(ctx))
}

func (s *deployerSuite) prepareSubordinates(c *tc.C) (*state.Unit, []*state.RelationUnit) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u.AssignToMachine(s.machine)
	c.Assert(err, tc.ErrorIsNil)
	rus := []*state.RelationUnit{}
	logging := s.AddTestingCharm(c, "logging")
	for _, name := range []string{"subsvc0", "subsvc1"} {
		s.AddTestingApplication(c, name, logging)
		eps, err := s.State.InferEndpoints("wordpress", name)
		c.Assert(err, tc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, tc.ErrorIsNil)
		ru, err := rel.Unit(u)
		c.Assert(err, tc.ErrorIsNil)
		rus = append(rus, ru)
	}
	return u, rus
}

func (s *deployerSuite) TestDeployRecallRemoveSubordinates(c *tc.C) {
	// Create a deployer acting on behalf of the principal.
	u, rus := s.prepareSubordinates(c)
	dep, ctx := s.makeDeployerAndContext(c)
	defer stop(c, dep)

	// Add a subordinate, and wait for it to be deployed.
	err := rus[0].EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	sub0, err := s.State.Unit("subsvc0/0")
	c.Assert(err, tc.ErrorIsNil)
	// Make sure the principal is deployed first, then the subordinate
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name()))

	// And another.
	err = rus[1].EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	sub1, err := s.State.Unit("subsvc1/0")
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name(), sub1.Name()))

	// Set one to Dying; check nothing happens.
	err = sub1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRemoved(s.State, sub1.Name())(c), tc.IsFalse)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub0.Name(), sub1.Name()))

	// Set the other to Dead; check it's recalled and removed.
	err = sub0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	s.waitFor(c, isDeployed(ctx, u.Name(), sub1.Name()))
	s.waitFor(c, isRemoved(s.State, sub0.Name()))
}

func (s *deployerSuite) TestNonAliveSubordinates(c *tc.C) {
	// Add two subordinate units and set them to Dead/Dying respectively.
	_, rus := s.prepareSubordinates(c)
	err := rus[0].EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	sub0, err := s.State.Unit("subsvc0/0")
	c.Assert(err, tc.ErrorIsNil)
	err = sub0.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = rus[1].EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	sub1, err := s.State.Unit("subsvc1/0")
	c.Assert(err, tc.ErrorIsNil)
	err = sub1.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// When we start a new deployer, neither unit will be deployed and
	// both will be removed.
	dep, _ := s.makeDeployerAndContext(c)
	defer stop(c, dep)
	s.waitFor(c, isRemoved(s.State, sub0.Name()))
	s.waitFor(c, isRemoved(s.State, sub1.Name()))
}

func (s *deployerSuite) waitFor(c *tc.C, t func(c *tc.C) bool) {
	if t(c) {
		return
	}
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout")
		case <-time.After(coretesting.ShortWait):
			if t(c) {
				return
			}
		}
	}
}

func isDeployed(ctx deployer.Context, expected ...string) func(*tc.C) bool {
	return func(c *tc.C) bool {
		sort.Strings(expected)
		current, err := ctx.DeployedUnits()
		c.Assert(err, tc.ErrorIsNil)
		sort.Strings(current)
		return strings.Join(expected, ":") == strings.Join(current, ":")
	}
}

func isRemoved(st *state.State, name string) func(*tc.C) bool {
	return func(c *tc.C) bool {
		_, err := st.Unit(name)
		if errors.IsNotFound(err) {
			return true
		}
		c.Assert(err, tc.ErrorIsNil)
		return false
	}
}

func unitStatus(u *state.Unit, statusInfo status.StatusInfo) func(*tc.C) bool {
	return func(c *tc.C) bool {
		sInfo, err := u.Status()
		c.Assert(err, tc.ErrorIsNil)
		return sInfo.Status == statusInfo.Status && sInfo.Message == statusInfo.Message
	}
}

func stop(c *tc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), tc.IsNil)
}

type fakeContext struct {
	deployer.Context

	config agent.Config

	deployed   set.Strings
	deployedMu sync.Mutex
}

func (c *fakeContext) Kill() {
}

func (c *fakeContext) Wait() error {
	return nil
}

func (c *fakeContext) DeployUnit(unitName, initialPassword string) error {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	// Doesn't check for existence.
	c.deployed.Add(unitName)
	return nil
}

func (c *fakeContext) RecallUnit(unitName string) error {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	c.deployed.Remove(unitName)
	return nil
}

func (c *fakeContext) DeployedUnits() ([]string, error) {
	c.deployedMu.Lock()
	defer c.deployedMu.Unlock()

	return c.deployed.SortedValues(), nil
}

func (c *fakeContext) AgentConfig() agent.Config {
	return c.config
}
