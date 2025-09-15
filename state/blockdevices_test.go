// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type BlockDevicesSuite struct {
	ConnSuite
	machine *state.Machine
}

func TestBlockDevicesSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &BlockDevicesSuite{})
}

func (s *BlockDevicesSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BlockDevicesSuite) assertBlockDevices(c *tc.C, tag names.MachineTag, expected []state.BlockDeviceInfo) {
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	info, err := sb.BlockDevices(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevices(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)

	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sdb})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdates(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err := s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda, sdb})

	sdb.Label = "root"
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sdb})

	// If a device is attached, unattached, then attached again,
	// then it gets a new name.
	sdb.Label = "" // Label should be reset.
	sdb.FilesystemType = "ext4"
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{
		sda,
		sdb,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesMachineDying(c *tc.C) {
	err := s.machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err = s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUnchanged(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})

	// Setting the same should not change txn-revno.
	docID := state.DocID(s.State, s.machine.Id())
	before, err := state.TxnRevno(s.State, state.BlockDevicesC, docID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)

	after, err := state.TxnRevno(s.State, state.BlockDevicesC, docID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(after, tc.Equals, before)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesConcurrently(c *tc.C) {
	sdaInner := state.BlockDeviceInfo{DeviceName: "sda"}
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.machine.SetMachineBlockDevices(sdaInner)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	sdaOuter := state.BlockDeviceInfo{
		DeviceName: "sda",
		Label:      "root",
	}
	err := s.machine.SetMachineBlockDevices(sdaOuter)
	c.Assert(err, tc.ErrorIsNil)

	// the outer call should wipe out the inner one's update.
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{
		sdaOuter,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesEmpty(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})

	err = s.machine.SetMachineBlockDevices()
	c.Assert(err, tc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{})
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	_, err = sb.BlockDevices(s.machine.MachineTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *BlockDevicesSuite) TestWatchBlockDevices(c *tc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	sdc := state.BlockDeviceInfo{DeviceName: "sdc"}
	err := s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, tc.ErrorIsNil)

	// Start block device watcher.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	w := sb.WatchBlockDevices(s.machine.MachineTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Setting the same should not trigger the watcher.
	err = s.machine.SetMachineBlockDevices(sdc, sdb, sda)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// change sdb's label.
	sdb.Label = "fatty"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// change sda's label and sdb's UUID at once.
	sda.Label = "giggly"
	sdb.UUID = "4c062658-6225-4f4b-96f3-debf00b964b4"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// drop sdc.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// add sdc again: should get a new name.
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}
