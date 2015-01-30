// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package volume_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	testing "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/volume"
)

type VolumeSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	testing.JujuConnSuite
	machine *state.Machine
	vm      volume.VolumeManager
}

var _ = gc.Suite(&VolumeSuite{})

func (s *VolumeSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.vm = volume.NewVolumeManager(s.JujuConnSuite.State)
}

func (s *VolumeSuite) TestList(c *gc.C) {
	dName := "sda"
	sda := state.BlockDeviceInfo{DeviceName: dName}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	all, err := s.vm.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 1)
	oneDisk := all[0]
	c.Assert(oneDisk.Attachments(), gc.HasLen, 1)
	attachment := oneDisk.Attachments()[0]
	c.Assert(attachment.DeviceName(), gc.DeepEquals, dName)
}
