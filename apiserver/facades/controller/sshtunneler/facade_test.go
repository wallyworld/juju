// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"errors"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestSshtunnelerSuite(t *tctesting.T) {
	tc.Run(t, &sshtunnelerSuite{})
}

type sshtunnelerSuite struct {
	ctx     *MockContext
	backend *MockBackend
}

func (s *sshtunnelerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctx = NewMockContext(ctrl)
	s.backend = NewMockBackend(ctrl)
	return ctrl
}

func (s *sshtunnelerSuite) TestAuth(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authorizer := NewMockAuthorizer(s.setupMocks(c))
	s.ctx.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := newExternalFacade(s.ctx)
	c.Assert(err, tc.ErrorMatches, `permission denied`)
}

func (s *sshtunnelerSuite) TestInsertSSHConnRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
}

func (s *sshtunnelerSuite) TestInsertSSHConnRequestError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().InsertSSHConnRequest(gomock.Any()).Return(errors.New("insert error"))

	result, err := f.InsertSSHConnRequest(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error.Message, tc.Equals, "insert error")
}

func (s *sshtunnelerSuite) TestRemoveSSHConnRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(nil)

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
}

func (s *sshtunnelerSuite) TestRemoveSSHConnRequestError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHConnRequestRemoveArg{
		TunnelID: "tunnel-id",
	}

	s.backend.EXPECT().RemoveSSHConnRequest(gomock.Any()).Return(errors.New("remove error"))

	result, err := f.RemoveSSHConnRequest(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error.Message, tc.Equals, "remove error")
}

func (s *sshtunnelerSuite) TestControllerAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.backend.EXPECT().ControllerMachine("1").Return(
		&state.Machine{}, nil,
	)

	f := newFacade(s.ctx, s.backend)

	entity := params.Entity{Tag: names.NewMachineTag("1").String()}
	addresses := f.ControllerAddresses(entity)
	c.Assert(addresses, tc.DeepEquals, params.StringsResult{})
}

func (s *sshtunnelerSuite) TestControllerAddressWithControllerTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	f := newFacade(s.ctx, s.backend)

	entity := params.Entity{Tag: names.NewControllerTag("1").String()}
	result := f.ControllerAddresses(entity)
	c.Assert(result.Error.Message, tc.Equals, "SSH proxy from machine to k8s controller not supported")
}

func (s *sshtunnelerSuite) TestMachineHostKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "my-model"
	machineTag := names.NewMachineTag("1")

	s.backend.EXPECT().SSHHostKeys(modelUUID, machineTag).Return(
		[]string{"key-1", "key-2"}, nil,
	)

	f := newFacade(s.ctx, s.backend)

	arg := params.SSHMachineHostKeysArg{
		ModelUUID:  modelUUID,
		MachineTag: machineTag.String(),
	}
	result, err := f.MachineHostKeys(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.PublicKeys, tc.DeepEquals, []string{"key-1", "key-2"})
}
