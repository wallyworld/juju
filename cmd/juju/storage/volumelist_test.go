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

	s.mockAPI = &mockVolumeListAPI{}
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
    disktag:
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

func (s *VolumeListSuite) TestVolumeListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2", "--format", "json"},
		`
[{"Attachments":{"disktag":{"storage":"shared-fs","assigned":true,"machine":"2","attached":true,"device-name":"testdevice","size":1024,"file-system":"fstype","provisioned":true}}}]
`[1:],
	)
}

func (s *VolumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"2"},
		// Default format is tabular
		`
VOLUME   ATTACHED  MACHINE  DEVICE NAME  SIZE    
disktag  true      2        testdevice   1.0GiB  

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
}

func (s mockVolumeListAPI) Close() error {
	return nil
}

func (s mockVolumeListAPI) ListVolumes(machines []string) ([]params.StorageVolume, error) {
	results := make([]params.StorageVolume, len(machines))
	for i, amachine := range machines {
		results[i] = createTestVolumeInstance(amachine)
	}
	return results, nil
}

func createTestVolumeInstance(amachine string) params.StorageVolume {
	return params.StorageVolume{
		Attachments: []params.VolumeAttachment{
			createTestAttachmentInstance(amachine),
		},
	}
}
func createTestAttachmentInstance(amachine string) params.VolumeAttachment {
	size := uint64(1024)
	return params.VolumeAttachment{
		Volume:      "disktag",
		Storage:     "storage-shared-fs-0",
		Assigned:    true,
		Machine:     "machine-" + amachine,
		Attached:    true,
		DeviceName:  "testdevice",
		Size:        &size,
		FileSystem:  "fstype",
		Provisioned: true,
	}
}
