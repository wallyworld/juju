// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/storage"
)

func TestDiskManagerWorkerSuite(t *tctesting.T) {
	tc.Run(t, &DiskManagerWorkerSuite{})
}

type DiskManagerWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(diskmanager.BlockDeviceInUse, func(storage.BlockDevice) (bool, error) {
		return false, nil
	})
}

func (s *DiskManagerWorkerSuite) TestWorker(c *tc.C) {
	done := make(chan struct{})
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		close(done)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{DeviceName: "whatever"}}, nil
	}

	w := diskmanager.NewWorker(listDevices, setDevices)
	defer w.Wait()
	defer w.Kill()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskmanager to update")
	}
}

func (s *DiskManagerWorkerSuite) TestBlockDeviceChanges(c *tc.C) {
	var oldDevices []storage.BlockDevice
	var devicesSet [][]storage.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		devicesSet = append(devicesSet, append([]storage.BlockDevice{}, devices...))
		return nil
	}

	device := storage.BlockDevice{DeviceName: "sda", DeviceLinks: []string{"a", "b"}}
	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{device}, nil
	}

	err := diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 1)

	// diskmanager only calls the BlockDeviceSetter when it sees a
	// change in disks. Order of DeviceLinks should not matter.
	device.DeviceLinks = []string{"b", "a"}
	err = diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 1)

	device.DeviceName = "sdb"
	err = diskmanager.DoWork(listDevices, setDevices, &oldDevices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devicesSet, tc.HasLen, 2)

	c.Assert(devicesSet[0], tc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sda", DeviceLinks: []string{"a", "b"},
	}})
	c.Assert(devicesSet[1], tc.DeepEquals, []storage.BlockDevice{{
		DeviceName: "sdb", DeviceLinks: []string{"a", "b"},
	}})
}

func (s *DiskManagerWorkerSuite) TestBlockDevicesSorted(c *tc.C) {
	var devicesSet [][]storage.BlockDevice
	var setDevices BlockDeviceSetterFunc = func(devices []storage.BlockDevice) error {
		devicesSet = append(devicesSet, devices)
		return nil
	}

	var listDevices diskmanager.ListBlockDevicesFunc = func() ([]storage.BlockDevice, error) {
		return []storage.BlockDevice{{
			DeviceName: "sdb",
		}, {
			DeviceName: "sda",
		}, {
			DeviceName: "sdc",
		}}, nil
	}
	err := diskmanager.DoWork(listDevices, setDevices, new([]storage.BlockDevice))
	c.Assert(err, tc.ErrorIsNil)

	// The block Devices should be sorted when passed to the block
	// device setter.
	c.Assert(devicesSet, tc.DeepEquals, [][]storage.BlockDevice{{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}, {
		DeviceName: "sdc",
	}}})
}

type BlockDeviceSetterFunc func([]storage.BlockDevice) error

func (f BlockDeviceSetterFunc) SetMachineBlockDevices(devices []storage.BlockDevice) error {
	return f(devices)
}
