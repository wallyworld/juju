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
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/volume"
)

type volumeSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	volumeManager volume.VolumeManager

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
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func (s *volumeSuite) createUnitForTest(c *gc.C) string {
	jujustorage.RegisterDefaultPool("someprovider", jujustorage.StorageKindBlock, "block")
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	devices, err := machine.BlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 1)

	return machineId
}

func (s *volumeSuite) TestVolumeList(c *gc.C) {
	s.createUnitForTest(c)
	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 1)
	c.Assert(volumes.Disks[0].Attachments, gc.HasLen, 1)
}

func (s *volumeSuite) TestVolumeListByMachine(c *gc.C) {
	m1 := s.createUnitForTest(c)

	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{Machines: []string{m1}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 1)
	c.Assert(volumes.Disks[0].Attachments, gc.HasLen, 1)

	none, err := s.api.ListVolumes(params.StorageVolumeFilter{Machines: []string{"blah"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(none.Disks, gc.HasLen, 0)
}

func (s *volumeSuite) TestVolumeListNoVolumes(c *gc.C) {
	volumes, err := s.api.ListVolumes(params.StorageVolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Disks, gc.HasLen, 0)
}
