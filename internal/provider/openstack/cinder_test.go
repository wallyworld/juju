// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"fmt"
	tctesting "testing"

	"github.com/go-goose/goose/v5/cinder"
	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/common/mocks"
	"github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/storage"
)

const (
	mockVolId    = "0"
	mockVolSize  = 1024 * 2
	mockVolName  = "123"
	mockServerId = "mock-server-id"
)

var (
	mockVolumeTag  = names.NewVolumeTag(mockVolName)
	mockMachineTag = names.NewMachineTag("456")
)

func TestCinderVolumeSourceSuite(t *tctesting.T) {
	tc.Run(t, &cinderVolumeSourceSuite{})
}

type cinderVolumeSourceSuite struct {
	testing.BaseSuite

	callCtx           *context.CloudCallContext
	invalidCredential bool
	env               *mocks.MockZonedEnviron
}

func (s *cinderVolumeSourceSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidCredential = true
			return nil
		},
	}
}

func (s *cinderVolumeSourceSuite) TearDownTest(c *tc.C) {
	s.invalidCredential = false
	s.BaseSuite.TearDownTest(c)
}

func init() {
	// Override attempt strategy to speed things up.
	openstack.CinderAttempt.Delay = 0
}

func toStringPtr(s string) *string {
	return &s
}

func (s *cinderVolumeSourceSuite) TestAttachVolumes(c *tc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, tc.Equals, mockVolId)
			c.Check(serverId, tc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	results, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: mockServerId,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []storage.AttachVolumesResult{{
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  mockVolumeTag,
			Machine: mockMachineTag,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceName: "sda",
			},
		},
	}})
}

var testUnauthorisedGooseError = gooseerrors.NewUnauthorisedf(nil, "", "invalid auth")

func (s *cinderVolumeSourceSuite) TestAttachVolumesInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{}, testUnauthorisedGooseError
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: mockServerId,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.invalidCredential, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestAttachVolumesNoDevice(c *tc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   nil,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	results, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: mockServerId,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Error, tc.ErrorMatches, "device not assigned to volume attachment")
}

func (s *cinderVolumeSourceSuite) TestCreateVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	const (
		requestedSize = 2 * 1024
		providedSize  = 3 * 1024
	)

	s.PatchValue(openstack.CinderAttempt, utils.AttemptStrategy{Min: 3})

	var getVolumeCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			c.Assert(args, tc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size:             requestedSize / 1024,
				Name:             "juju-testmodel-volume-123",
				AvailabilityZone: "zone-1",
			})
			return &cinder.Volume{
				ID: mockVolId,
			}, nil
		},
		listAvailabilityZones: func() ([]cinder.AvailabilityZone, error) {
			return []cinder.AvailabilityZone{{
				Name:  "zone-1",
				State: cinder.AvailabilityZoneState{Available: true},
			}}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			var status string
			getVolumeCalls++
			if getVolumeCalls > 1 {
				status = "available"
			}
			return &cinder.Volume{
				ID:     volumeId,
				Size:   providedSize / 1024,
				Status: status,
			}, nil
		},
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, tc.Equals, mockVolId)
			c.Check(serverId, tc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	results, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     requestedSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: mockServerId,
			},
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Error, tc.ErrorIsNil)

	c.Check(results[0].Volume, tc.DeepEquals, &storage.Volume{
		Tag: mockVolumeTag,
		VolumeInfo: storage.VolumeInfo{
			VolumeId:   mockVolId,
			Size:       providedSize,
			Persistent: true,
		},
	})

	// should have been 2 calls to GetVolume: twice initially
	// to wait until the volume became available.
	c.Check(getVolumeCalls, tc.Equals, 2)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeNoCompatibleZones(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, tc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: 1,
				Name: "juju-testmodel-volume-123",
			})
			return &cinder.Volume{
				ID: mockVolId,
			}, nil
		},
		listAvailabilityZones: func() ([]cinder.AvailabilityZone, error) {
			return []cinder.AvailabilityZone{{
				Name:  "nova",
				State: cinder.AvailabilityZoneState{Available: true},
			}}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(created, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeZonesNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var created bool
	mockAdapter := &mockAdapter{
		// listAvailabilityZones not implemented so we get a NotImplemented error.
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, tc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: 1,
				Name: "juju-testmodel-volume-123",
			})
			return &cinder.Volume{
				ID: mockVolId,
			}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(created, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeVolumeType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, tc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size:       1,
				Name:       "juju-testmodel-volume-123",
				VolumeType: "SSD",
			})
			return &cinder.Volume{ID: mockVolId}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
		Attributes: map[string]interface{}{
			"volume-type": "SSD",
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(created, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeInvalidCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			return &cinder.Volume{}, testUnauthorisedGooseError
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{}, testUnauthorisedGooseError
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
		Attributes: map[string]interface{}{
			"volume-type": "SSD",
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.invalidCredential, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestResourceTags(c *tc.C) {
	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, tc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: 1,
				Name: "juju-testmodel-volume-123",
				Metadata: map[string]string{
					"ResourceTag1": "Value1",
					"ResourceTag2": "Value2",
				},
			})
			return &cinder.Volume{ID: mockVolId}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
		ResourceTags: map[string]string{
			"ResourceTag1": "Value1",
			"ResourceTag2": "Value2",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: mockServerId,
			},
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(created, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestListVolumes(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID: "volume-1",
			}, {
				ID: "volume-2",
				Metadata: map[string]string{
					tags.JujuModel: "something-else",
				},
			}, {
				ID: "volume-3",
				Metadata: map[string]string{
					tags.JujuModel: testing.ModelTag.Id(),
				},
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	volumeIds, err := volSource.ListVolumes(s.callCtx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(volumeIds, tc.DeepEquals, []string{"volume-3"})
}

func (s *cinderVolumeSourceSuite) TestListVolumesInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{}, testUnauthorisedGooseError
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.ListVolumes(s.callCtx)
	c.Assert(err, tc.ErrorMatches, "invalid auth")
	c.Assert(s.invalidCredential, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumes(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID:   mockVolId,
				Size: mockVolSize / 1024,
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	volumes, err := volSource.DescribeVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(volumes, tc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   mockVolId,
			Size:       mockVolSize,
			Persistent: true,
		},
	}})
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumesInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{}, testUnauthorisedGooseError
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.DescribeVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorMatches, "invalid auth")
	c.Assert(s.invalidCredential, tc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumes(c *tc.C) {
	mockAdapter := &mockAdapter{}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
		{"DeleteVolume", []interface{}{mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesNotFound(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			return nil, errors.NotFoundf("volume %q", volId)
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesAttached(c *tc.C) {
	statuses := []string{"in-use", "detaching", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			c.Assert(statuses, tc.Not(tc.HasLen), 0)
			status := statuses[0]
			statuses = statuses[1:]
			return &cinder.Volume{
				ID:     volId,
				Status: status,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIsNil)
	c.Assert(statuses, tc.HasLen, 0)
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{{
		"GetVolume", []interface{}{mockVolId},
	}, {
		"GetVolume", []interface{}{mockVolId},
	}, {
		"GetVolume", []interface{}{mockVolId},
	}, {
		"DeleteVolume", []interface{}{mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			return &cinder.Volume{}, testUnauthorisedGooseError
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorMatches, "getting volume: invalid auth")
	c.Assert(s.invalidCredential, tc.IsTrue)
	mockAdapter.CheckCallNames(c, "GetVolume")
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumes(c *tc.C) {
	mockAdapter := &mockAdapter{}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
	metadata := map[string]string{
		"juju-controller-uuid": "",
		"juju-model-uuid":      "",
	}
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
		{"SetVolumeMetadata", []interface{}{mockVolId, metadata}},
	})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesAttached(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volId,
				Status: "in-use",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorMatches, `cannot release volume "0": volume still in-use`)
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{{
		"GetVolume", []interface{}{mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			return &cinder.Volume{}, testUnauthorisedGooseError
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.invalidCredential, tc.IsTrue)
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{{
		"GetVolume", []interface{}{mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesDetaching(c *tc.C) {
	statuses := []string{"detaching", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
			c.Assert(statuses, tc.Not(tc.HasLen), 0)
			status := statuses[0]
			statuses = statuses[1:]
			return &cinder.Volume{
				ID:     volId,
				Status: status,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIsNil)
	c.Assert(statuses, tc.HasLen, 0)
	mockAdapter.CheckCallNames(c, "GetVolume", "GetVolume", "SetVolumeMetadata")
}

func (s *cinderVolumeSourceSuite) TestDetachVolumes(c *tc.C) {
	const mockServerId2 = mockServerId + "2"

	var numDetachCalls int
	mockAdapter := &mockAdapter{
		detachVolume: func(serverId, volId string) error {
			numDetachCalls++
			if volId == "42" {
				return errors.NotFoundf("attachment")
			}
			c.Check(serverId, tc.Equals, mockServerId)
			c.Check(volId, tc.Equals, mockVolId)
			return nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	errs, err := volSource.DetachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   names.NewVolumeTag("123"),
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: mockServerId,
		},
	}, {
		Volume:   names.NewVolumeTag("42"),
		VolumeId: "42",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: mockServerId2,
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil, nil})
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"DetachVolume", []interface{}{mockServerId, mockVolId}},
		{"DetachVolume", []interface{}{mockServerId2, "42"}},
	})
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeCleanupDestroys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var numCreateCalls, numDestroyCalls, numGetCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			numCreateCalls++
			if numCreateCalls == 3 {
				return nil, errors.New("no volume for you")
			}
			return &cinder.Volume{
				ID:     fmt.Sprint(numCreateCalls),
				Status: "",
			}, nil
		},
		deleteVolume: func(volId string) error {
			numDestroyCalls++
			c.Assert(volId, tc.Equals, "2")
			return errors.New("destroy fails")
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			numGetCalls++
			if numGetCalls == 2 {
				return nil, errors.New("no volume details for you")
			}
			return &cinder.Volume{
				ID:     "4",
				Size:   mockVolSize / 1024,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	volumeParams := []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("0"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}, {
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("1"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}, {
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("2"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}}
	results, err := volSource.CreateVolumes(s.callCtx, volumeParams)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)
	c.Assert(results[0].Error, tc.ErrorIsNil)
	c.Assert(results[1].Error, tc.ErrorMatches, "waiting for volume to be provisioned: getting volume: no volume details for you")
	c.Assert(results[2].Error, tc.ErrorMatches, "no volume for you")
	c.Assert(numCreateCalls, tc.Equals, 3)
	c.Assert(numGetCalls, tc.Equals, 2)
	c.Assert(numDestroyCalls, tc.Equals, 1)
}

func (s *cinderVolumeSourceSuite) TestImportVolume(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   mockVolSize / 1024,
				Status: "available",
			}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	c.Assert(volSource, tc.Implements, new(storage.VolumeImporter))

	tags := map[string]string{
		"a": "b",
		"c": "d",
	}
	info, err := volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, "", tags, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, storage.VolumeInfo{
		VolumeId:   mockVolId,
		Size:       mockVolSize,
		Persistent: true,
	})
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
		{"SetVolumeMetadata", []interface{}{mockVolId, tags}},
	})
}

func (s *cinderVolumeSourceSuite) TestImportVolumeInUse(c *tc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Status: "in-use",
			}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, "", nil, false)
	c.Assert(err, tc.ErrorMatches, `cannot import volume "0" with status "in-use"`)
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestImportVolumeInvalidCredential(c *tc.C) {
	c.Assert(s.invalidCredential, tc.IsFalse)
	mockAdapter := &mockAdapter{
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{}, testUnauthorisedGooseError
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter, s.env)
	_, err := volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, "", nil, false)
	c.Assert(err, tc.ErrorMatches, `getting volume: invalid auth`)
	mockAdapter.CheckCalls(c, []testhelpers.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
	})
	c.Assert(s.invalidCredential, tc.IsTrue)
}

type mockAdapter struct {
	testhelpers.Stub
	getVolume             func(string) (*cinder.Volume, error)
	getVolumesDetail      func() ([]cinder.Volume, error)
	deleteVolume          func(string) error
	createVolume          func(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	attachVolume          func(string, string, string) (*nova.VolumeAttachment, error)
	detachVolume          func(string, string) error
	listVolumeAttachments func(string) ([]nova.VolumeAttachment, error)
	setVolumeMetadata     func(string, map[string]string) (map[string]string, error)
	listAvailabilityZones func() ([]cinder.AvailabilityZone, error)
}

func (ma *mockAdapter) GetVolume(volumeId string) (*cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolume", volumeId)
	if ma.getVolume != nil {
		return ma.getVolume(volumeId)
	}
	return &cinder.Volume{
		ID:     volumeId,
		Status: "available",
	}, nil
}

func (ma *mockAdapter) GetVolumesDetail() ([]cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolumesDetail")
	if ma.getVolumesDetail != nil {
		return ma.getVolumesDetail()
	}
	return nil, nil
}

func (ma *mockAdapter) DeleteVolume(volId string) error {
	ma.MethodCall(ma, "DeleteVolume", volId)
	if ma.deleteVolume != nil {
		return ma.deleteVolume(volId)
	}
	return nil
}

func (ma *mockAdapter) CreateVolume(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	ma.MethodCall(ma, "CreateVolume", args)
	if ma.createVolume != nil {
		return ma.createVolume(args)
	}
	return nil, errors.NotImplementedf("CreateVolume")
}

func (ma *mockAdapter) AttachVolume(serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "AttachVolume", serverId, volumeId, mountPoint)
	if ma.attachVolume != nil {
		return ma.attachVolume(serverId, volumeId, mountPoint)
	}
	return nil, errors.NotImplementedf("AttachVolume")
}

func (ma *mockAdapter) DetachVolume(serverId, attachmentId string) error {
	ma.MethodCall(ma, "DetachVolume", serverId, attachmentId)
	if ma.detachVolume != nil {
		return ma.detachVolume(serverId, attachmentId)
	}
	return nil
}

func (ma *mockAdapter) ListVolumeAttachments(serverId string) ([]nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "ListVolumeAttachments", serverId)
	if ma.listVolumeAttachments != nil {
		return ma.listVolumeAttachments(serverId)
	}
	return nil, nil
}

func (ma *mockAdapter) SetVolumeMetadata(volumeId string, metadata map[string]string) (map[string]string, error) {
	ma.MethodCall(ma, "SetVolumeMetadata", volumeId, metadata)
	if ma.setVolumeMetadata != nil {
		return ma.setVolumeMetadata(volumeId, metadata)
	}
	return nil, nil
}

func (ma *mockAdapter) ListVolumeAvailabilityZones() ([]cinder.AvailabilityZone, error) {
	ma.MethodCall(ma, "ListAvailabilityZones")
	if ma.listAvailabilityZones != nil {
		return ma.listAvailabilityZones()
	}
	return nil, gooseerrors.NewNotImplementedf(nil, nil, "ListAvailabilityZones")
}

type testEndpointResolver struct {
	authenticated   bool
	regionEndpoints map[string]identity.ServiceURLs
}

func (r *testEndpointResolver) IsAuthenticated() bool {
	return r.authenticated
}

func (r *testEndpointResolver) Authenticate() error {
	r.authenticated = true
	return nil
}

func (r *testEndpointResolver) EndpointsForRegion(region string) identity.ServiceURLs {
	if !r.authenticated {
		return identity.ServiceURLs{}
	}
	return r.regionEndpoints[region]
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointVolume(c *tc.C) {
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"west": map[string]string{"volume": "http://cinder.testing/v1"},
	}}
	url, err := openstack.GetVolumeEndpointURL(client, "west")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url.String(), tc.Equals, "http://cinder.testing/v1")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointVolumeV2(c *tc.C) {
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"west": map[string]string{"volumev2": "http://cinder.testing/v2"},
	}}
	url, err := openstack.GetVolumeEndpointURL(client, "west")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url.String(), tc.Equals, "http://cinder.testing/v2")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointV2IfNoV3(c *tc.C) {
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"south": map[string]string{
			"volume":   "http://cinder.testing/v1",
			"volumev2": "http://cinder.testing/v2",
		},
	}}
	url, err := openstack.GetVolumeEndpointURL(client, "south")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url.String(), tc.Equals, "http://cinder.testing/v2")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointPreferV3(c *tc.C) {
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"south": map[string]string{
			"volume":   "http://cinder.testing/v1",
			"volumev2": "http://cinder.testing/v2",
			"volumev3": "http://cinder.testing/v3",
		},
	}}
	url, err := openstack.GetVolumeEndpointURL(client, "south")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url.String(), tc.Equals, "http://cinder.testing/v3")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointMissing(c *tc.C) {
	client := &testEndpointResolver{}
	url, err := openstack.GetVolumeEndpointURL(client, "east")
	c.Assert(err, tc.ErrorMatches, `endpoint "volume" in region "east" not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(url, tc.IsNil)
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointBadURL(c *tc.C) {
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"north": map[string]string{"volumev2": "some %4"},
	}}
	url, err := openstack.GetVolumeEndpointURL(client, "north")
	// NOTE(achilleasa): go1.14 quotes malformed URLs in error messages
	// hence the optional quotes in the regex below.
	c.Assert(err, tc.ErrorMatches, `parse ("?)some %4("?): .*`)
	c.Assert(url, tc.IsNil)
}

func (s *cinderVolumeSourceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.env = mocks.NewMockZonedEnviron(ctrl)
	s.env.EXPECT().InstanceAvailabilityZoneNames(
		s.callCtx, []instance.Id{mockServerId},
	).Return(map[instance.Id]string{mockServerId: "zone-1"}, nil).AnyTimes()

	return ctrl
}
