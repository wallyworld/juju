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
		fillProvisioned: true,
		fillStorage:     true,
	}
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          assigned: true
          attached: true
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          assigned: true
          attached: true
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          assigned: true
          attached: true
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          assigned: true
          attached: true
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
    "2":
      "":
        zdisktag:
          storage: shared-fs
          assigned: true
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          assigned: true
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
    "2":
      testdevice:
        zdisktag:
          storage: shared-fs
          attached: true
          size: 1024
          file-system: fstype
          provisioned: true
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListYamlNoStorage(c *gc.C) {
	s.mockAPI.fillStorage = false
	s.assertValidList(
		c,
		[]string{"2", "--format", "yaml"},
		`
- attachments:
    "2":
      testdevice:
        zdisktag:
          assigned: true
          attached: true
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
[{"Attachments":{"2":{"testdevice":{"zdisktag":{"storage":"shared-fs","assigned":true,"attached":true,"size":1024,"file-system":"fstype","provisioned":true}}}}}]
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
MACHINE  DEVICE NAME  VOLUME    ATTACHED  SIZE    
2        testdevice   zdisktag  true      1.0GiB  

`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabularSort(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE  DEVICE NAME  VOLUME    ATTACHED  SIZE    
2        testdevice   zdisktag  true      1.0GiB  
3        testdevice   mdisktag  true      1.0GiB  

`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabularSortByMachine(c *gc.C) {
	s.mockAPI.bulk = true
	s.assertValidList(
		c,
		[]string{"2", "3"},
		// Default format is tabular
		`
MACHINE  DEVICE NAME  VOLUME   ATTACHED  SIZE    
0        xvda1        disk-0   false     1.0GiB  
0        xvda3        disk-1   false     1.0GiB  
0        xvdb         disk-2   false     1.0GiB  
1        xvda1        disk-3   false     1.0GiB  
1        xvda3        disk-4   false     1.0GiB  
1        xvdb         disk-5   false     1.0GiB  
2        loop0        disk-6   false     1.0GiB  
2        xvda1        disk-8   false     1.0GiB  
2        xvda3        disk-9   false     1.0GiB  
2        xvdb         disk-10  false     1.0GiB  
3        xvda1        disk-11  false     1.0GiB  
3        xvda3        disk-12  false     1.0GiB  
3        xvdb         disk-13  false     1.0GiB  
3        xvdf1        disk-7   false     1.0GiB  
4        xvda1        disk-14  false     1.0GiB  
4        xvda3        disk-15  false     1.0GiB  
4        xvdb         disk-16  false     1.0GiB  
5        xvda1        disk-17  false     1.0GiB  
5        xvda3        disk-18  false     1.0GiB  
5        xvdb         disk-19  false     1.0GiB  

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
	fillAssigned, fillAttached, fillDeviceName, fillSize, fillFileSystem, fillProvisioned, fillStorage bool
	// TODO(anastasiamac 2015-02-01) ATM , this can only create
	// multiple attachments per volume.

	bulk bool
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
	if s.bulk {
		return params.StorageVolume{
			Attachments: bulkCreateAttachmentsForSort(),
		}
	}

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

func bulkCreateAttachmentsForSort() []params.VolumeAttachment {
	size := uint64(1024)
	result := []params.VolumeAttachment{
		{Volume: "disk-0", Assigned: true, Machine: "machine-0", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-1", Assigned: true, Machine: "machine-0", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-2", Assigned: true, Machine: "machine-0", DeviceName: "xvdb", Size: &size},
		{Volume: "disk-3", Assigned: true, Machine: "machine-1", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-4", Assigned: true, Machine: "machine-1", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-5", Assigned: true, Machine: "machine-1", DeviceName: "xvdb", Size: &size},
		{Volume: "disk-10", Assigned: true, Machine: "machine-2", DeviceName: "xvdb", Size: &size},
		{Volume: "disk-6", Assigned: true, Machine: "machine-2", DeviceName: "loop0", Size: &size},
		{Volume: "disk-8", Assigned: true, Machine: "machine-2", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-9", Assigned: true, Machine: "machine-2", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-11", Assigned: true, Machine: "machine-3", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-12", Assigned: true, Machine: "machine-3", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-13", Assigned: true, Machine: "machine-3", DeviceName: "xvdb", Size: &size},
		{Volume: "disk-7", Assigned: true, Machine: "machine-3", DeviceName: "xvdf1", Size: &size},
		{Volume: "disk-14", Assigned: true, Machine: "machine-4", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-15", Assigned: true, Machine: "machine-4", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-16", Assigned: true, Machine: "machine-4", DeviceName: "xvdb", Size: &size},
		{Volume: "disk-17", Assigned: true, Machine: "machine-5", DeviceName: "xvda1", Size: &size},
		{Volume: "disk-18", Assigned: true, Machine: "machine-5", DeviceName: "xvda3", Size: &size},
		{Volume: "disk-19", Assigned: true, Machine: "machine-5", DeviceName: "xvdb", Size: &size},
	}
	return result
}

func (s mockVolumeListAPI) createTestAttachmentInstance(amachine, prefix string) params.VolumeAttachment {
	result := params.VolumeAttachment{
		Volume:  prefix + "disktag",
		Machine: "machine-" + amachine,
	}
	if s.fillStorage {
		result.Storage = "storage-shared-fs-0"
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
