// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	backupsAPI "github.com/juju/juju/apiserver/facades/client/backups"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type backupsSuite struct {
	testing.JujuConnSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *backupsAPI.API
	meta       *backups.Metadata
	machineTag names.MachineTag
}

func TestBackupsSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &backupsSuite{})
}

func (s *backupsSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.resources = common.NewResources()
	s.resources.RegisterNamed("dataDir", common.StringResource(s.DataDir()))
	s.resources.RegisterNamed("machineID", common.StringResource(s.machineTag.Id()))

	ssInfo, err := s.State.StateServingInfo()
	c.Assert(err, tc.ErrorIsNil)
	agentConfig := s.AgentConfigForTag(c, s.machineTag)
	agentConfig.SetStateServingInfo(controller.StateServingInfo{
		PrivateKey:   ssInfo.PrivateKey,
		Cert:         ssInfo.Cert,
		CAPrivateKey: ssInfo.CAPrivateKey,
		SharedSecret: ssInfo.SharedSecret,
		APIPort:      ssInfo.APIPort,
		StatePort:    ssInfo.StatePort,
	})
	err = agentConfig.Write()
	c.Assert(err, tc.ErrorIsNil)

	tag := names.NewLocalUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	shim := &stateShim{
		State:            s.State,
		Model:            s.Model,
		controllerNodesF: func() ([]state.ControllerNode, error) { return nil, nil },
		machineF:         func(id string) (backupsAPI.Machine, error) { return &testMachine{}, nil },
	}
	s.api, err = backupsAPI.NewAPI(shim, s.resources, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	s.meta = backupstesting.NewMetadataStarted()
}

func (s *backupsSuite) setBackups(c *tc.C, meta *backups.Metadata, err string) *backupstesting.FakeBackups {
	fake := backupstesting.FakeBackups{
		Meta:     meta,
		Filename: "test-filename",
	}
	if meta != nil {
		fake.MetaList = append(fake.MetaList, meta)
	}
	if err != "" {
		fake.Error = errors.New(err)
	}
	s.PatchValue(backupsAPI.NewBackups,
		func(paths *backups.Paths) backups.Backups {
			return &fake
		},
	)
	return &fake
}

func (s *backupsSuite) TestNewAPIOkay(c *tc.C) {
	_, err := backupsAPI.NewAPI(&stateShim{State: s.State, Model: s.Model}, s.resources, s.authorizer)
	c.Check(err, tc.ErrorIsNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *tc.C) {
	s.authorizer.Tag = names.NewApplicationTag("eggs")
	_, err := backupsAPI.NewAPI(&stateShim{State: s.State, Model: s.Model}, s.resources, s.authorizer)
	c.Check(errors.Cause(err), tc.Equals, apiservererrors.ErrPerm)
}

func (s *backupsSuite) TestNewAPIHostedEnvironmentFails(c *tc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = backupsAPI.NewAPI(&stateShim{State: otherState, Model: otherModel}, s.resources, s.authorizer)
	c.Check(err, tc.ErrorMatches, "backups are only supported from the controller model\nUse juju switch to select the controller model")
}

func (s *backupsSuite) TestBackupsCAASFails(c *tc.C) {
	otherState := s.Factory.MakeCAASModel(c, nil)
	defer otherState.Close()
	otherModel, err := otherState.Model()
	c.Assert(err, tc.ErrorIsNil)

	isController := true
	_, err = backupsAPI.NewAPI(&stateShim{State: otherState, Model: otherModel, isController: &isController}, s.resources, s.authorizer)
	c.Assert(err, tc.ErrorMatches, "backups on kubernetes controllers not supported")
}
