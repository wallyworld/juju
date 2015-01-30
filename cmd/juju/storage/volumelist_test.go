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
		[]string{},
		"[]\n",
	)
}

func (s *VolumeListSuite) TestVolumeList(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"a"},
		// Default format is yaml
		"- attachments:\n"+
			"  - volume: disktag\n"+
			"    storage: storageId\n"+
			"    assigned: true\n"+
			"    machine: a\n"+
			"    attached: true\n"+
			"    device-name: testdevice\n"+
			"    uuid: testUUID\n"+
			"    label: testlabel\n"+
			"    size: 17876\n"+
			"    in-use: true\n"+
			"    file-system-type: fstype\n"+
			"    provisioned: true\n",
	)
}

func (s *VolumeListSuite) TestVolumeListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"a", "--format", "json"},
		`[{"Attachments":[{"volume":"disktag","storage":"storageId",`+
			`"assigned":true,"machine":"a","attached":true,`+
			`"device-name":"testdevice","uuid":"testUUID",`+
			`"label":"testlabel","size":17876,"in-use":true,`+
			`"file-system-type":"fstype","provisioned":true}]}`+
			"]\n",
	)
}

func (s *VolumeListSuite) TestVolumeListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"a", "--format", "tabular"},
		"VOLUME   ATTACHED  MACHINE  DEVICE NAME  UUID      LABEL      SIZE\n"+
			"disktag  true      a        testdevice   testUUID  testlabel  17876\n"+
			"\n",
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

func (s mockVolumeListAPI) ListVolumes(machines []string) ([]params.StorageDisk, error) {
	results := make([]params.StorageDisk, len(machines))
	for i, amachine := range machines {
		results[i] = createTestDiskInstance(amachine)
	}
	return results, nil
}

func createTestDiskInstance(amachine string) params.StorageDisk {
	return params.StorageDisk{
		Attachments: []params.VolumeAttachment{
			createTestAttachmentInstance(amachine),
		},
	}
}
func createTestAttachmentInstance(amachine string) params.VolumeAttachment {
	return params.VolumeAttachment{
		Volume:      "disktag",
		StorageId:   "storageId",
		Assigned:    true,
		MachineId:   amachine,
		Attached:    true,
		DeviceName:  "testdevice",
		UUID:        "testUUID",
		Label:       "testlabel",
		Size:        17876,
		InUse:       true,
		FSType:      "fstype",
		Provisioned: true,
	}
}
