// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type VolumeListSuite struct {
	SubStorageSuite
	mockAPI *mockVolumeListAPI
}

var _ = gc.Suite(&VolumeListSuite{})

func (s *VolumeListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockVolumeListAPI{fillAssigned: true,
		fillAttached:    true,
		fillDeviceName:  true,
		fillSize:        true,
		fillFileSystem:  true,
		fillProvisioned: true}
	s.PatchValue(storage.GetVolumeListAPI, func(c *storage.VolumeListCommand) (storage.VolumeListAPI, error) {
		return s.mockAPI, nil
	})

}

func runVolumeList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.VolumeListCommand{}), args...)
}

func (s *VolumeListSuite) TestVolumeListEmpty(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		"[]\n",
	)
}

func (s *VolumeListSuite) TestVolumeListYaml(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      attached: true
      device-name: testdevice
      size: 1024
      file-system: fstype
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoProvisioned(c *gc.C) {
	s.mockAPI.fillProvisioned = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      attached: true
      device-name: testdevice
      size: 1024
      file-system: fstype
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoFileSystem(c *gc.C) {
	s.mockAPI.fillFileSystem = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      attached: true
      device-name: testdevice
      size: 1024
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoSize(c *gc.C) {
	s.mockAPI.fillSize = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      attached: true
      device-name: testdevice
      file-system: fstype
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoDeviceName(c *gc.C) {
	s.mockAPI.fillDeviceName = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      attached: true
      size: 1024
      file-system: fstype
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoAttached(c *gc.C) {
	s.mockAPI.fillAttached = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      assigned: true
      machine: "2"
      device-name: testdevice
      size: 1024
      file-system: fstype
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoAssigned(c *gc.C) {
	s.mockAPI.fillAssigned = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    zdisktag:
      storage: shared-fs
      machine: "2"
      attached: true
      device-name: testdevice
      size: 1024
      file-system: fstype
      provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "--format", "json"},
		`
[{"Attachments":{"zdisktag":{"storage":"shared-fs","assigned":true,"machine":"2","attached":true,"device-name":"testdevice","size":1024,"file-system":"fstype","provisioned":true}}}]
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
VOLUME    ATTACHED  MACHINE  DEVICE NAME  SIZE    
zdisktag  true      2        testdevice   1.0GiB  

`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabularSort(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
VOLUME    ATTACHED  MACHINE  DEVICE NAME  SIZE    
mdisktag  true      3        testdevice   1.0GiB  
zdisktag  true      2        testdevice   1.0GiB  

`[1:],
	)
}

func (s *VolumeListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runVolumeList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockVolumeListAPI struct {
	fillAssigned, fillAttached, fillDeviceName, fillSize, fillFileSystem, fillProvisioned bool
	//	fillAssigned, fillAttached, fillDeviceName, fillSize, fillFileSystem, fillProvisioned bool
	// TODO(anastasiamac 2015-02-01) ATM , this can only create
	// multiple attachments per volume.
}

func (s mockVolumeListAPI) Close() error {
	return nil
}

func (s mockVolumeListAPI) ListVolumes(machines []string) ([]params.StorageVolume, error) {
	if len(machines) == 0 {
		return nil, nil
	}
	return []params.StorageVolume{s.createTestVolumeInstance(machines)}, nil
}

func (s mockVolumeListAPI) createTestVolumeInstance(machines []string) params.StorageVolume {
	// want to have out-of-lexical order volume names for machines
	prefix := map[string]string{
		"0": "w",
		"1": "t",
		"2": "z",
		"3": "m",
	}
	attachments := make([]params.VolumeAttachment, len(machines))
	for i, amachine := range machines {
		attachments[i] = s.createTestAttachmentInstance(amachine, prefix[amachine])
	}

	return params.StorageVolume{
		Attachments: attachments,
	}
}
func (s mockVolumeListAPI) createTestAttachmentInstance(amachine, prefix string) params.VolumeAttachment {
	result := params.VolumeAttachment{
		Volume:  prefix + "disktag",
		Storage: "storage-shared-fs-0",
		Machine: "machine-" + amachine,
	}
	if s.fillAssigned {
		result.Assigned = true
	}
	if s.fillAttached {
		result.Attached = true
	}
	if s.fillDeviceName {
		result.DeviceName = "testdevice"
	}
	if s.fillSize {
		size := uint64(1024)
		result.Size = &size
	}
	if s.fillFileSystem {
		result.FileSystem = "fstype"
	}
	if s.fillProvisioned {
		result.Provisioned = true
	}
	return result
}
