// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	k8stesting "github.com/juju/juju/caas/kubernetes/testing"
	"github.com/juju/juju/core/status"
	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type FilesystemStateSuite struct {
	StorageStateSuiteBase
}

type FilesystemIAASModelSuite struct {
	FilesystemStateSuite
}

type FilesystemCAASModelSuite struct {
	FilesystemStateSuite
}

func TestFilesystemIAASModelSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &FilesystemIAASModelSuite{})
}
func TestFilesystemCAASModelSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &FilesystemCAASModelSuite{})
}

func (s *FilesystemCAASModelSuite) SetUpTest(c *tc.C) {
	s.series = "kubernetes"
	s.FilesystemStateSuite.SetUpTest(c)
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
}

func (s *FilesystemStateSuite) TestAddApplicationInvalidPool(c *tc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "storage-filesystem", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, tc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefault(c *tc.C) {
	// no pool specified, no default configured: use default.
	expected := "rootfs"
	if s.series == "kubernetes" {
		expected = "kubernetes"
	}
	s.testAddApplicationDefaultPool(c, expected, 0)
}

func (s *FilesystemStateSuite) TestAddApplicationNoPoolNoDefaultWithUnits(c *tc.C) {
	// no pool specified, no default configured: use default, add a unit during
	// app deploy.
	expected := "rootfs"
	if s.series == "kubernetes" {
		expected = "kubernetes"
	}
	s.testAddApplicationDefaultPool(c, expected, 1)
}

func (s *FilesystemIAASModelSuite) TestAddApplicationNoPoolDefaultFilesystem(c *tc.C) {
	// no pool specified, default filesystem configured: use default
	// filesystem.
	m, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = m.UpdateModelConfig(map[string]interface{}{
		"storage-default-filesystem-source": "machinescoped",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.testAddApplicationDefaultPool(c, "machinescoped", 0)
}

func (s *FilesystemIAASModelSuite) TestAddApplicationNoPoolDefaultBlock(c *tc.C) {
	// no pool specified, default block configured: use default
	// block with managed fs on top.
	m, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = m.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "modelscoped-block",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.testAddApplicationDefaultPool(c, "modelscoped-block", 0)
}

func (s *FilesystemStateSuite) testAddApplicationDefaultPool(c *tc.C, expectedPool string, numUnits int) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}

	args := state.AddApplicationArgs{
		Name:  "storage-filesystem",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage:  storage,
		NumUnits: numUnits,
	}
	app, err := s.st.AddApplication(args)
	c.Assert(err, tc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, tc.ErrorIsNil)
	expected := map[string]state.StorageConstraints{
		"data": {
			Pool:  expectedPool,
			Size:  1024,
			Count: 1,
		},
	}
	if s.series == "kubernetes" {
		expected["cache"] = state.StorageConstraints{Count: 0, Size: 1024, Pool: expectedPool}
	}
	c.Assert(cons, tc.DeepEquals, expected)

	app, err = s.st.Application(args.Name)
	c.Assert(err, tc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, numUnits)

	for _, unit := range units {
		scons, err := unit.StorageConstraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(scons, tc.DeepEquals, expected)

		storageAttachments, err := s.storageBackend.UnitStorageAttachments(unit.UnitTag())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(storageAttachments, tc.HasLen, 1)
		storageInstance, err := s.storageBackend.StorageInstance(storageAttachments[0].StorageInstance())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(storageInstance.Kind(), tc.Equals, state.StorageKindFilesystem)
	}
}

func (s *FilesystemStateSuite) TestAddFilesystemWithoutBackingVolume(c *tc.C) {
	s.addUnitWithFilesystem(c, "rootfs", false)
}

func (s *FilesystemIAASModelSuite) TestAddFilesystemWithBackingVolume(c *tc.C) {
	s.addUnitWithFilesystem(c, "modelscoped-block", true)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoImmutable(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	hostTag := s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	if _, ok := hostTag.(names.MachineTag); ok {
		machine := unitMachine(c, s.st, u)
		err := machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	filesystemInfoSet := state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"}
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, tc.ErrorIsNil)

	// The first call to SetFilesystemInfo takes the pool name from
	// the params; the second does not, but it must not change
	// either. Callers are expected to get the existing info and
	// update it, leaving immutable values intact.
	err = s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, tc.ErrorMatches, `cannot set info for filesystem ".*0/0": cannot change pool from "rootfs" to ""`)

	filesystemInfoSet.Pool = "rootfs"
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfoSet)
}

func (s *FilesystemStateSuite) maybeAssignUnit(c *tc.C, u *state.Unit) names.Tag {
	m, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)
	if m.Type() == state.ModelTypeCAAS {
		return u.UnitTag()
	}
	err = s.st.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)
	machineId, err := u.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	return names.NewMachineTag(machineId)
}

func (s *FilesystemStateSuite) TestSetFilesystemInfoNoFilesystemId(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "tmpfs-pool")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()
	s.assertFilesystemUnprovisioned(c, filesystemTag)

	filesystemInfoSet := state.FilesystemInfo{Size: 123}
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), filesystemInfoSet)
	c.Assert(err, tc.ErrorMatches, `cannot set info for filesystem ".*0/0": filesystem ID not set`)
}

func (s *FilesystemIAASModelSuite) TestVolumeFilesystem(c *tc.C) {
	filesystem, _, _ := s.addUnitWithFilesystem(c, "modelscoped-block", true)
	volumeTag, err := filesystem.Volume()
	c.Assert(err, tc.ErrorIsNil)

	volumeFilesystem := s.volumeFilesystem(c, volumeTag)
	c.Assert(volumeFilesystem.FilesystemTag(), tc.Equals, filesystem.FilesystemTag())
}

func (s *FilesystemStateSuite) addUnitWithFilesystem(c *tc.C, pool string, withVolume bool) (
	state.Filesystem,
	state.FilesystemAttachment,
	state.StorageAttachment,
) {
	filesystem, filesystemAttachment, storageAttachment := s.addUnitWithFilesystemUnprovisioned(
		c, pool, withVolume,
	)

	if machineTag, ok := filesystemAttachment.Host().(names.MachineTag); ok {
		// Machine must be provisioned before either volume or
		// filesystem can be attached.
		machine, err := s.st.Machine(machineTag.Id())
		c.Assert(err, tc.ErrorIsNil)
		err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	if withVolume {
		// Volume must be provisioned before the filesystem.
		volume := s.filesystemVolume(c, filesystem.FilesystemTag())
		err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
		c.Assert(err, tc.ErrorIsNil)

		// Volume must be attached before the filesystem.
		err = s.storageBackend.SetVolumeAttachmentInfo(
			filesystemAttachment.Host(),
			volume.VolumeTag(),
			state.VolumeAttachmentInfo{DeviceName: "sdc"},
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Filesystem must be provisioned before it can be attached.
	err := s.storageBackend.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{FilesystemId: "fs-123"},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host(),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{MountPoint: "/srv"},
	)
	c.Assert(err, tc.ErrorIsNil)

	return filesystem, filesystemAttachment, storageAttachment
}

func (s *FilesystemStateSuite) addUnitWithFilesystemUnprovisioned(c *tc.C, pool string, withVolume bool) (
	state.Filesystem,
	state.FilesystemAttachment,
	state.StorageAttachment,
) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	hostTag := s.maybeAssignUnit(c, unit)

	storageAttachments, err := s.storageBackend.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageAttachments, tc.HasLen, 1)
	storageInstance, err := s.storageBackend.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageInstance.Kind(), tc.Equals, state.StorageKindFilesystem)

	filesystem := s.storageInstanceFilesystem(c, storageInstance.StorageTag())
	filesystemStorageTag, err := filesystem.Storage()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystemStorageTag, tc.Equals, storageInstance.StorageTag())
	_, err = filesystem.Info()
	c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystem.Params()
	c.Assert(ok, tc.IsTrue)

	volume, err := s.storageBackend.StorageInstanceVolume(storageInstance.StorageTag())
	if withVolume {
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(volume.VolumeTag(), tc.Equals, names.NewVolumeTag("0"))
		volumeStorageTag, err := volume.StorageInstance()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(volumeStorageTag, tc.Equals, storageInstance.StorageTag())
		filesystemVolume, err := filesystem.Volume()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(filesystemVolume, tc.Equals, volume.VolumeTag())
		_, err = s.storageBackend.VolumeAttachment(hostTag, filesystemVolume)
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
		_, err = filesystem.Volume()
		c.Assert(errors.Cause(err), tc.Equals, state.ErrNoBackingVolume)
	}

	if s.series != "kubernetes" {
		machineTag := hostTag.(names.MachineTag)
		filesystemAttachments, err := s.storageBackend.MachineFilesystemAttachments(machineTag)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(filesystemAttachments, tc.HasLen, 1)
		c.Assert(filesystemAttachments[0].Filesystem(), tc.Equals, filesystem.FilesystemTag())
		c.Assert(filesystemAttachments[0].Host(), tc.Equals, hostTag)
		_, err = filesystemAttachments[0].Info()
		c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
		_, ok = filesystemAttachments[0].Params()
		c.Assert(ok, tc.IsTrue)

		assertMachineStorageRefs(c, s.storageBackend, machineTag)
	}

	att, err := s.storageBackend.FilesystemAttachment(hostTag, filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	return filesystem, att, storageAttachments[0]
}

func (s *FilesystemIAASModelSuite) TestWatchFilesystemAttachment(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	err := s.st.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)
	assignedMachineId, err := u.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)
	machineTag := names.NewMachineTag(assignedMachineId)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchFilesystemAttachment(machineTag, filesystemTag)
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	machine, err := s.st.Machine(assignedMachineId)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// filesystem attachment will NOT react to filesystem changes
	err = s.storageBackend.SetFilesystemInfo(filesystemTag, state.FilesystemInfo{
		FilesystemId: "fs-123",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		machineTag, filesystemTag, state.FilesystemAttachmentInfo{
			MountPoint: "/srv",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *FilesystemStateSuite) TestFilesystemInfo(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	hostTag := s.maybeAssignUnit(c, u)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemTag := filesystem.FilesystemTag()

	s.assertFilesystemUnprovisioned(c, filesystemTag)
	s.assertFilesystemAttachmentUnprovisioned(c, hostTag, filesystemTag)

	if _, ok := hostTag.(names.MachineTag); ok {
		machine, err := s.st.Machine(hostTag.Id())
		c.Assert(err, tc.ErrorIsNil)
		err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	filesystemInfo := state.FilesystemInfo{FilesystemId: "fs-123", Size: 456}
	err := s.storageBackend.SetFilesystemInfo(filesystemTag, filesystemInfo)
	c.Assert(err, tc.ErrorIsNil)
	filesystemInfo.Pool = "rootfs" // taken from params
	s.assertFilesystemInfo(c, filesystemTag, filesystemInfo)
	s.assertFilesystemAttachmentUnprovisioned(c, hostTag, filesystemTag)

	filesystemAttachmentInfo := state.FilesystemAttachmentInfo{MountPoint: "/srv"}
	err = s.storageBackend.SetFilesystemAttachmentInfo(hostTag, filesystemTag, filesystemAttachmentInfo)
	c.Assert(err, tc.ErrorIsNil)
	s.assertFilesystemAttachmentInfo(c, hostTag, filesystemTag, filesystemAttachmentInfo)
}

func (s *FilesystemIAASModelSuite) TestVolumeBackedFilesystemScope(c *tc.C) {
	_, unit, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	err := s.st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Tag(), tc.Equals, names.NewFilesystemTag("0"))
	volumeTag, err := filesystem.Volume()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeTag, tc.Equals, names.NewVolumeTag("0"))
}

func (s *FilesystemIAASModelSuite) TestWatchModelFilesystems(c *tc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, tc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchModelFilesystems()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0", "1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("4", "5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0") // last attachment removed
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchModelFilesystemAttachments(c *tc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, tc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchModelFilesystemAttachments()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0", "0:1") // initial
	wc.AssertNoChange()

	addUnit()
	wc.AssertChange("1:4", "1:5")
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	machineTag := names.NewMachineTag("0")
	err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0:0")
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0:0")
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchMachineFilesystems(c *tc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem")
	addUnit := func() *state.Unit {
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, tc.ErrorIsNil)
		return u
	}
	u := addUnit()
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchMachineFilesystems(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0/2", "0/3") // initial
	wc.AssertNoChange()

	addUnit()
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("0/2")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0/2")
	wc.AssertNoChange()

	attachments, err := s.storageBackend.FilesystemAttachments(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	for _, a := range attachments {
		err := s.storageBackend.DetachFilesystem(a.Host(), filesystemTag)
		c.Assert(err, tc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(a.Host(), filesystemTag, false)
		c.Assert(err, tc.ErrorIsNil)
	}
	wc.AssertChange("0/2") // Dying -> Dead
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystem(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	// no more changes after seeing Dead
	wc.AssertNoChange()
}

func (s *FilesystemIAASModelSuite) TestWatchMachineFilesystemAttachments(c *tc.C) {
	app := s.setupMixedScopeStorageApplication(c, "filesystem", "machinescoped", "modelscoped")
	addUnit := func(to *state.Machine) (u *state.Unit, m *state.Machine) {
		var err error
		u, err = app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		if to != nil {
			err = u.AssignToMachine(to)
			c.Assert(err, tc.ErrorIsNil)
			return u, to
		}
		err = s.st.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, tc.ErrorIsNil)
		m = unitMachine(c, s.st, u)
		return u, m
	}
	_, m0 := addUnit(nil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchMachineFilesystemAttachments(names.NewMachineTag("0"))
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("0:0/0", "0:0/1") // initial
	wc.AssertNoChange()

	addUnit(nil)
	// no change, since we're only interested in the one machine.
	wc.AssertNoChange()

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("2"))
	c.Assert(err, tc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// machine-scoped volumes.
	wc.AssertNoChange()

	removeFilesystemStorageInstance(c, s.storageBackend, names.NewFilesystemTag("0/0"))
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0:0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0:0/0") // removed
	wc.AssertNoChange()

	addUnit(m0)
	wc.AssertChange("0:0/8", "0:0/9")
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestWatchUnitFilesystems(c *tc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data":  {Count: 1, Size: 1024, Pool: "kubernetes"},
		"cache": {Count: 1, Size: 1024, Pool: "rootfs"},
	}
	app, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "mariadb", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, tc.ErrorIsNil)

	addUnit := func(app *state.Application) *state.Unit {
		var err error
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		return u
	}
	u := addUnit(app)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchUnitFilesystems(app.ApplicationTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mariadb/0/0") // initial
	wc.AssertNoChange()

	app2, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "another", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, tc.ErrorIsNil)
	addUnit(app2)
	// no change, since we're only interested in the one application.
	wc.AssertNoChange()

	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	filesystemTag := names.NewFilesystemTag("mariadb/0/0")
	removeFilesystemStorageInstance(c, s.storageBackend, filesystemTag)

	err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mariadb/0/0")
	wc.AssertNoChange()

	attachments, err := s.storageBackend.FilesystemAttachments(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	for _, a := range attachments {
		err := s.storageBackend.DetachFilesystem(a.Host(), filesystemTag)
		c.Assert(err, tc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(a.Host(), filesystemTag, false)
		c.Assert(err, tc.ErrorIsNil)
	}
	wc.AssertChange("mariadb/0/0") // Dying -> Dead
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystem(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	// no more changes after seeing Dead
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestWatchUnitFilesystemAttachments(c *tc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data":  {Count: 1, Size: 1024, Pool: "kubernetes"},
		"cache": {Count: 1, Size: 1024, Pool: "rootfs"},
	}
	app, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "mariadb", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, tc.ErrorIsNil)

	addUnit := func(app *state.Application) *state.Unit {
		var err error
		u, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		return u
	}
	addUnit(app)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := s.storageBackend.WatchUnitFilesystemAttachments(app.ApplicationTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)

	wc.AssertChange("mariadb/0:mariadb/0/0") // initial
	wc.AssertNoChange()

	app2, err := s.st.AddApplication(state.AddApplicationArgs{
		Name: "another", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Storage: storage,
	})
	c.Assert(err, tc.ErrorIsNil)
	addUnit(app2)
	// no change, since we're only interested in the one application.
	wc.AssertNoChange()

	err = s.storageBackend.DetachFilesystem(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1"))
	c.Assert(err, tc.ErrorIsNil)
	// no change, since we're only interested in attachments of
	// unit-scoped volumes.
	wc.AssertNoChange()

	removeFilesystemStorageInstance(c, s.storageBackend, names.NewFilesystemTag("mariadb/0/0"))
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("mariadb/0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mariadb/0:mariadb/0/0") // dying
	wc.AssertNoChange()

	err = s.storageBackend.RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("mariadb/0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("mariadb/0:mariadb/0/0") // removed
	wc.AssertNoChange()
}

func (s *FilesystemCAASModelSuite) TestAddExistingFilesystemVolumeBackedDuplicateVolumeId(c *tc.C) {
	// First, create a storage instance with a filesystem and set its VolumeId
	_, _, storageTag1 := s.setupSingleStorage(c, "filesystem", "kubernetes")

	volume := s.storageInstanceVolume(c, storageTag1)
	err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has the same VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "kubernetes",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123", // Same VolumeId as the first volume
	}
	_, err = s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, tc.ErrorMatches, `cannot add existing filesystem: volume with provider-id "existing-volume-123" exists, id: "0"`)
}

func (s *FilesystemCAASModelSuite) TestAddExistingFilesystemVolumeBackedUniqueVolumeId(c *tc.C) {
	// First, create a storage instance with a filesystem and set its VolumeId
	_, _, storageTag1 := s.setupSingleStorage(c, "filesystem", "kubernetes")

	volume := s.storageInstanceVolume(c, storageTag1)
	err := s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has a different VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "kubernetes",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "kubernetes",
		Size:     123,
		VolumeId: "different-volume-456", // Different VolumeId
	}
	storageTag2, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag2, tc.Equals, names.NewStorageTag("fsdata/1"))

	// Verify both the filesystem and its backing volume were created
	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag2)
	c.Assert(err, tc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsInfoOut.FilesystemId, tc.Equals, "filesystem-1")
	c.Assert(fsInfoOut.Pool, tc.Equals, "kubernetes")
	c.Assert(fsInfoOut.Size, tc.Equals, uint64(123))

	backingVolume, err := s.storageBackend.StorageInstanceVolume(storageTag2)
	c.Assert(err, tc.ErrorIsNil)
	volInfoOut, err := backingVolume.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volInfoOut.VolumeId, tc.Equals, "different-volume-456")
	c.Assert(volInfoOut.Pool, tc.Equals, "kubernetes")
	c.Assert(volInfoOut.Size, tc.Equals, uint64(123))
}

func (s *FilesystemStateSuite) TestParseFilesystemAttachmentId(c *tc.C) {
	assertValid := func(id string, m names.Tag, v names.FilesystemTag) {
		machineTag, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(machineTag, tc.Equals, m)
		c.Assert(filesystemTag, tc.Equals, v)
	}
	assertValid("0:0", names.NewMachineTag("0"), names.NewFilesystemTag("0"))
	assertValid("0:0/1", names.NewMachineTag("0"), names.NewFilesystemTag("0/1"))
	assertValid("0/lxd/0:1", names.NewMachineTag("0/lxd/0"), names.NewFilesystemTag("1"))
	assertValid("some-unit/0:1", names.NewUnitTag("some-unit/0"), names.NewFilesystemTag("1"))
}

func (s *FilesystemStateSuite) TestParseFilesystemAttachmentIdError(c *tc.C) {
	assertError := func(id, expect string) {
		_, _, err := state.ParseFilesystemAttachmentId(id)
		c.Assert(err, tc.ErrorMatches, expect)
	}
	assertError("", `invalid filesystem attachment ID ""`)
	assertError("0", `invalid filesystem attachment ID "0"`)
	assertError("0:foo", `invalid filesystem attachment ID "0:foo"`)
	assertError("bar:0", `invalid filesystem attachment ID "bar:0"`)
}

func (s *FilesystemIAASModelSuite) TestRemoveStorageInstanceDestroysAndUnassignsFilesystem(c *tc.C) {
	filesystem, filesystemAttachment, storageAttachment := s.addUnitWithFilesystem(c, "modelscoped-block", true)
	volume := s.filesystemVolume(c, filesystemAttachment.Filesystem())
	storageTag := storageAttachment.StorageInstance()
	unitTag := storageAttachment.Unit()

	err := s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host().(names.MachineTag),
		filesystem.FilesystemTag(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, tc.ErrorIsNil)

	u, err := s.st.Unit(unitTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, unitTag, false, dontWait)
	c.Assert(err, tc.ErrorIsNil)

	// The storage instance and attachment are dying, but not yet
	// removed from state. The filesystem should still be assigned.
	s.storageInstanceFilesystem(c, storageTag)
	s.storageInstanceVolume(c, storageTag)

	err = s.storageBackend.RemoveStorageAttachment(storageTag, unitTag, false)
	c.Assert(err, tc.ErrorIsNil)

	// The storage instance is now gone; the filesystem should no longer
	// be assigned to the storage.
	_, err = s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, tc.ErrorMatches, `filesystem for storage instance "data/0" not found`)
	_, err = s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, tc.ErrorMatches, `volume for storage instance "data/0" not found`)

	// The filesystem and volume should still exist. The filesystem
	// should be dying; the volume will be destroyed only once the
	// filesystem is removed.
	f := s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(f.Life(), tc.Equals, state.Dying)
	v := s.volume(c, volume.VolumeTag())
	c.Assert(v.Life(), tc.Equals, state.Alive)
}

func (s *FilesystemIAASModelSuite) TestReleaseStorageInstanceFilesystemReleasing(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Releasing(), tc.IsFalse)
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, tc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, tc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), tc.Equals, state.Dying)
	c.Assert(filesystem.Releasing(), tc.IsTrue)
}

func (s *FilesystemIAASModelSuite) TestReleaseStorageInstanceFilesystemUnreleasable(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-unreleasable")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	c.Assert(filesystem.Releasing(), tc.IsFalse)
	err := s.storageBackend.SetFilesystemInfo(filesystem.FilesystemTag(), state.FilesystemInfo{FilesystemId: "vol-123"})
	c.Assert(err, tc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storageTag, true, false, dontWait)
	c.Assert(err, tc.ErrorMatches,
		`cannot release storage "data/0": storage provider "modelscoped-unreleasable" does not support releasing storage`)
	err = s.storageBackend.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, tc.ErrorIsNil)

	// The filesystem should should be dying, and releasing.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), tc.Equals, state.Alive)
	c.Assert(filesystem.Releasing(), tc.IsFalse)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemAttachmentInfoFilesystemNotProvisioned(c *tc.C) {
	_, filesystemAttachment, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host().(names.MachineTag),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, tc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: filesystem "0/0" not provisioned`)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemAttachmentInfoMachineNotProvisioned(c *tc.C) {
	_, filesystemAttachment, _ := s.addUnitWithFilesystemUnprovisioned(c, "rootfs", false)
	err := s.storageBackend.SetFilesystemInfo(
		filesystemAttachment.Filesystem(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemAttachmentInfo(
		filesystemAttachment.Host(),
		filesystemAttachment.Filesystem(),
		state.FilesystemAttachmentInfo{},
	)
	c.Assert(err, tc.ErrorMatches, `cannot set info for filesystem attachment 0/0:0: machine 0 not provisioned`)
}

func (s *FilesystemIAASModelSuite) TestSetFilesystemInfoVolumeAttachmentNotProvisioned(c *tc.C) {
	filesystem, _, _ := s.addUnitWithFilesystemUnprovisioned(c, "modelscoped-block", true)
	err := s.storageBackend.SetFilesystemInfo(
		filesystem.FilesystemTag(),
		state.FilesystemInfo{Size: 123, FilesystemId: "fs-id"},
	)
	c.Assert(err, tc.ErrorMatches, `cannot set info for filesystem "0": backing volume "0" is not attached`)
}

func (s *FilesystemIAASModelSuite) TestDestroyFilesystem(c *tc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	assertDestroy := func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}
	defer state.SetBeforeHooks(c, s.st, assertDestroy).Check()
	assertDestroy()
}

func (s *FilesystemStateSuite) TestDestroyFilesystemNotFound(c *tc.C) {
	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0"), false)
	c.Assert(err, tc.ErrorMatches, `destroying filesystem 0: filesystem "0" not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemStorageAssignedNoForce(c *tc.C) {
	// Create a filesystem-type storage instance, and show that we
	// cannot destroy the filesystem while there is storage assigned.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	err := s.storageBackend.DestroyFilesystem(filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorMatches, "destroying filesystem .*0/0: filesystem is assigned to storage data/0")

	// We must destroy the unit before we can remove the storage.
	err = u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
}

func (s *FilesystemStateSuite) TestDestroyFilesystemStorageAssignedWithForce(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	err := s.storageBackend.DestroyFilesystem(filesystem.FilesystemTag(), true)
	c.Assert(err, tc.ErrorIsNil)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), tc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestDestroyFilesystemNoAttachments(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
		c.Assert(err, tc.ErrorIsNil)
		assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	}).Check()

	// There are no more attachments, so the filesystem should
	// be progressed directly to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystem(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	assertRemove := func() {
		err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}
	defer state.SetBeforeHooks(c, s.st, assertRemove).Check()
	assertRemove()
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemVolumeBacked(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())
	assertVolumeLife := func(life state.Life) {
		volume := s.volume(c, volume.VolumeTag())
		c.Assert(volume.Life(), tc.Equals, life)
	}
	assertVolumeAttachmentLife := func(life state.Life) {
		attachment := s.volumeAttachment(c, machine.MachineTag(), volume.VolumeTag())
		c.Assert(attachment.Life(), tc.Equals, life)
	}

	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	// Destroying the filesystem does not trigger destruction
	// of the volume. It cannot be destroyed until all remnants
	// of the filesystem are gone.
	assertVolumeLife(state.Alive)

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	// Likewise for the volume attachment.
	assertVolumeAttachmentLife(state.Alive)

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	// Removing the filesystem attachment causes the backing-volume
	// to be detached.
	assertVolumeAttachmentLife(state.Dying)

	// Removing the last attachment should cause the filesystem
	// to be removed, since it is volume-backed and dying.
	_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	// Removing the filesystem causes the backing-volume to be
	// destroyed.
	assertVolumeLife(state.Dying)

	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemIAASModelSuite) TestFilesystemVolumeBackedDestroyDetachVolumeFail(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")
	volume := s.filesystemVolume(c, filesystem.FilesystemTag())

	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	// Can't destroy (detach) volume until the filesystem (attachment) is removed.
	err = s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorMatches, "detaching volume 0 from machine 0: volume contains attached filesystem")
	c.Assert(err, tc.Satisfies, state.IsContainsFilesystem)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorMatches, "destroying volume 0: volume contains filesystem")
	c.Assert(err, tc.Satisfies, state.IsContainsFilesystem)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	err = s.storageBackend.DetachVolume(machine.MachineTag(), volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(volume.VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestRemoveFilesystemNotFound(c *tc.C) {
	err := s.storageBackend.RemoveFilesystem(names.NewFilesystemTag("42"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemNotDead(c *tc.C) {
	filesystem, _ := s.setupFilesystemAttachment(c, "rootfs")
	err := s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	err = s.storageBackend.RemoveFilesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorMatches, "removing filesystem 0/0: filesystem is not dead")
}

func (s *FilesystemIAASModelSuite) TestDetachFilesystem(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	assertDetach := func() {
		err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, tc.ErrorIsNil)
		attachment := s.filesystemAttachment(c, machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(attachment.Life(), tc.Equals, state.Dying)
	}
	defer state.SetBeforeHooks(c, s.st, assertDetach).Check()
	assertDetach()
}

func (s *FilesystemIAASModelSuite) TestRemoveLastFilesystemAttachment(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The filesystem has no attachments, so it should go straight to Dead.
	s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dead)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemIAASModelSuite) TestRemoveLastFilesystemAttachmentConcurrently(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		s.assertDestroyFilesystem(c, filesystem.FilesystemTag(), state.Dying)
	}).Check()

	err = s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorIsNil)

	// Last attachment was removed, and the filesystem was (concurrently)
	// destroyed, so the filesystem should be Dead.
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), tc.Equals, state.Dead)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
}

func (s *FilesystemStateSuite) TestRemoveFilesystemAttachmentNotFound(c *tc.C) {
	err := s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("42"), names.NewFilesystemTag("42"), false)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(err, tc.ErrorMatches, `removing attachment of filesystem 42 from machine 42: filesystem "42" on "machine 42" not found`)
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemAttachmentConcurrently(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")
	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	remove := func() {
		err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
		c.Assert(err, tc.ErrorIsNil)
		assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	}
	defer state.SetBeforeHooks(c, s.st, remove).Check()
	remove()
}

func (s *FilesystemIAASModelSuite) TestRemoveFilesystemAttachmentAlive(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	err := s.storageBackend.RemoveFilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag(), false)
	c.Assert(err, tc.ErrorMatches, "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying")
}

func (s *FilesystemIAASModelSuite) TestRemoveMachineRemovesFilesystems(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	c.Assert(machine.Destroy(), tc.ErrorIsNil)
	c.Assert(machine.EnsureDead(), tc.ErrorIsNil)
	c.Assert(machine.Remove(), tc.ErrorIsNil)

	// Machine is gone: filesystem should be gone too.
	_, err := s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	attachments, err := s.storageBackend.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachments, tc.HasLen, 0)
}

func (s *FilesystemIAASModelSuite) TestDestroyMachineRemovesNonDetachableFilesystems(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems to be destroyed, detached, and
	// finally removed.
	c.Assert(machine.Destroy(), tc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	_, err := s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemIAASModelSuite) TestDestroyMachineDetachesDetachableFilesystems(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystems to be detached, but not destroyed.
	c.Assert(machine.Destroy(), tc.ErrorIsNil)
	assertCleanupRuns(c, s.st)
	s.testfilesystemDetached(
		c, machine.MachineTag(), filesystem.FilesystemTag(),
	)
}

// TODO(caas) - destroy caas storage when unit dies
func (s *FilesystemIAASModelSuite) TestDestroyHostDetachesDetachableFilesystems(c *tc.C) {
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "modelscoped-block")
	hostTag := s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// Destroying the unit should, if necessary, destroy its host machine, which
	// triggers the detachment of storage.
	s.obliterateUnit(c, u.UnitTag())
	assertCleanupRuns(c, s.st)

	s.testfilesystemDetached(
		c, hostTag, filesystem.FilesystemTag(),
	)
}

func (s *FilesystemStateSuite) testfilesystemDetached(
	c *tc.C,
	hostTag names.Tag,
	filesystemTag names.FilesystemTag,
) {
	// Filesystem is still alive...
	filesystem, err := s.storageBackend.Filesystem(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystem.Life(), tc.Equals, state.Alive)

	// ... but it has been detached.
	_, err = s.storageBackend.FilesystemAttachment(hostTag, filesystemTag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	filesystemStatus, err := filesystem.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystemStatus.Status, tc.Equals, status.Detached)
	c.Assert(filesystemStatus.Message, tc.Equals, "")
}

func (s *FilesystemIAASModelSuite) TestDestroyManualMachineDoesntRemoveNonDetachableFilesystems(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "loop")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "", "manual:machine", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// non-detachable filesystems and attachments to be set to Dying,
	// but not completely removed.
	c.Assert(machine.Destroy(), tc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	filesystem, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystem.Life(), tc.Equals, state.Dying)
	attachment, err := s.storageBackend.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachment.Life(), tc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestDestroyManualMachineDoesntDetachDetachableFilesystems(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped-block")

	// Make this a manual machine, so the cleanup.
	err := machine.SetProvisioned("inst-id", "", "manual:machine", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Destroy the machine and run cleanups, which should cause the
	// detachable filesystem attachments to be set to Dying, but not
	// completely removed. The filesystem itself should be left Alive.
	c.Assert(machine.Destroy(), tc.ErrorIsNil)
	assertCleanupRuns(c, s.st)

	filesystem, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystem.Life(), tc.Equals, state.Alive)
	attachment, err := s.storageBackend.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachment.Life(), tc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestFilesystemMachineScoped(c *tc.C) {
	// Machine-scoped filesystems created unassigned to a storage
	// instance are bound to the machine.
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")

	err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, tc.ErrorMatches, "detaching filesystem 0/0 from machine 0: filesystem is not detachable")
	err = machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.storageBackend.Filesystem(filesystem.FilesystemTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.storageBackend.FilesystemAttachment(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
	)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *FilesystemStateSuite) TestFilesystemRemoveStorageDestroysFilesystem(c *tc.C) {
	// Filesystems created assigned to a storage instance are bound
	// to the machine/model, and not the storage. i.e. storage is
	// persistent by default.
	_, u, storageTag := s.setupSingleStorage(c, "filesystem", "rootfs")
	s.maybeAssignUnit(c, u)
	filesystem := s.storageInstanceFilesystem(c, storageTag)

	// The filesystem should transition to Dying when the storage is removed.
	// We must destroy the unit before we can remove the storage.
	err := u.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	removeStorageInstance(c, s.storageBackend, storageTag)
	filesystem = s.filesystem(c, filesystem.FilesystemTag())
	c.Assert(filesystem.Life(), tc.Equals, state.Dying)
}

func (s *FilesystemIAASModelSuite) TestEnsureMachineDeadAddFilesystemConcurrently(c *tc.C) {
	_, machine := s.setupFilesystemAttachment(c, "rootfs")
	addFilesystem := func() {
		_, u, _ := s.setupSingleStorage(c, "filesystem", "rootfs")
		err := u.AssignToMachine(machine)
		c.Assert(err, tc.ErrorIsNil)
		s.obliterateUnit(c, u.UnitTag())
	}
	defer state.SetBeforeHooks(c, s.st, addFilesystem).Check()

	// Adding another filesystem to the machine will cause EnsureDead to
	// retry, but it will succeed because both filesystems are inherently
	// machine-bound.
	err := machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FilesystemIAASModelSuite) TestEnsureMachineDeadRemoveFilesystemConcurrently(c *tc.C) {
	filesystem, machine := s.setupFilesystemAttachment(c, "rootfs")
	removeFilesystem := func() {
		s.obliterateFilesystem(c, filesystem.FilesystemTag())
	}
	defer state.SetBeforeHooks(c, s.st, removeFilesystem).Check()

	// Removing a filesystem concurrently does not cause a transaction failure.
	err := machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsSingletonNoLocation(c *tc.C) {
	s.testFilesystemAttachmentParams(c, 0, 1, "", state.FilesystemAttachmentParams{
		Location: "/var/lib/juju/storage/data/0",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsMultipleNoLocation(c *tc.C) {
	s.testFilesystemAttachmentParams(c, 0, -1, "", state.FilesystemAttachmentParams{
		Location: "/var/lib/juju/storage/data/0",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsSingletonLocation(c *tc.C) {
	s.testFilesystemAttachmentParams(c, 0, 1, "/srv", state.FilesystemAttachmentParams{
		Location: "/srv",
	})
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsMultipleLocation(c *tc.C) {
	s.testFilesystemAttachmentParams(c, 0, -1, "/srv", state.FilesystemAttachmentParams{
		Location: "/srv/data/0",
	})
}

func (s *FilesystemStateSuite) testFilesystemAttachmentParams(
	c *tc.C, countMin, countMax int, location string,
	expect state.FilesystemAttachmentParams,
) {
	ch := s.createStorageCharmWithSeries(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: countMin,
		CountMax: countMax,
		Location: location,
	}, s.series)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("rootfs", 1024, 1),
	}

	app := s.AddTestingApplicationWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	hostTag := s.maybeAssignUnit(c, unit)

	storageTag := names.NewStorageTag("data/0")
	filesystem := s.storageInstanceFilesystem(c, storageTag)
	filesystemAttachment := s.filesystemAttachment(
		c, hostTag, filesystem.FilesystemTag(),
	)
	params, ok := filesystemAttachment.Params()
	c.Assert(ok, tc.IsTrue)
	c.Assert(params, tc.DeepEquals, expect)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationConflictConcurrent(c *tc.C) {
	s.testFilesystemAttachmentParamsConcurrent(
		c, "/srv", "/srv",
		`cannot assign unit "storage-filesystem-after/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for "data" storage contains mount point "/srv" for "data" storage`)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationAutoConcurrent(c *tc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "", "")
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsLocationAutoAndManualConcurrent(c *tc.C) {
	s.testFilesystemAttachmentParamsConcurrent(c, "", "/srv", "")
}

func (s *FilesystemStateSuite) testFilesystemAttachmentParamsConcurrent(c *tc.C, locBefore, locAfter, expectErr string) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("rootfs", 1024, 1),
	}

	deploy := func(rev int, location, applicationname string) error {
		ch := s.createStorageCharmRev(c, "storage-filesystem", charm.Storage{
			Name:     "data",
			Type:     charm.StorageFilesystem,
			CountMin: 1,
			CountMax: 1,
			Location: location,
		}, rev)
		app := s.AddTestingApplicationWithStorage(c, applicationname, ch, storage)
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		return unit.AssignToMachine(machine)
	}

	defer state.SetBeforeHooks(c, s.st, func() {
		err := deploy(1, locBefore, "storage-filesystem-before")
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err = deploy(2, locAfter, "storage-filesystem-after")
	if expectErr != "" {
		c.Assert(err, tc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentParamsConcurrentRemove(c *tc.C) {
	// this creates a filesystem mounted at "/srv".
	filesystem, machine := s.setupFilesystemAttachment(c, "modelscoped")

	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/not/in/srv",
	})
	app := s.AddTestingApplication(c, "storage-filesystem", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.st, func() {
		err := s.storageBackend.DetachFilesystem(machine.MachineTag(), filesystem.FilesystemTag())
		c.Assert(err, tc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(
			machine.MachineTag(), filesystem.FilesystemTag(), false,
		)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FilesystemStateSuite) TestFilesystemAttachmentParamsLocationStorageDir(c *tc.C) {
	ch := s.createStorageCharmWithSeries(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/var/lib/juju/storage",
	}, s.series)
	app := s.AddTestingApplication(c, "storage-filesystem", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	if s.series != "kubernetes" {
		c.Assert(err, tc.ErrorIsNil)
		err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	}
	c.Assert(err, tc.ErrorMatches, `.*`+
		`getting filesystem mount point for storage data: `+
		`invalid location "/var/lib/juju/storage": `+
		`must not fall within "/var/lib/juju/storage"`)
}

func (s *FilesystemIAASModelSuite) TestFilesystemAttachmentLocationConflict(c *tc.C) {
	// this creates a filesystem mounted at "/srv".
	_, machine := s.setupFilesystemAttachment(c, "rootfs")

	ch := s.createStorageCharm(c, "storage-filesystem", charm.Storage{
		Name:     "data",
		Type:     charm.StorageFilesystem,
		CountMin: 1,
		CountMax: 1,
		Location: "/srv/within",
	})
	app := s.AddTestingApplication(c, "storage-filesystem", ch)

	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, tc.ErrorMatches,
		`cannot assign unit "storage-filesystem/0" to machine 0: `+
			`validating filesystem mount points: `+
			`mount point "/srv" for filesystem 0/0 contains `+
			`mount point "/srv/within" for "data" storage`)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystem(c *tc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool:         "modelscoped",
		Size:         123,
		FilesystemId: "foo",
	}
	storageTag, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag, tc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, tc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsInfoOut, tc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsStatus.Status, tc.Equals, status.Detached)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemEmptyFilesystemId(c *tc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped",
		Size: 123,
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, nil, "pgdata")
	c.Assert(err, tc.ErrorMatches, "cannot add existing filesystem: empty filesystem ID not valid")
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBacked(c *tc.C) {
	fsInfoIn := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfoIn := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "foo",
	}
	storageTag, err := s.storageBackend.AddExistingFilesystem(fsInfoIn, &volInfoIn, "pgdata")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag, tc.Equals, names.NewStorageTag("pgdata/0"))

	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag)
	c.Assert(err, tc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, tc.ErrorIsNil)
	fsInfoIn.FilesystemId = "filesystem-0" // set by AddExistingFilesystem
	c.Assert(fsInfoOut, tc.DeepEquals, fsInfoIn)

	fsStatus, err := filesystem.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsStatus.Status, tc.Equals, status.Detached)

	volume, err := s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, tc.ErrorIsNil)
	volInfoOut, err := volume.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volInfoOut, tc.DeepEquals, volInfoIn)

	volStatus, err := volume.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volStatus.Status, tc.Equals, status.Detached)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedVolumeInfoMissing(c *tc.C) {
	fsInfo := state.FilesystemInfo{
		Pool:         "modelscoped-block",
		Size:         123,
		FilesystemId: "foo",
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, nil, "pgdata")
	c.Assert(err, tc.ErrorMatches, "cannot add existing filesystem: backing volume info missing")
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBackedFilesystemIdSupplied(c *tc.C) {
	fsInfo := state.FilesystemInfo{
		Pool:         "modelscoped-block",
		Size:         123,
		FilesystemId: "foo",
	}
	volInfo := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "foo",
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
	c.Assert(err, tc.ErrorMatches, "cannot add existing filesystem: non-empty filesystem ID with backing volume not valid")
}

func (s *FilesystemStateSuite) TestAddExistingFilesystemVolumeBackedEmptyVolumeId(c *tc.C) {
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo := state.VolumeInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	_, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo, "pgdata")
	c.Assert(err, tc.ErrorMatches, "cannot add existing filesystem: empty backing volume ID not valid")
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedDuplicateVolumeId(c *tc.C) {
	// First, create a storage instance with a block device and set its VolumeId
	_, u, storageTag1 := s.setupSingleStorage(c, "block", "modelscoped-block")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag1)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has the same VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123", // Same VolumeId as the first volume
	}
	_, err = s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, tc.ErrorMatches, `cannot add existing filesystem: volume with provider-id "existing-volume-123" exists, id: "0"`)
}

func (s *FilesystemIAASModelSuite) TestAddExistingFilesystemVolumeBackedUniqueVolumeId(c *tc.C) {
	// First, create a storage instance with a block device and set its VolumeId
	_, u, storageTag1 := s.setupSingleStorage(c, "block", "modelscoped-block")
	err := s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	volume := s.storageInstanceVolume(c, storageTag1)
	err = s.storageBackend.SetVolumeInfo(volume.VolumeTag(), state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "existing-volume-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now try to add a filesystem with a backing volume that has a different VolumeId
	fsInfo := state.FilesystemInfo{
		Pool: "modelscoped-block",
		Size: 123,
	}
	volInfo2 := state.VolumeInfo{
		Pool:     "modelscoped-block",
		Size:     123,
		VolumeId: "different-volume-456", // Different VolumeId
	}
	storageTag2, err := s.storageBackend.AddExistingFilesystem(fsInfo, &volInfo2, "fsdata")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageTag2, tc.Equals, names.NewStorageTag("fsdata/1"))

	// Verify both the filesystem and its backing volume were created
	filesystem, err := s.storageBackend.StorageInstanceFilesystem(storageTag2)
	c.Assert(err, tc.ErrorIsNil)
	fsInfoOut, err := filesystem.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsInfoOut.FilesystemId, tc.Equals, "filesystem-0")
	c.Assert(fsInfoOut.Pool, tc.Equals, "modelscoped-block")
	c.Assert(fsInfoOut.Size, tc.Equals, uint64(123))

	backingVolume, err := s.storageBackend.StorageInstanceVolume(storageTag2)
	c.Assert(err, tc.ErrorIsNil)
	volInfoOut, err := backingVolume.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volInfoOut.VolumeId, tc.Equals, "different-volume-456")
	c.Assert(volInfoOut.Pool, tc.Equals, "modelscoped-block")
	c.Assert(volInfoOut.Size, tc.Equals, uint64(123))
}

func (s *FilesystemStateSuite) setupFilesystemAttachment(c *tc.C, pool string) (state.Filesystem, *state.Machine) {
	machine, err := s.st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: pool, Size: 1024},
			Attachment: state.FilesystemAttachmentParams{
				Location: "/srv",
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	sb, err := state.NewStorageBackend(s.st)
	c.Assert(err, tc.ErrorIsNil)
	attachments, err := sb.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachments, tc.HasLen, 1)
	c.Assert(err, tc.ErrorIsNil)
	assertMachineStorageRefs(c, s.storageBackend, machine.MachineTag())
	return s.filesystem(c, attachments[0].Filesystem()), machine
}

func removeFilesystemStorageInstance(c *tc.C, sb *state.StorageBackend, filesystemTag names.FilesystemTag) {
	filesystem, err := sb.Filesystem(filesystemTag)
	c.Assert(err, tc.ErrorIsNil)
	storageTag, err := filesystem.Storage()
	c.Assert(err, tc.ErrorIsNil)
	removeStorageInstance(c, sb, storageTag)
}

func (s *FilesystemStateSuite) assertDestroyFilesystem(c *tc.C, tag names.FilesystemTag, life state.Life) {
	err := s.storageBackend.DestroyFilesystem(tag, false)
	c.Assert(err, tc.ErrorIsNil)
	filesystem := s.filesystem(c, tag)
	c.Assert(filesystem.Life(), tc.Equals, life)
}
