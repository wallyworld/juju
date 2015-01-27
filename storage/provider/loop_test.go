package provider

import (
	"fmt"

	"github.com/juju/juju/storage"
	gc "gopkg.in/check.v1"
)

const stubMachineId = "machine101"

var _ = gc.Suite(&loopSuite{})

type loopSuite struct{}

func (s *loopSuite) TestCreateVolumes(c *gc.C) {

	var cmdStore []string
	lvs := &loopVolumeSource{
		"dataDir",
		"subDir",
		stubMachineId,
		mockRunCmdFn(&cmdStore),
		make(map[string]blockDevicePlus),
	}
	devices, err := lvs.CreateVolumes([]storage.VolumeParams{{
		Name: "test volume",
		Size: 2,
	}})

	c.Assert(err, gc.IsNil)
	c.Assert(devices, gc.HasLen, 1)
	c.Check(cmdStore, gc.Not(gc.HasLen), 0)

	device := devices[0]
	c.Check(device.Name, gc.Equals, "test volume")
	c.Check(device.DeviceName, gc.Equals, "")
	c.Check(device.Size, gc.Equals, uint64(2))
	c.Check(device.InUse, gc.Equals, false)
	c.Check(device.ProviderId, gc.Equals, fmt.Sprintf("%s-loop0", stubMachineId))
}

func (s *loopSuite) TestDescribeVolumes(c *gc.C) {
	expectedBlockDevice := storage.BlockDevice{Name: "foo"}
	lvs := &loopVolumeSource{volIdToBlockDevice: map[string]blockDevicePlus{
		"a": blockDevicePlus{expectedBlockDevice, "bar"},
	}}

	blockDevices, err := lvs.DescribeVolumes([]string{"a"})

	c.Assert(err, gc.IsNil)
	c.Assert(blockDevices, gc.HasLen, 1)

	c.Check(blockDevices[0], gc.DeepEquals, expectedBlockDevice)
}

func (s *loopSuite) TestDestroyVolumes(c *gc.C) {

	expectedBlockDevice := storage.BlockDevice{DeviceName: "foo"}

	var cmdStore []string
	lvs := &loopVolumeSource{
		"dataDir",
		"subDir",
		stubMachineId,
		mockRunCmdFn(&cmdStore),
		map[string]blockDevicePlus{
			"a": blockDevicePlus{expectedBlockDevice, "b"},
		},
	}

	err := lvs.DestroyVolumes([]string{"a"})

	c.Assert(err, gc.IsNil)
	c.Check(cmdStore, gc.Not(gc.HasLen), 0)
}

func mockRunCmdFn(cmdStore *[]string) RunCommandFn {
	return func(cmd string, args ...string) (stdout string, err error) {
		*cmdStore = append(append(*cmdStore, cmd), args...)
		return "", nil
	}
}
