// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/diskmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

func TestDiskManagerSuite(t *tctesting.T) {
	tc.Run(t, &DiskManagerSuite{})
}

type DiskManagerSuite struct {
	coretesting.BaseSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *diskmanager.DiskManagerAPI
}

func (s *DiskManagerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	s.st = &mockState{}
	diskmanager.PatchState(s, s.st)

	var err error
	s.api, err = diskmanager.NewDiskManagerAPI(facadetest.Context{
		Auth_: s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *tc.C) {
	devices := []storage.BlockDevice{{DeviceName: "sda"}, {DeviceName: "sdb"}}
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      "machine-0",
			BlockDevices: devices,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesEmptyArgs(c *tc.C) {
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *DiskManagerSuite) TestNewDiskManagerAPINonMachine(c *tc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := diskmanager.NewDiskManagerAPI(facadetest.Context{
		Auth_: s.authorizer,
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesInvalidTags(c *tc.C) {
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}, {
			Machine: "machine-1",
		}, {
			Machine: "unit-mysql-0",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})
	c.Assert(s.st.calls, tc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesStateError(c *tc.C) {
	s.st.err = errors.New("boom")
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
}

type mockState struct {
	calls   int
	devices map[string][]state.BlockDeviceInfo
	err     error
}

func (st *mockState) SetMachineBlockDevices(machineId string, devices []state.BlockDeviceInfo) error {
	st.calls++
	if st.devices == nil {
		st.devices = make(map[string][]state.BlockDeviceInfo)
	}
	st.devices[machineId] = devices
	return st.err
}
