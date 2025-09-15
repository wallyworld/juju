// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"bytes"
	"fmt"
	"strings"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/machine/mocks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type UpgradeMachineSuite struct {
	testhelpers.IsolationSuite

	statusExpectation   *statusExpectation
	prepareExpectation  *upgradeMachinePrepareExpectation
	completeExpectation *upgradeMachineCompleteExpectation
}

func TestUpgradeMachineSuite(t *tctesting.T) {
	tc.Run(t, &UpgradeMachineSuite{})
}

func (s *UpgradeMachineSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.statusExpectation = &statusExpectation{
		status: &params.FullStatus{
			Machines: map[string]params.MachineStatus{
				"1": {Id: "1"},
				"2": {
					Id: "2",
					Containers: map[string]params.MachineStatus{
						"2/lxd/0": {Id: "2/lxd/0"},
					},
				},
			},
			Applications: map[string]params.ApplicationStatus{
				"foo": {
					Units: map[string]params.UnitStatus{
						"foo/1": {
							Machine: "1",
							Subordinates: map[string]params.UnitStatus{
								"sub/1": {},
							},
						},
						"foo/2": {
							Machine: "2/lxd/0",
							Subordinates: map[string]params.UnitStatus{
								"sub/2": {},
							},
						},
					},
				},
			},
		},
	}
	s.prepareExpectation = &upgradeMachinePrepareExpectation{gomock.Any(), gomock.Any(), gomock.Any()}
	s.completeExpectation = &upgradeMachineCompleteExpectation{gomock.Any()}

}

const (
	machineArg   = "1"
	containerArg = "2/lxd/0"

	channelArg = "20.04/stable"
	baseArg    = "ubuntu@20.04"
)

func (s *UpgradeMachineSuite) runUpgradeMachineCommand(c *tc.C, args ...string) error {
	_, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", args...)
	return err
}

func (s *UpgradeMachineSuite) ctxWithConfirmation(c *tc.C, confirmation string) *cmd.Context {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	ctx.Stderr = &stderr
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin
	stdin.WriteString(confirmation)

	return ctx
}

func (s *UpgradeMachineSuite) runUpgradeMachineCommandWithConfirmation(
	c *tc.C, confirmation string, args ...string,
) (*cmd.Context, error) {
	ctx := s.ctxWithConfirmation(c, confirmation)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockStatusAPI := mocks.NewMockStatusAPI(ctrl)
	mockUpgradeMachineAPI := mocks.NewMockUpgradeMachineAPI(ctrl)

	uExp := mockUpgradeMachineAPI.EXPECT()
	prep := s.prepareExpectation
	uExp.UpgradeSeriesPrepare(prep.machineArg, prep.channelArg, prep.force).AnyTimes()
	uExp.UpgradeSeriesComplete(s.completeExpectation.machineNumber).AnyTimes()

	mockStatusAPI.EXPECT().Status(gomock.Nil()).AnyTimes().Return(s.statusExpectation.status, nil)

	com := machine.NewUpgradeMachineCommandForTest(mockStatusAPI, mockUpgradeMachineAPI)

	err := cmdtesting.InitCommand(com, args)
	if err != nil {
		return nil, err
	}
	err = com.Run(ctx)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (s *UpgradeMachineSuite) TestPrepareCommandMachines(c *tc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{machineArg, channelArg, gomock.Eq(false)}
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandContainers(c *tc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{containerArg, channelArg, gomock.Eq(false)}
	err := s.runUpgradeMachineCommand(c, containerArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestTooFewArgs(c *tc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg)
	c.Assert(err, tc.ErrorMatches, "wrong number of arguments")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptForceOption(c *tc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{machineArg, channelArg, gomock.Eq(true)}
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAbortOnFailedConfirmation(c *tc.C) {
	_, err := s.runUpgradeMachineCommandWithConfirmation(c, "n", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorMatches, "upgrade machine: aborted")
}

func (s *UpgradeMachineSuite) TestUpgradeCommandShouldNotAcceptInvalidPrepCommands(c *tc.C) {
	invalidPrepCommand := "actuate"
	err := s.runUpgradeMachineCommand(c, machineArg, invalidPrepCommand, baseArg)
	c.Assert(err, tc.ErrorMatches,
		".* \"actuate\" is an invalid upgrade-machine command; valid commands are: prepare, complete.")
}

func (s *UpgradeMachineSuite) TestUpgradeCommandShouldNotAcceptInvalidMachineArgs(c *tc.C) {
	invalidMachineArg := "machine5"
	err := s.runUpgradeMachineCommand(c, invalidMachineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorMatches, "\"machine5\" is an invalid machine name")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldOnlyAcceptSupportedSeries(c *tc.C) {
	BadSeries := "Combative Caribou"
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, BadSeries)
	c.Assert(err, tc.ErrorMatches, "series .* not valid")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldSupportSeriesRegardlessOfCase(c *tc.C) {
	capitalizedCaseJammy := "Jammy"
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, capitalizedCaseJammy)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestCompleteCommand(c *tc.C) {
	s.completeExpectation.machineNumber = machineArg
	err := s.runUpgradeMachineCommand(c, machineArg, machine.CompleteCommand)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestCompleteCommandDoesNotAcceptSeries(c *tc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.CompleteCommand, baseArg)
	c.Assert(err, tc.ErrorMatches, "wrong number of arguments")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYes(c *tc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "--yes")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYesAbbreviation(c *tc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "-y")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldPromptUserForConfirmation(c *tc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), tc.Matches, `(?s).*Continue \[y\/N\]\? .*`)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldIndicateOnlySubordinatesOnMachine(c *tc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, tc.ErrorIsNil)

	out := ctx.Stdout.(*bytes.Buffer).String()
	c.Check(strings.Contains(out, "sub/1"), tc.IsTrue)
	c.Check(strings.Contains(out, "sub/2"), tc.IsFalse)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYesFlagAndNotPrompt(c *tc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "n", machineArg, machine.PrepareCommand, baseArg, "-y")
	c.Assert(err, tc.ErrorIsNil)

	//There is no confirmation message since the `-y/--yes` flag is being used to avoid the prompt.
	confirmationMessage := ""

	finishedMessage := fmt.Sprintf(machine.UpgradeMachinePrepareFinishedMessage, machineArg)
	displayedMessage := strings.Join([]string{confirmationMessage, finishedMessage}, "") + "\n"
	out := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(out, tc.Equals, displayedMessage)
	c.Assert(out, tc.Contains, fmt.Sprintf("juju upgrade-machine %s complete", machineArg))
}

type statusExpectation struct {
	status interface{}
}

type upgradeMachinePrepareExpectation struct {
	machineArg, channelArg, force interface{}
}

type upgradeMachineCompleteExpectation struct {
	machineNumber interface{}
}
