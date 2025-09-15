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

type FilesystemStatusSuite struct {
	StorageStateSuiteBase
	machine    *state.Machine
	filesystem state.Filesystem
}

func TestFilesystemStatusSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &FilesystemStatusSuite{})
}

func (s *FilesystemStatusSuite) SetUpTest(c *tc.C) {
	s.StorageStateSuiteBase.SetUpTest(c)

	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{
				Pool: "modelscoped", Size: 1024,
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	filesystemAttachments, err := s.storageBackend.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystemAttachments, tc.HasLen, 1)

	filesystem, err := s.storageBackend.Filesystem(filesystemAttachments[0].Filesystem())
	c.Assert(err, tc.ErrorIsNil)

	s.machine = machine
	s.filesystem = filesystem
}

func (s *FilesystemStatusSuite) TestInitialStatus(c *tc.C) {
	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) checkInitialStatus(c *tc.C) {
	statusInfo, err := s.filesystem.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Pending)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *FilesystemStatusSuite) TestSetErrorStatusWithoutInfo(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) TestSetUnknownStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Assert(err, tc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) TestSetOverwritesData(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusAlive(c *tc.C) {
	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) checkGetSetStatus(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	filesystem, err := s.storageBackend.Filesystem(s.filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	statusInfo, err := filesystem.Status()
	c.Check(err, tc.ErrorIsNil)
	c.Check(statusInfo.Status, tc.Equals, status.Attaching)
	c.Check(statusInfo.Message, tc.Equals, "blah")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, tc.NotNil)
}

func (s *FilesystemStatusSuite) TestGetSetStatusDying(c *tc.C) {
	err := s.storageBackend.DestroyFilesystem(s.filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusDead(c *tc.C) {
	err := s.storageBackend.DestroyFilesystem(s.filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DetachFilesystem(s.machine.MachineTag(), s.filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(s.machine.MachineTag(), s.filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	filesystem, err := s.storageBackend.Filesystem(s.filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystem.Life(), tc.Equals, state.Dead)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusGone(c *tc.C) {
	s.obliterateFilesystem(c, s.filesystem.FilesystemTag())

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "not really",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status: filesystem not found`)

	statusInfo, err := s.filesystem.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: filesystem not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *FilesystemStatusSuite) TestSetStatusPendingUnprovisioned(c *tc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "still",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)
}

func (s *FilesystemStatusSuite) TestSetStatusPendingProvisioned(c *tc.C) {
	err := s.storageBackend.SetFilesystemInfo(s.filesystem.FilesystemTag(), state.FilesystemInfo{
		FilesystemId: "fs-id",
	})
	c.Assert(err, tc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = s.filesystem.SetStatus(sInfo)
	c.Check(err, tc.ErrorMatches, `cannot set status "pending"`)
}
