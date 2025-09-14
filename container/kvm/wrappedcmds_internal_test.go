// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"errors"
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/container/kvm/libvirt"
	"github.com/juju/juju/internal/testhelpers"
)

type libvirtInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestLibvirtInternalSuite(t *tctesting.T) {
	tc.Run(t, &libvirtInternalSuite{})
}

func (libvirtInternalSuite) TestWriteMetadata(c *tc.C) {
	d := c.MkDir()

	err := writeMetadata(d)
	c.Check(err, tc.ErrorIsNil)
	b, err := os.ReadFile(filepath.Join(d, metadata))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(string(b), tc.Matches, `{"instance-id": ".*-.*-.*-.*"}`)
}

func (libvirtInternalSuite) TestWriteDomainXMLSucceeds(c *tc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "qcow2"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Matches, `/tmp/check-.*/\d+/host00.xml`)
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingValidSystemDisk(c *tc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "raw"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, tc.ErrorMatches, "missing system disk")
	c.Assert(got, tc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingOneDisk(c *tc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, tc.ErrorMatches, "got 1 disks, need at least 2")
	c.Assert(got, tc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingBothDisk(c *tc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks:    []libvirt.DiskInfo{},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, tc.ErrorMatches, "got 0 disks, need at least 2")
	c.Assert(got, tc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLNoHostname(c *tc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		runCmd: stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "qcow"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, tc.ErrorMatches, "missing required hostname")
	c.Assert(got, tc.Matches, "")
}

func (libvirtInternalSuite) TestPoolInfoSuccess(c *tc.C) {
	output := `
Name:           juju-pool
UUID:           06ebee2d-6bd0-4f47-a7dc-dea555fdaa3b
State:          running
Persistent:     yes
Autostart:      yes
Capacity:       35.31 GiB
Allocation:     3.54 GiB
Available:      31.77 GiB
`
	stub := runStub{output: output}
	got, err := poolInfo(stub.Run)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &libvirtPool{Name: "juju-pool", Autostart: "yes", State: "running"})

}

func (libvirtInternalSuite) TestPoolInfoNoPool(c *tc.C) {
	stub := runStub{err: errors.New("boom")}
	got, err := poolInfo(stub.Run)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.IsNil)
}
