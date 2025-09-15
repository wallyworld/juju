// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/internal/worker/provisioner/mocks"
)

type containerManifoldSuite struct {
	machine *mocks.MockContainerMachine
	getter  *mocks.MockContainerMachineGetter
}

func TestContainerManifoldSuite(t *tctesting.T) {
	tc.Run(t, &containerManifoldSuite{})
}

func (s *containerManifoldSuite) TestConfigValidateAgentName(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "empty AgentName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateAPICallerName(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{AgentName: "testing"}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "empty APICallerName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateLogger(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "nil Logger not valid")
}

func (s *containerManifoldSuite) TestConfigValidateMachineLock(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        &noOpLogger{},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "missing MachineLock not valid")
}

func (s *containerManifoldSuite) TestConfigValidateCredentialValidatorFacade(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        &noOpLogger{},
		MachineLock:   &fakeMachineLock{},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "missing NewCredentialValidatorFacade not valid")
}

func (s *containerManifoldSuite) TestConfigValidateContainerType(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:                    "testing",
		APICallerName:                "another string",
		Logger:                       &noOpLogger{},
		MachineLock:                  &fakeMachineLock{},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "missing Container Type not valid")
}

func (s *containerManifoldSuite) TestConfigValidateSuccess(c *tc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:                    "testing",
		APICallerName:                "another string",
		Logger:                       &noOpLogger{},
		MachineLock:                  &fakeMachineLock{},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
		ContainerType:                instance.LXD,
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifold(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{instance.LXD}, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	m, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.NotNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldContainersNotKnown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return(nil, false, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(errors.Is(err, errors.NotYetAvailable), tc.IsTrue)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldNoContainerSupport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return(nil, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, tc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldMachineDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().Life().Return(life.Dead)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, tc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = mocks.NewMockContainerMachine(ctrl)
	s.getter = mocks.NewMockContainerMachineGetter(ctrl)

	return ctrl
}
