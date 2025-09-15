// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
)

// TODO(mjs) - these tests are too tightly coupled to the
// implementation. They needn't be internal tests.

type UpgradeSuite struct {
	statetesting.StateSuite

	oldVersion      version.Binary
	logWriter       loggo.TestWriter
	connectionDead  bool
	preUpgradeError bool
}

func TestUpgradeSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &UpgradeSuite{})
}

const fails = true

const succeeds = false

func (s *UpgradeSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)

	s.preUpgradeError = false
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	// XXXX
	// DumpTestLogsAfter(time.Minute, c, s)

	s.oldVersion = coretesting.CurrentVersion()
	s.oldVersion.Major = 1
	s.oldVersion.Minor = 16

	// Don't wait so long in tests.
	s.PatchValue(&UpgradeStartTimeoutController, time.Millisecond*50)

	// Allow tests to make the API connection appear to be dead.
	s.connectionDead = false
	s.PatchValue(&agenterrors.ConnectionIsDead, func(loggo.Logger, agenterrors.Breakable) bool {
		return s.connectionDead
	})
}

func (s *UpgradeSuite) captureLogs(c *tc.C) {
	c.Assert(loggo.RegisterWriter("upgrade-tests", &s.logWriter), tc.IsNil)
	s.AddCleanup(func(*tc.C) {
		loggo.RemoveWriter("upgrade-tests")
		s.logWriter.Clear()
	})
}

func (s *UpgradeSuite) countUpgradeAttempts(upgradeErr error) *int {
	count := 0
	s.PatchValue(&PerformUpgrade, func(version.Number, []upgrades.Target, upgrades.Context) error {
		count++
		return upgradeErr
	})
	return &count
}

func (s *UpgradeSuite) TestNewChannelWhenNoUpgradeRequired(c *tc.C) {
	// Set the agent's upgradedToVersion to version.Current,
	// to simulate the upgrade steps having been run already.
	initialVersion := jujuversion.Current
	config := NewFakeConfigSetter(names.NewMachineTag("0"), initialVersion)

	lock := NewLock(config)

	// Upgrade steps have already been run.
	c.Assert(lock.IsUnlocked(), tc.IsTrue)
}

func (s *UpgradeSuite) TestNewChannelWhenUpgradeRequired(c *tc.C) {
	// Set the agent's upgradedToVersion so that upgrade steps are required.
	initialVersion := version.MustParse("1.16.0")
	config := NewFakeConfigSetter(names.NewMachineTag("0"), initialVersion)

	lock := NewLock(config)

	c.Assert(lock.IsUnlocked(), tc.IsFalse)
	// The agent's version should NOT have been updated.
	c.Assert(config.Version, tc.Equals, initialVersion)
}

func (s *UpgradeSuite) TestNoUpgradeNecessary(c *tc.C) {
	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)
	s.oldVersion.Number = jujuversion.Current // nothing to do

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, tc.IsNil)
	c.Check(*attemptsP, tc.Equals, 0)
	c.Check(config.Version, tc.Equals, jujuversion.Current)
	c.Check(doneLock.IsUnlocked(), tc.IsTrue)
}

func (s *UpgradeSuite) TestNoUpgradeNecessaryIgnoresBuildNumbers(c *tc.C) {
	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)
	s.oldVersion.Number = jujuversion.Current
	s.oldVersion.Build = 1 // Ensure there's a build number mismatch.

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, tc.IsNil)
	c.Check(*attemptsP, tc.Equals, 0)
	c.Check(config.Version, tc.Equals, s.oldVersion.Number)
	c.Check(doneLock.IsUnlocked(), tc.IsTrue)
}

func (s *UpgradeSuite) TestUpgradeStepsFailure(c *tc.C) {
	// This test checks what happens when every upgrade attempt fails.
	// A number of retries should be observed and the agent should end
	// up in a state where it is is still running but is reporting an
	// error and the upgrade is not flagged as having completed (which
	// prevents most of the agent's workers from running and keeps the
	// API in restricted mode).

	attemptsP := s.countUpgradeAttempts(errors.New("boom"))
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	// The worker shouldn't return an error so that the worker and
	// agent keep running.
	c.Check(workerErr, tc.IsNil)

	c.Check(*attemptsP, tc.Equals, maxUpgradeRetries)
	c.Check(config.Version, tc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(statusCalls, tc.DeepEquals,
		s.makeExpectedStatusCalls(maxUpgradeRetries-1, fails, "boom"))
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc),
		s.makeExpectedUpgradeLogs(maxUpgradeRetries-1, "hostMachine", fails, "boom"))
	c.Assert(doneLock.IsUnlocked(), tc.IsFalse)
}

func (s *UpgradeSuite) TestUpgradeStepsRetries(c *tc.C) {
	// This test checks what happens when the first upgrade attempt
	// fails but the following on succeeds. The final state should be
	// the same as a successful upgrade which worked first go.
	attempts := 0
	fail := true
	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		attempts++
		if fail {
			fail = false
			return errors.New("boom")
		} else {
			return nil
		}
	}
	s.PatchValue(&PerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, tc.IsNil)
	c.Check(attempts, tc.Equals, 2)
	c.Check(config.Version, tc.Equals, jujuversion.Current) // Upgrade finished
	c.Assert(statusCalls, tc.DeepEquals, s.makeExpectedStatusCalls(1, succeeds, "boom"))
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc),
		s.makeExpectedUpgradeLogs(1, "hostMachine", succeeds, "boom"))
	c.Check(doneLock.IsUnlocked(), tc.IsTrue)
}

func (s *UpgradeSuite) TestOtherUpgradeRunFailure(c *tc.C) {
	// This test checks what happens something other than the upgrade
	// steps themselves fails, ensuring the something is logged and
	// the agent status is updated.

	m := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	s.captureLogs(c)

	// Simulate the upgrade-database worker having run successfully.
	info, err := s.State.EnsureUpgradeInfo(m.Id(), s.oldVersion.Number, jujuversion.Current)
	c.Assert(err, tc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, tc.ErrorIsNil)

	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		// Violate the state-machine rules so that finaliseUpgrade() will fail.
		// Recreating the upgrade doc will put us into status "pending" without
		// any recorded controller ready/completed entries.
		if err := s.State.ClearUpgradeInfo(); err != nil {
			return err
		}
		info, err = s.State.EnsureUpgradeInfo(m.Id(), s.oldVersion.Number, jujuversion.Current)
		return err
	}
	s.PatchValue(&PerformUpgrade, fakePerformUpgrade)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)
	c.Check(workerErr, tc.IsNil)

	c.Check(config.Version, tc.Equals, jujuversion.Current) // Upgrade almost finished

	failReason := `upgrade done but failed to synchronise: cannot complete upgrade: upgrade has not yet run`
	c.Assert(statusCalls, tc.DeepEquals, s.makeExpectedStatusCalls(0, fails, failReason))
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc),
		s.makeExpectedUpgradeLogs(0, "databaseMaster", fails, failReason))
	c.Assert(doneLock.IsUnlocked(), tc.IsFalse)
}

func (s *UpgradeSuite) TestAPIConnectionFailure(c *tc.C) {
	// This test checks what happens when an upgrade fails because the
	// connection to mongo has gone away. This will happen when the
	// mongo master changes. In this case we want the upgrade worker
	// to return immediately without further retries. The error should
	// be returned by the worker so that the agent will restart.

	attemptsP := s.countUpgradeAttempts(errors.New("boom"))
	s.connectionDead = true // Make the connection to state appear to be dead
	s.captureLogs(c)

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, tc.ErrorMatches, "API connection lost during upgrade: boom")
	c.Check(*attemptsP, tc.Equals, 1)
	c.Check(config.Version, tc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(doneLock.IsUnlocked(), tc.IsFalse)
}

func (s *UpgradeSuite) TestAbortWhenOtherControllerDoesNotStartUpgrade(c *tc.C) {
	// This test checks when a controller is upgrading and one of
	// the other controllers doesn't signal it is ready in time.

	err := s.State.SetModelAgentVersion(jujuversion.Current, nil, false)
	c.Assert(err, tc.ErrorIsNil)

	s.create3Controllers(c)
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, tc.IsNil)
	c.Check(*attemptsP, tc.Equals, 0)
	c.Check(config.Version, tc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	c.Assert(doneLock.IsUnlocked(), tc.IsFalse)

	// The environment agent-version should still be the new version.
	// It's up to the master to trigger the rollback.
	s.assertEnvironAgentVersion(c, jujuversion.Current)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	causeMsg := " timed out after 50ms"
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{
		{Level: loggo.INFO, Message: "waiting for other controllers to be ready for upgrade"},
		{Level: loggo.ERROR, Message: "aborted wait for other controllers: timed out after 50ms"},
		{Level: loggo.ERROR, Message: `upgrade from .+ to .+ for "machine-0" failed \(giving up\): ` +
			"aborted wait for other controllers:" + causeMsg},
	})
	c.Assert(statusCalls, tc.DeepEquals, []StatusCall{{
		status.Error,
		fmt.Sprintf(
			"upgrade to %s failed (giving up): aborted wait for other controllers:"+causeMsg,
			jujuversion.Current),
	}})
}

func (s *UpgradeSuite) TestSuccessLeadingController(c *tc.C) {
	// This test checks what happens when an upgrade works on the first
	// attempt, on the first controller to set the status to "running".
	info := s.checkSuccess(c, "databaseMaster", func(i *state.UpgradeInfo) {
		err := i.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, tc.ErrorIsNil)
	})
	c.Assert(info.Status(), tc.Equals, state.UpgradeRunning)
}

func (s *UpgradeSuite) TestSuccessFollowingController(c *tc.C) {
	// This test checks what happens when an upgrade works on the a controller
	// following a controller having already set the status to "running".
	s.checkSuccess(c, "controller", func(info *state.UpgradeInfo) {
		// Indicate that the master is done
		err := info.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, tc.ErrorIsNil)
		err = info.SetStatus(state.UpgradeRunning)
		c.Assert(err, tc.ErrorIsNil)
	})
}

func (s *UpgradeSuite) checkSuccess(c *tc.C, target string, mungeInfo func(*state.UpgradeInfo)) *state.UpgradeInfo {
	_, machineIdB, machineIdC := s.create3Controllers(c)

	// Indicate that machine B and C are ready to upgrade
	vPrevious := s.oldVersion.Number
	vNext := jujuversion.Current
	info, err := s.State.EnsureUpgradeInfo(machineIdB, vPrevious, vNext)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(machineIdC, vPrevious, vNext)
	c.Assert(err, tc.ErrorIsNil)

	mungeInfo(info)

	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, tc.IsNil)
	c.Check(*attemptsP, tc.Equals, 1)
	c.Check(config.Version, tc.Equals, jujuversion.Current) // Upgrade finished
	c.Assert(statusCalls, tc.DeepEquals, s.makeExpectedStatusCalls(0, succeeds, ""))
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc),
		s.makeExpectedUpgradeLogs(0, target, succeeds, ""))
	c.Check(doneLock.IsUnlocked(), tc.IsTrue)

	err = info.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.ControllersDone(), tc.DeepEquals, []string{"0"})
	return info
}

func (s *UpgradeSuite) TestJobsToTargets(c *tc.C) {
	c.Assert(upgradeTargets(false), tc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
	c.Assert(upgradeTargets(true), tc.SameContents, []upgrades.Target{upgrades.HostMachine, upgrades.Controller})
}

func (s *UpgradeSuite) TestPreUpgradeFail(c *tc.C) {
	s.preUpgradeError = true
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, tc.ErrorIsNil)
	c.Check(config.Version, tc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(doneLock.IsUnlocked(), tc.IsFalse)

	causeMessage := `machine 0 cannot be upgraded: preupgrade error`
	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(giving up\): %s`,
		s.oldVersion.Number, jujuversion.Current, causeMessage)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{
		{Level: loggo.INFO, Message: "checking that upgrade can proceed"},
		{Level: loggo.ERROR, Message: failMessage},
	})

	statusMessage := fmt.Sprintf(
		`upgrade to %s failed (giving up): %s`, jujuversion.Current, causeMessage)
	c.Assert(statusCalls, tc.DeepEquals, []StatusCall{{
		status.Error, statusMessage,
	}})
}

// Run just the upgradeSteps worker with a fake machine agent and
// fake agent config.
func (s *UpgradeSuite) runUpgradeWorker(c *tc.C, isController bool) (
	error, *fakeConfigSetter, []StatusCall, gate.Lock,
) {
	config := s.makeFakeConfig()
	agent := NewFakeAgent(config)
	doneLock := NewLock(config)
	machineStatus := &testStatusSetter{}
	testRetryStrategy := retry.CallArgs{
		Clock:    clock.WallClock,
		Delay:    time.Millisecond,
		Attempts: maxUpgradeRetries,
	}
	worker, err := NewWorker(
		doneLock,
		agent,
		nil,
		isController,
		s.openStateForUpgrade,
		s.preUpgradeSteps,
		testRetryStrategy,
		machineStatus,
		false,
	)
	c.Assert(err, tc.ErrorIsNil)
	return worker.Wait(), config, machineStatus.Calls, doneLock
}

func (s *UpgradeSuite) openStateForUpgrade() (*state.StatePool, error) {
	newPolicy := stateenvirons.GetNewPolicyFunc()
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.Model.ModelTag(),
		MongoSession:       s.State.MongoSession(),
		NewPolicy:          newPolicy,
	})
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *UpgradeSuite) preUpgradeSteps(_ *state.StatePool, _ agent.Config, _, _ bool) error {
	if s.preUpgradeError {
		return errors.New("preupgrade error")
	}
	return nil
}

func (s *UpgradeSuite) makeFakeConfig() *fakeConfigSetter {
	return NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
}

func (s *UpgradeSuite) create3Controllers(c *tc.C) (machineIdA, machineIdB, machineIdC string) {
	machine0 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	machineIdA = machine0.Id()

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(changes.Added), tc.Equals, 2)

	machineIdB = changes.Added[0]
	s.setMachineProvisioned(c, machineIdB)

	machineIdC = changes.Added[1]
	s.setMachineProvisioned(c, machineIdC)

	return
}

func (s *UpgradeSuite) setMachineProvisioned(c *tc.C, id string) {
	machine, err := s.State.Machine(id)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id(id+"-inst"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
}

const maxUpgradeRetries = 3

func (s *UpgradeSuite) makeExpectedStatusCalls(retryCount int, expectFail bool, failReason string) []StatusCall {
	calls := []StatusCall{{
		status.Started,
		fmt.Sprintf("upgrading to %s", jujuversion.Current),
	}}
	for i := 0; i < retryCount; i++ {
		calls = append(calls, StatusCall{
			status.Error,
			fmt.Sprintf("upgrade to %s failed (will retry): %s", jujuversion.Current, failReason),
		})
	}
	if expectFail {
		calls = append(calls, StatusCall{
			status.Error,
			fmt.Sprintf("upgrade to %s failed (giving up): %s", jujuversion.Current, failReason),
		})
	} else {
		calls = append(calls, StatusCall{status.Started, ""})
	}
	return calls
}

func (s *UpgradeSuite) makeExpectedUpgradeLogs(retryCount int, target string, expectFail bool, failReason string) []loggo.Entry {
	var outLogs []loggo.Entry

	if target == "databaseMaster" || target == "controller" {
		outLogs = append(outLogs, loggo.Entry{
			Level: loggo.INFO, Message: "waiting for other controllers to be ready for upgrade",
		})
		outLogs = append(outLogs, loggo.Entry{
			Level:   loggo.INFO,
			Message: "finished waiting - all controllers are ready to run upgrade steps",
		})
	}

	outLogs = append(outLogs, loggo.Entry{
		Level: loggo.INFO, Message: fmt.Sprintf(
			`starting upgrade from %s to %s for "machine-0"`,
			s.oldVersion.Number, jujuversion.Current),
	})

	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(%%s\): %s`,
		s.oldVersion.Number, jujuversion.Current, failReason)

	for i := 0; i < retryCount; i++ {
		outLogs = append(outLogs, loggo.Entry{Level: loggo.ERROR, Message: fmt.Sprintf(failMessage, "will retry")})
	}
	if expectFail {
		outLogs = append(outLogs, loggo.Entry{Level: loggo.ERROR, Message: fmt.Sprintf(failMessage, "giving up")})
	} else {
		outLogs = append(outLogs, loggo.Entry{Level: loggo.INFO,
			Message: fmt.Sprintf(`upgrade to %s completed successfully.`, jujuversion.Current)})
	}
	return outLogs
}

func (s *UpgradeSuite) assertEnvironAgentVersion(c *tc.C, expected version.Number) {
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Assert(agentVersion, tc.Equals, expected)
}

// NewFakeConfigSetter returns a fakeConfigSetter which implements
// just enough of the agent.ConfigSetter interface to keep the upgrade
// steps worker happy.
func NewFakeConfigSetter(agentTag names.Tag, initialVersion version.Number) *fakeConfigSetter {
	return &fakeConfigSetter{
		AgentTag: agentTag,
		Version:  initialVersion,
	}
}

type fakeConfigSetter struct {
	agent.ConfigSetter
	AgentTag names.Tag
	Version  version.Number
}

func (s *fakeConfigSetter) Tag() names.Tag {
	return s.AgentTag
}

func (s *fakeConfigSetter) UpgradedToVersion() version.Number {
	return s.Version
}

func (s *fakeConfigSetter) SetUpgradedToVersion(newVersion version.Number) {
	s.Version = newVersion
}

// NewFakeAgent returns a fakeAgent which implements the agent.Agent
// interface. This provides enough MachineAgent functionality to
// support upgrades.
func NewFakeAgent(confSetter agent.ConfigSetter) *fakeAgent {
	return &fakeAgent{
		config: confSetter,
	}
}

type fakeAgent struct {
	config agent.ConfigSetter
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return a.config
}

func (a *fakeAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(a.config)
}

type StatusCall struct {
	Status status.Status
	Info   string
}

type testStatusSetter struct {
	Calls []StatusCall
}

func (s *testStatusSetter) SetStatus(status status.Status, info string, _ map[string]interface{}) error {
	s.Calls = append(s.Calls, StatusCall{status, info})
	return nil
}
