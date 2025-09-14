// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type VolumeStatusSuite struct {
	StorageStateSuiteBase
	machine *state.Machine
	volume  state.Volume
}

func TestVolumeStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &VolumeStatusSuite{})
}

func (s *VolumeStatusSuite) SetUpTest(c *tc.C) {
	s.StorageStateSuiteBase.SetUpTest(c)

	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped", Size: 1024,
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	volumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 1)

	volume, err := s.storageBackend.Volume(volumeAttachments[0].Volume())
	c.Assert(err, tc.ErrorIsNil)

	s.machine = machine
	s.volume = volume
}

func (s *VolumeStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.volume.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Pending)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *VolumeStatusSuite) TestSetErrorStatusWithoutInfo(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Assert(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusAlive(c *tc.C) {
	validStatuses := []status.Status{
		status.Attaching, status.Attached, status.Detaching,
		status.Detached, status.Destroying,
	}
	for _, status := range validStatuses {
		s.checkGetSetStatus(c, status)
	}
}

func (s *VolumeStatusSuite) checkGetSetStatus(c *tc.C, volumeStatus status.Status) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  volumeStatus,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	volume, err := s.storageBackend.Volume(s.volume.VolumeTag())
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := volume.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, volumeStatus)
	c.Check(statusInfo.Message, tc.Equals, "blah")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *VolumeStatusSuite) TestGetSetStatusDying(c *tc.C) {
	err := s.storageBackend.DestroyVolume(s.volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusDead(c *tc.C) {
	err := s.storageBackend.DestroyVolume(s.volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DetachVolume(s.machine.MachineTag(), s.volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(s.machine.MachineTag(), s.volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	volume, err := s.storageBackend.Volume(s.volume.VolumeTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volume.Life(), tc.Equals, state.Dead)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusGone(c *tc.C) {
	s.obliterateVolume(c, s.volume.VolumeTag())

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "not really",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: volume not found`)

	statusInfo, err := s.volume.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: volume not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *VolumeStatusSuite) TestSetStatusPendingUnprovisioned(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "still",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)
}

func (s *VolumeStatusSuite) TestSetStatusPendingProvisioned(c *tc.C) {
	err := s.storageBackend.SetVolumeInfo(s.volume.VolumeTag(), state.VolumeInfo{
		VolumeId: "vol-ume",
	})
	c.Assert(err, tc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = s.volume.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "pending"`)
}
