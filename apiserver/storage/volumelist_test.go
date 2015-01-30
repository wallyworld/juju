// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/volume"
)

type volumeSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	volumeManager volume.VolumeManager
	machine       *state.Machine

	api        *storage.API
	authorizer testing.FakeAuthorizer
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) SetUpTest(c *gc.C) {
	// TODO(anastasiamac 2015-01-30) mock it
	s.JujuConnSuite.SetUpTest(c)

	s.volumeManager = volume.NewVolumeManager(s.State)
	s.authorizer = testing.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.api, err = storage.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(storage.GetVolumeManager, func(vs volume.VolumeState) volume.VolumeManager {
		return s.volumeManager
	})

	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *volumeSuite) createBlockDevicesOnMachine(c *gc.C, machine *state.Machine, deviceNames []string) {
	devices := make([]state.BlockDeviceInfo, len(deviceNames))
	for i, dName := range deviceNames {
		devices[i] = state.BlockDeviceInfo{DeviceName: dName}
	}
	err := machine.SetMachineBlockDevices(devices...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *volumeSuite) TestVolumeList(c *gc.C) {
	dName := "sda"
	s.createBlockDevicesOnMachine(c, s.machine, []string{dName})

	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 1)
	c.Assert(volumes.Disks[0].Attachments, gc.HasLen, 1)
	one := volumes.Disks[0].Attachments[0]
	c.Assert(one.DeviceName, gc.Equals, dName)
}

func (s *volumeSuite) TestVolumeListManyResults(c *gc.C) {
	s.createBlockDevicesOnMachine(c, s.machine, []string{"one", "two"})

	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 1)
	c.Assert(volumes.Disks[0].Attachments, gc.HasLen, 2)
}

func (s *volumeSuite) TestVolumeListByMachine(c *gc.C) {
	tstName := "fluff"
	s.createBlockDevicesOnMachine(c, s.machine, []string{tstName, "two"})

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.createBlockDevicesOnMachine(c, machine, []string{"123"})

	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{Machines: []string{s.machine.Id()}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 1)
	c.Assert(volumes.Disks[0].Attachments, gc.HasLen, 2)
	c.Assert(volumes.Disks[0].Attachments[0].DeviceName, gc.DeepEquals, tstName)
}

func (s *volumeSuite) TestVolumeListNoVolumes(c *gc.C) {
	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 0)
}
