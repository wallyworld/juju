// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	jujuversion "github.com/juju/juju/version"
)

// Constraints stores megabytes by default for memory and root disk.
const (
	gig uint64 = 1024

	addedHistoryCount = 5
	// 6 for the one initial + 5 added.
	expectedHistoryCount = addedHistoryCount + 1
)

var testAnnotations = map[string]string{
	"string":  "value",
	"another": "one",
}

type MigrationBaseSuite struct {
	ConnWithWallClockSuite
}

func (s *MigrationBaseSuite) setLatestTools(c *tc.C, latestTools version.Number) {
	err := s.Model.UpdateLatestToolsVersion(latestTools)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MigrationBaseSuite) setRandSequenceValue(c *tc.C, name string) int {
	var value int
	var err error
	count := rand.Intn(5) + 1
	for i := 0; i < count; i++ {
		value, err = state.Sequence(s.State, name)
		c.Assert(err, tc.ErrorIsNil)
	}
	// The value stored in the doc is one higher than what it returns.
	return value + 1
}

func (s *MigrationBaseSuite) primeStatusHistory(c *tc.C, entity statusSetter, statusVal status.Status, count int) {
	primeStatusHistory(c, s.StatePool.Clock(), entity, statusVal, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"index": count - i}
	}, 0, "")
}

func (s *MigrationBaseSuite) makeApplicationWithUnits(c *tc.C, applicationName string, count int) {
	units := make([]*state.Unit, count)
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: applicationName,
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: applicationName,
		}),
	})
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{
			Application: application,
		})
	}
}

func (s *MigrationBaseSuite) makeUnitWithStorage(c *tc.C) (*state.Application, *state.Unit, names.StorageTag) {
	pool := "modelscoped"
	kind := "block"
	// Create a default pool for block devices.
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create(pool, provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)

	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	machine := s.Factory.MakeMachine(c, nil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(err, tc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	agentVersion := version.MustParseBinary("2.0.1-ubuntu-and64")
	err = unit.SetAgentVersion(agentVersion)
	c.Assert(err, tc.ErrorIsNil)
	return application, unit, storageTag
}

type MigrationExportSuite struct {
	MigrationBaseSuite
}

func TestMigrationExportSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MigrationExportSuite{})
}

func (s *MigrationExportSuite) SetUpTest(c *tc.C) {
	s.MigrationBaseSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.StrictMigration)
}

func (s *MigrationExportSuite) checkStatusHistory(c *tc.C, history []description.Status, statusVal status.Status) {
	for i, st := range history {
		c.Logf("status history #%d: %s", i, st.Updated())
		c.Check(st.Value(), tc.Equals, string(statusVal))
		c.Check(st.Message(), tc.Equals, "")
		c.Check(st.Data(), tc.DeepEquals, map[string]interface{}{"index": i + 1})
	}
}

func (s *MigrationExportSuite) TestModelInfo(c *tc.C) {
	err := s.Model.SetAnnotations(s.Model, testAnnotations)
	c.Assert(err, tc.ErrorIsNil)
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	err = s.State.SetModelConstraints(constraints.MustParse("arch=amd64 mem=8G"))
	c.Assert(err, tc.ErrorIsNil)
	machineSeq := s.setRandSequenceValue(c, "machine")
	fooSeq := s.setRandSequenceValue(c, "application-foo")

	err = s.State.SwitchBlockOn(state.ChangeBlock, "locked down")
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.SetPassword("supppperrrrsecret1235556667777")
	c.Assert(err, tc.ErrorIsNil)

	environVersion := 123
	err = s.Model.SetEnvironVersion(environVersion)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.PasswordHash(), tc.Equals, utils.AgentPasswordHash("supppperrrrsecret1235556667777"))
	c.Assert(model.Type(), tc.Equals, string(s.Model.Type()))
	c.Assert(model.Tag(), tc.Equals, s.Model.ModelTag())
	c.Assert(model.Owner(), tc.Equals, s.Model.Owner())
	dbModelCfg, err := s.Model.Config()
	c.Assert(err, tc.ErrorIsNil)
	modelAttrs := dbModelCfg.AllAttrs()
	modelCfg := model.Config()
	// Config as read from state has resources tags coerced to a map.
	modelCfg["resource-tags"] = map[string]string{}
	c.Assert(modelCfg, tc.DeepEquals, modelAttrs)
	c.Assert(model.LatestToolsVersion(), tc.Equals, latestTools)
	c.Assert(model.EnvironVersion(), tc.Equals, environVersion)
	c.Assert(model.Annotations(), tc.DeepEquals, testAnnotations)
	constraints := model.Constraints()
	c.Assert(constraints, tc.NotNil)
	c.Assert(constraints.Architecture(), tc.Equals, "amd64")
	c.Assert(constraints.Memory(), tc.Equals, 8*gig)
	c.Assert(model.Sequences(), tc.DeepEquals, map[string]int{
		"machine":         machineSeq,
		"application-foo": fooSeq,
		// blocks is added by the switch block on call above.
		"block": 1,
	})
	c.Assert(model.Blocks(), tc.DeepEquals, map[string]string{
		"all-changes": "locked down",
	})
}

func (s *MigrationExportSuite) TestModelUsers(c *tc.C) {
	// Make sure we have some last connection times for the admin user,
	// and create a few other users.
	lastConnection := state.NowToTheSecond(s.State)
	owner, err := s.State.UserAccess(s.Owner, s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(s.State, owner, lastConnection)
	c.Assert(err, tc.ErrorIsNil)

	bobTag := names.NewUserTag("bob@external")
	bob, err := s.Model.AddUser(state.UserAccessSpec{
		User:      bobTag,
		CreatedBy: s.Owner,
		Access:    permission.ReadAccess,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(s.State, bob, lastConnection)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	users := model.Users()
	c.Assert(users, tc.HasLen, 2)

	exportedBob := users[0]
	// admin is "test-admin", and results are sorted
	exportedAdmin := users[1]

	c.Assert(exportedAdmin.Name(), tc.Equals, s.Owner)
	c.Assert(exportedAdmin.DisplayName(), tc.Equals, owner.DisplayName)
	c.Assert(exportedAdmin.CreatedBy(), tc.Equals, s.Owner)
	c.Assert(exportedAdmin.DateCreated(), tc.Equals, owner.DateCreated)
	c.Assert(exportedAdmin.LastConnection(), tc.Equals, lastConnection)
	c.Assert(exportedAdmin.Access(), tc.Equals, "admin")

	c.Assert(exportedBob.Name(), tc.Equals, bobTag)
	c.Assert(exportedBob.DisplayName(), tc.Equals, "")
	c.Assert(exportedBob.CreatedBy(), tc.Equals, s.Owner)
	c.Assert(exportedBob.DateCreated(), tc.Equals, bob.DateCreated)
	c.Assert(exportedBob.LastConnection(), tc.Equals, lastConnection)
	c.Assert(exportedBob.Access(), tc.Equals, "read")
}

func (s *MigrationExportSuite) TestSLAs(c *tc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	sla := model.SLA()

	c.Assert(sla.Level(), tc.Equals, "essential")
	c.Assert(sla.Credentials(), tc.DeepEquals, "creds")
}

func (s *MigrationExportSuite) TestMachines(c *tc.C) {
	s.assertMachinesMigrated(c, constraints.MustParse("arch=amd64 mem=8G tags=foo,bar spaces=dmz"))
}

func (s *MigrationExportSuite) TestMachinesWithVirtConstraint(c *tc.C) {
	s.assertMachinesMigrated(c, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationExportSuite) TestMachinesWithRootDiskSourceConstraint(c *tc.C) {
	s.assertMachinesMigrated(c, constraints.MustParse("arch=amd64 mem=8G root-disk-source=aldous"))
}

func (s *MigrationExportSuite) assertMachinesMigrated(c *tc.C, cons constraints.Value) {
	// Add a machine with an LXC container.
	source := "vashti"
	displayName := "test-display-name"

	addr := network.NewSpaceAddress("1.1.1.1")
	addr.SpaceID = "0"

	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: cons,
		Characteristics: &instance.HardwareCharacteristics{
			RootDiskSource: &source,
		},
		DisplayName: displayName,
		Addresses:   network.SpaceAddresses{addr},
	})
	nested := s.Factory.MakeMachineNested(c, machine1.Id(), nil)

	err := s.Model.SetAnnotations(machine1, testAnnotations)
	c.Assert(err, tc.ErrorIsNil)
	s.primeStatusHistory(c, machine1, status.Started, addedHistoryCount)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	machines := model.Machines()
	c.Assert(machines, tc.HasLen, 1)

	exported := machines[0]
	c.Assert(exported.Tag(), tc.Equals, machine1.MachineTag())
	c.Assert(exported.Base(), tc.Equals, machine1.Base().String())
	c.Assert(exported.Annotations(), tc.DeepEquals, testAnnotations)

	expCons := exported.Constraints()
	c.Assert(expCons, tc.NotNil)
	c.Assert(expCons.Architecture(), tc.Equals, *cons.Arch)
	c.Assert(expCons.Memory(), tc.Equals, *cons.Mem)
	if cons.HasVirtType() {
		c.Assert(expCons.VirtType(), tc.Equals, *cons.VirtType)
	}
	if cons.HasRootDiskSource() {
		c.Assert(expCons.RootDiskSource(), tc.Equals, *cons.RootDiskSource)
	}
	if cons.HasRootDisk() {
		c.Assert(expCons.RootDisk(), tc.Equals, *cons.RootDisk)
	}

	tools, err := machine1.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	exTools := exported.Tools()
	c.Assert(exTools, tc.NotNil)
	c.Assert(exTools.Version(), tc.DeepEquals, tools.Version)

	history := exported.StatusHistory()
	c.Assert(history, tc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, history[:addedHistoryCount], status.Started)

	containers := exported.Containers()
	c.Assert(containers, tc.HasLen, 1)
	container := containers[0]
	c.Assert(container.Tag(), tc.Equals, nested.MachineTag())

	// Ensure that a new machine has a modification set to its initial state.
	inst := exported.Instance()
	c.Assert(inst.ModificationStatus().Value(), tc.Equals, "idle")
	c.Assert(inst.RootDiskSource(), tc.Equals, "vashti")
	c.Assert(inst.DisplayName(), tc.Equals, displayName)

	c.Assert(exported.ProviderAddresses(), tc.HasLen, 1)
	exAddr := exported.ProviderAddresses()[0]
	c.Assert(exAddr.Value(), tc.Equals, "1.1.1.1")
	c.Assert(exAddr.SpaceID(), tc.Equals, "0")
}

func (s *MigrationExportSuite) TestMachineDevices(c *tc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	// Create two devices, first with all fields set, second just to show that
	// we do both.
	sda := state.BlockDeviceInfo{
		DeviceName:     "sda",
		DeviceLinks:    []string{"some", "data"},
		Label:          "sda-label",
		UUID:           "some-uuid",
		HardwareId:     "magic",
		WWN:            "drbr",
		BusAddress:     "bus stop",
		Size:           16 * 1024 * 1024 * 1024,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
	}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb", MountPoint: "/var/lib/lxd"}
	err := machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
	machines := model.Machines()
	c.Assert(machines, tc.HasLen, 1)
	exported := machines[0]

	devices := exported.BlockDevices()
	c.Assert(devices, tc.HasLen, 2)
	ex1, ex2 := devices[0], devices[1]

	c.Check(ex1.Name(), tc.Equals, "sda")
	c.Check(ex1.Links(), tc.DeepEquals, []string{"some", "data"})
	c.Check(ex1.Label(), tc.Equals, "sda-label")
	c.Check(ex1.UUID(), tc.Equals, "some-uuid")
	c.Check(ex1.HardwareID(), tc.Equals, "magic")
	c.Check(ex1.WWN(), tc.Equals, "drbr")
	c.Check(ex1.BusAddress(), tc.Equals, "bus stop")
	c.Check(ex1.Size(), tc.Equals, uint64(16*1024*1024*1024))
	c.Check(ex1.FilesystemType(), tc.Equals, "ext4")
	c.Check(ex1.InUse(), tc.IsTrue)
	c.Check(ex1.MountPoint(), tc.Equals, "/")

	c.Check(ex2.Name(), tc.Equals, "sdb")
	c.Check(ex2.MountPoint(), tc.Equals, "/var/lib/lxd")
}

func (s *MigrationExportSuite) TestApplications(c *tc.C) {
	s.assertMigrateApplications(c, false, s.State, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationExportSuite) TestCAASLegacyApplications(c *tc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { caasSt.Close() })

	s.assertMigrateApplications(c, false, caasSt, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationExportSuite) TestCAASSidecarApplications(c *tc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { caasSt.Close() })

	s.assertMigrateApplications(c, true, caasSt, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationExportSuite) TestApplicationsWithVirtConstraint(c *tc.C) {
	s.assertMigrateApplications(c, false, s.State, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationExportSuite) TestApplicationsWithRootDiskSourceConstraint(c *tc.C) {
	s.assertMigrateApplications(c, false, s.State, constraints.MustParse("arch=amd64 mem=8G root-disk-source=vonnegut"))
}

func (s *MigrationExportSuite) assertMigrateApplications(c *tc.C, isSidecar bool, st *state.State, cons constraints.Value) {
	f := factory.NewFactory(st, s.StatePool)

	dbModel, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	series := "quantal"
	if dbModel.Type() == state.ModelTypeCAAS && !isSidecar {
		series = "kubernetes"
	}
	var ch *state.Charm
	if isSidecar {
		ch = f.MakeCharmV2(c, &factory.CharmParams{
			Name:   "snappass-test",
			Series: series,
		})
	} else {
		ch = f.MakeCharm(c, &factory.CharmParams{Series: series})
	}
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
		CharmOrigin: &state.CharmOrigin{
			Channel: &state.Channel{
				Risk: "beta",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "20.04/stable",
			},
		},
		ApplicationConfig: map[string]interface{}{
			"app foo": "app bar",
		},
		ApplicationConfigFields: environschema.Fields{
			"app foo": environschema.Attr{Type: environschema.Tstring}},
		Constraints:  cons,
		DesiredScale: 3,
	})

	err = application.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = application.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, tc.ErrorIsNil)
	err = dbModel.SetAnnotations(application, testAnnotations)
	c.Assert(err, tc.ErrorIsNil)

	if dbModel.Type() == state.ModelTypeCAAS {
		_, err = application.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id1")})
		c.Assert(err, tc.ErrorIsNil)
		application.SetOperatorStatus(status.StatusInfo{Status: status.Running})

		caasModel, err := dbModel.CAASModel()
		c.Assert(err, tc.ErrorIsNil)
		if !isSidecar {
			err = caasModel.SetPodSpec(nil, application.ApplicationTag(), strPtr("pod spec"))
			c.Assert(err, tc.ErrorIsNil)
		}
		addr := network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeCloudLocal))
		err = application.UpdateCloudService("provider-id", []network.SpaceAddress{addr})
		c.Assert(err, tc.ErrorIsNil)

		if isSidecar {
			err = application.SetProvisioningState(state.ApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 3,
			})
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	agentVer, err := version.ParseBinary("2.9.1-ubuntu-amd64")
	c.Assert(err, tc.ErrorIsNil)
	if dbModel.Type() == state.ModelTypeCAAS && !isSidecar {
		err = application.SetAgentVersion(agentVer)
		c.Assert(err, tc.ErrorIsNil)
	} else {
		units, err := application.AllUnits()
		c.Assert(err, tc.ErrorIsNil)
		for _, unit := range units {
			err = unit.SetAgentVersion(agentVer)
			c.Assert(err, tc.ErrorIsNil)
		}
	}

	s.primeStatusHistory(c, application, status.Active, addedHistoryCount)

	model, err := st.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	exported := applications[0]
	c.Assert(exported.Name(), tc.Equals, application.Name())
	c.Assert(exported.Tag(), tc.Equals, application.ApplicationTag())
	c.Assert(exported.Type(), tc.Equals, string(dbModel.Type()))
	c.Assert(exported.Annotations(), tc.DeepEquals, testAnnotations)

	origin := exported.CharmOrigin()
	c.Assert(origin.Channel(), tc.Equals, "beta")
	c.Assert(origin.Platform(), tc.Equals, "amd64/ubuntu/20.04/stable")

	c.Assert(exported.CharmConfig(), tc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(exported.ApplicationConfig(), tc.DeepEquals, map[string]interface{}{
		"app foo": "app bar",
	})
	c.Assert(exported.LeadershipSettings(), tc.DeepEquals, map[string]interface{}{
		"leader": "true",
	})
	c.Assert(exported.MetricsCredentials(), tc.DeepEquals, []byte("sekrit"))

	constraints := exported.Constraints()
	c.Assert(constraints, tc.NotNil)
	c.Assert(constraints.Architecture(), tc.Equals, *cons.Arch)
	c.Assert(constraints.Memory(), tc.Equals, *cons.Mem)
	if cons.HasVirtType() {
		c.Assert(constraints.VirtType(), tc.Equals, *cons.VirtType)
	}
	if cons.HasRootDiskSource() {
		c.Assert(constraints.RootDiskSource(), tc.Equals, *cons.RootDiskSource)
	}
	if cons.HasRootDisk() {
		c.Assert(constraints.RootDisk(), tc.Equals, *cons.RootDisk)
	}

	history := exported.StatusHistory()
	c.Assert(history, tc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, history[:addedHistoryCount], status.Active)

	if dbModel.Type() == state.ModelTypeCAAS {
		if !isSidecar {
			c.Assert(exported.PodSpec(), tc.Equals, "pod spec")
			tools, err := application.AgentTools()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(exported.Tools().Version(), tc.Equals, tools.Version)
		} else {
			c.Assert(exported.PodSpec(), tc.Equals, "")
			units, err := application.AllUnits()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(len(units), tc.Equals, len(exported.Units()))

			for _, exportedUnit := range exported.Units() {
				tools := exportedUnit.Tools()
				c.Assert(tools.Version(), tc.Equals, agentVer)
			}
		}
		c.Assert(exported.CloudService().ProviderId(), tc.Equals, "provider-id")
		c.Assert(exported.DesiredScale(), tc.Equals, 3)
		c.Assert(exported.Placement(), tc.Equals, "")
		c.Assert(exported.HasResources(), tc.IsTrue)
		addresses := exported.CloudService().Addresses()
		addr := addresses[0]
		c.Assert(addr.Value(), tc.Equals, "192.168.1.1")
		c.Assert(addr.Scope(), tc.Equals, "local-cloud")
		c.Assert(addr.Type(), tc.Equals, "ipv4")
		c.Assert(addr.Origin(), tc.Equals, "provider")
	} else {
		c.Assert(exported.PodSpec(), tc.Equals, "")
		c.Assert(exported.CloudService(), tc.IsNil)
		_, err := application.AgentTools()
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}

	if dbModel.Type() == state.ModelTypeCAAS && isSidecar {
		ps := exported.ProvisioningState()
		c.Assert(ps, tc.NotNil)
		c.Assert(ps.Scaling(), tc.IsTrue)
		c.Assert(ps.ScaleTarget(), tc.Equals, 3)
	} else {
		c.Assert(exported.ProvisioningState(), tc.IsNil)
	}

	// Check that we're exporting the metadata.
	exportedCharmMetadata := exported.CharmMetadata()
	c.Assert(exportedCharmMetadata, tc.NotNil)
	s.assertMigrateCharmMetadata(c, exportedCharmMetadata, ch.Meta())

	// Check that we're exporting the manifest.
	exportedCharmManifest := exported.CharmManifest()
	c.Assert(exportedCharmManifest, tc.NotNil)
	s.assertMigrateCharmManifest(c, exportedCharmManifest, ch.Manifest())

	// Check that we're exporting the actions.
	exportedCharmActions := exported.CharmActions()
	c.Assert(exportedCharmActions, tc.NotNil)
	s.assertMigrateCharmActions(c, exportedCharmActions, ch.Actions())

	// Check that we're exporting the configs.
	exportedCharmConfigs := exported.CharmConfigs()
	c.Assert(exportedCharmConfigs, tc.NotNil)
	s.assertMigrateCharmConfigs(c, exportedCharmConfigs, ch.Config())
}

func (s *MigrationExportSuite) assertMigrateCharmMetadata(c *tc.C, exported description.CharmMetadata, meta *charm.Meta) {
	c.Assert(exported, tc.NotNil)
	c.Check(exported.Name(), tc.Equals, meta.Name)
	c.Check(exported.Summary(), tc.Equals, meta.Summary)
	c.Check(exported.Description(), tc.Equals, meta.Description)
	c.Check(exported.Categories(), tc.DeepEquals, meta.Categories)
	c.Check(exported.Tags(), tc.DeepEquals, meta.Tags)
	c.Check(exported.Subordinate(), tc.Equals, meta.Subordinate)
	c.Check(exported.Terms(), tc.DeepEquals, meta.Terms)
	c.Check(exported.MinJujuVersion(), tc.DeepEquals, meta.MinJujuVersion.String())
	c.Check(exported.RunAs(), tc.DeepEquals, string(meta.CharmUser))

	// Check we're exporting ExtraBindings metadata.
	extraBindings := make(map[string]string)
	for name, binding := range meta.ExtraBindings {
		extraBindings[name] = binding.Name
	}
	c.Check(exported.ExtraBindings(), tc.DeepEquals, extraBindings)

	// Check we're exporting Provides metadata.
	expectedProvides := make(map[string]string)
	for name, provider := range meta.Provides {
		expectedProvides[name] = provider.Name
	}
	exportedProvides := make(map[string]string)
	for name, provider := range exported.Provides() {
		exportedProvides[name] = provider.Name()
	}
	c.Check(exportedProvides, tc.DeepEquals, expectedProvides)

	// Check we're exporting Requires metadata.
	expectedRequires := make(map[string]string)
	for name, provider := range meta.Requires {
		expectedRequires[name] = provider.Name
	}
	exportedRequires := make(map[string]string)
	for name, provider := range exported.Requires() {
		exportedRequires[name] = provider.Name()
	}
	c.Check(exportedRequires, tc.DeepEquals, expectedRequires)

	// Check we're exporting Peers metadata.
	expectedPeers := make(map[string]string)
	for name, provider := range meta.Peers {
		expectedPeers[name] = provider.Name
	}
	exportedPeers := make(map[string]string)
	for name, provider := range exported.Peers() {
		exportedPeers[name] = provider.Name()
	}
	c.Check(exportedPeers, tc.DeepEquals, expectedPeers)

	// Check we're exporting Containers metadata.
	expectedContainers := make(map[string]charm.Container)
	for name, container := range meta.Containers {
		expectedContainers[name] = container
	}
	exportedContainers := make(map[string]charm.Container)
	for name, container := range exported.Containers() {
		c := charm.Container{
			Resource: container.Resource(),
			Uid:      container.Uid(),
			Gid:      container.Gid(),
		}
		for _, mount := range container.Mounts() {
			c.Mounts = append(c.Mounts, charm.Mount{
				Storage:  mount.Storage(),
				Location: mount.Location(),
			})
		}
		exportedContainers[name] = c
	}
	c.Check(exportedContainers, tc.DeepEquals, expectedContainers)

	// Check we're exporting Assumes metadata.
	assumes, err := json.Marshal(meta.Assumes)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exported.Assumes(), tc.Equals, string(assumes))

	// Check that we're exporting Storage metadata
	expectedStorage := make(map[string]charm.Storage)
	for name, storage := range meta.Storage {
		expectedStorage[name] = storage
	}
	exportedStorage := make(map[string]charm.Storage)
	for name, storage := range exported.Storage() {
		exportedStorage[name] = charm.Storage{
			Name:        storage.Name(),
			Description: storage.Description(),
			Type:        charm.StorageType(storage.Type()),
			Shared:      storage.Shared(),
			ReadOnly:    storage.Readonly(),
			CountMin:    storage.CountMin(),
			CountMax:    storage.CountMax(),
			MinimumSize: uint64(storage.MinimumSize()),
			Location:    storage.Location(),
			Properties:  storage.Properties(),
		}
	}
	c.Check(exportedStorage, tc.DeepEquals, expectedStorage)

	// Check that we're exporting Devices metadata
	expectedDevices := make(map[string]charm.Device)
	for name, device := range meta.Devices {
		expectedDevices[name] = device
	}
	exportedDevices := make(map[string]charm.Device)
	for name, device := range exported.Devices() {
		exportedDevices[name] = charm.Device{
			Name:        device.Name(),
			Description: device.Description(),
			Type:        charm.DeviceType(device.Type()),
			CountMin:    int64(device.CountMin()),
			CountMax:    int64(device.CountMax()),
		}
	}
	c.Check(exportedDevices, tc.DeepEquals, expectedDevices)

	// Check that we're exporting Payloads metadata
	expectedPayloads := make(map[string]charm.PayloadClass)
	for name, payload := range meta.PayloadClasses {
		expectedPayloads[name] = payload
	}
	exportedPayloads := make(map[string]charm.PayloadClass)
	for name, payload := range exported.Payloads() {
		exportedPayloads[name] = charm.PayloadClass{
			Name: payload.Name(),
			Type: payload.Type(),
		}
	}
	c.Check(exportedPayloads, tc.DeepEquals, expectedPayloads)

	// Check that we're exporting Resources metadata
	expectedResources := make(map[string]charmresource.Meta)
	for name, resource := range meta.Resources {
		expectedResources[name] = resource
	}
	exportedResources := make(map[string]charmresource.Meta)
	for name, resource := range exported.Resources() {
		t, err := charmresource.ParseType(resource.Type())
		c.Assert(err, tc.ErrorIsNil)
		exportedResources[name] = charmresource.Meta{
			Name:        resource.Name(),
			Description: resource.Description(),
			Type:        t,
			Path:        resource.Path(),
		}
	}
	c.Check(exportedResources, tc.DeepEquals, expectedResources)
}

func (s *MigrationExportSuite) assertMigrateCharmManifest(
	c *tc.C,
	exported description.CharmManifest,
	manifest *charm.Manifest,
) {
	expectedManifestBases := make([]string, 0)
	for _, base := range manifest.Bases {
		expectedManifestBases = append(expectedManifestBases, fmt.Sprintf("%s %s %v",
			base.Name,
			base.Channel.String(),
			base.Architectures,
		))
	}
	exportedManifestBases := make([]string, 0)
	for _, base := range exported.Bases() {
		exportedManifestBases = append(exportedManifestBases, fmt.Sprintf("%s %s %v",
			base.Name(),
			base.Channel(),
			base.Architectures(),
		))
	}
	c.Check(exportedManifestBases, tc.DeepEquals, expectedManifestBases)
}

func (s *MigrationExportSuite) assertMigrateCharmActions(
	c *tc.C,
	exported description.CharmActions,
	actions *charm.Actions,
) {
	type actionSpec struct {
		Description    string
		Parallel       bool
		Params         map[string]interface{}
		ExecutionGroup string
	}

	expectedActions := make(map[string]actionSpec)
	for name, action := range actions.ActionSpecs {
		expectedActions[name] = actionSpec{
			Description:    action.Description,
			Parallel:       action.Parallel,
			Params:         action.Params,
			ExecutionGroup: action.ExecutionGroup,
		}
	}

	exportedActions := make(map[string]actionSpec)
	for name, action := range exported.Actions() {
		exportedActions[name] = actionSpec{
			Description:    action.Description(),
			Parallel:       action.Parallel(),
			Params:         action.Parameters(),
			ExecutionGroup: action.ExecutionGroup(),
		}
	}
	c.Check(exportedActions, tc.DeepEquals, expectedActions)
}

func (s *MigrationExportSuite) assertMigrateCharmConfigs(
	c *tc.C,
	exported description.CharmConfigs,
	config *charm.Config,
) {
	type configSpec struct {
		Type        string
		Description string
		Default     interface{}
	}

	expectedConfigs := make(map[string]configSpec)
	for name, config := range config.Options {
		expectedConfigs[name] = configSpec{
			Type:        config.Type,
			Description: config.Description,
			Default:     config.Default,
		}
	}

	exportedConfigs := make(map[string]configSpec)
	for name, config := range exported.Configs() {
		exportedConfigs[name] = configSpec{
			Type:        config.Type(),
			Description: config.Description(),
			Default:     config.Default(),
		}
	}
	c.Check(exportedConfigs, tc.DeepEquals, expectedConfigs)
}

func (s *MigrationExportSuite) TestCharmDataMigrated(c *tc.C) {
	st := s.State
	f := factory.NewFactory(st, s.StatePool)

	_, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	var ch *state.Charm
	ch = f.MakeCharm(c, &factory.CharmParams{
		Name:   "all-charm-data",
		Series: "jammy",
	})
	fmt.Printf("%#v", ch.Meta())

	f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
		CharmOrigin: &state.CharmOrigin{
			Channel: &state.Channel{
				Risk: "beta",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "20.04/stable",
			},
		},
		Devices: map[string]state.DeviceConstraints{
			"miner": {Count: 1},
		},
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024, Pool: "modelscoped-unreleasable"},
		},
	})

	model, err := st.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	exported := applications[0]

	// Check that we're exporting the metadata.
	exportedCharmMetadata := exported.CharmMetadata()
	c.Assert(exportedCharmMetadata, tc.NotNil)
	s.assertMigrateCharmMetadata(c, exportedCharmMetadata, ch.Meta())

	// Check that we're exporting the manifest.
	exportedCharmManifest := exported.CharmManifest()
	c.Assert(exportedCharmManifest, tc.NotNil)
	s.assertMigrateCharmManifest(c, exportedCharmManifest, ch.Manifest())

	// Check that we're exporting the actions.
	exportedCharmActions := exported.CharmActions()
	c.Assert(exportedCharmActions, tc.NotNil)
	s.assertMigrateCharmActions(c, exportedCharmActions, ch.Actions())

	// Check that we're exporting the configs.
	exportedCharmConfigs := exported.CharmConfigs()
	c.Assert(exportedCharmConfigs, tc.NotNil)
	s.assertMigrateCharmConfigs(c, exportedCharmConfigs, ch.Config())
}

func (s *MigrationExportSuite) TestMalformedApplications(c *tc.C) {
	f := factory.NewFactory(s.State, s.StatePool)

	dbModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	series := "quantal"
	ch := f.MakeCharm(c, &factory.CharmParams{Series: series})

	application := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
		CharmOrigin: &state.CharmOrigin{
			Channel: &state.Channel{},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "20.04/stable",
			},
		},
		ApplicationConfig: map[string]interface{}{
			"app foo": "app bar",
		},
		ApplicationConfigFields: environschema.Fields{
			"app foo": environschema.Attr{Type: environschema.Tstring}},
		DesiredScale: 3,
	})

	err = application.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = application.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, tc.ErrorIsNil)
	err = dbModel.SetAnnotations(application, testAnnotations)
	c.Assert(err, tc.ErrorIsNil)

	agentVer, err := version.ParseBinary("2.9.1-ubuntu-amd64")
	c.Assert(err, tc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	for _, unit := range units {
		err = unit.SetAgentVersion(agentVer)
		c.Assert(err, tc.ErrorIsNil)
	}

	s.primeStatusHistory(c, application, status.Active, addedHistoryCount)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	exported := applications[0]
	c.Assert(exported.Name(), tc.Equals, application.Name())
	c.Assert(exported.Tag(), tc.Equals, application.ApplicationTag())
	c.Assert(exported.Type(), tc.Equals, string(dbModel.Type()))
	c.Assert(exported.Annotations(), tc.DeepEquals, testAnnotations)

	origin := exported.CharmOrigin()
	c.Assert(origin.Channel(), tc.Equals, "stable")
	c.Assert(origin.Platform(), tc.Equals, "amd64/ubuntu/20.04/stable")
}

func (s *MigrationExportSuite) TestMultipleApplications(c *tc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "first"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "second"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "third"})

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 3)
}

func (s *MigrationExportSuite) TestApplicationExposeParameters(c *tc.C) {
	serverSpace, err := s.State.AddSpace("server", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	app := s.AddTestingApplicationWithBindings(c, "mysql",
		s.AddTestingCharm(c, "mysql"),
		map[string]string{
			"server": serverSpace.Id(),
		},
	)

	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{serverSpace.Id()},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	expEps := applications[0].ExposedEndpoints()
	c.Assert(expEps, tc.HasLen, 1)
	c.Assert(expEps["server"], tc.Not(tc.IsNil))
	c.Assert(expEps["server"].ExposeToSpaceIDs(), tc.DeepEquals, []string{serverSpace.Id()})
	c.Assert(expEps["server"].ExposeToCIDRs(), tc.DeepEquals, []string{"13.37.0.0/16"})
}

func (s *MigrationExportSuite) TestApplicationExposingOffers(c *tc.C) {
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "admin"})
	fooUser := s.Factory.MakeUser(c, &factory.UserParams{Name: "foo"})
	serverSpace, err := s.State.AddSpace("server", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	adminSpace, err := s.State.AddSpace("server-admin", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	app := s.AddTestingApplicationWithBindings(c, "mysql",
		s.AddTestingCharm(c, "mysql"),
		map[string]string{
			"server":       serverSpace.Id(),
			"server-admin": adminSpace.Id(),
		},
	)

	stOffers := state.NewApplicationOffers(s.State)
	stOffer, err := stOffers.AddOffer(
		crossmodel.AddApplicationOfferArgs{
			OfferName:              "my-offer",
			Owner:                  "admin",
			ApplicationName:        app.Name(),
			ApplicationDescription: fmt.Sprintf("%s description", app.Name()),
			Endpoints: map[string]string{
				"server":       serverSpace.Name(),
				"server-admin": adminSpace.Name(),
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Allow "foo" to consume offer
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(stOffer.OfferUUID),
		fooUser.UserTag(),
		permission.ConsumeAccess,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We only care for the offers
	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipActions:              true,
		SkipAnnotations:          true,
		SkipCloudImageMetadata:   true,
		SkipCredentials:          true,
		SkipIPAddresses:          true,
		SkipSettings:             true,
		SkipSSHHostKeys:          true,
		SkipStatusHistory:        true,
		SkipLinkLayerDevices:     true,
		SkipUnitAgentBinaries:    true,
		SkipMachineAgentBinaries: true,
		SkipRelationData:         true,
		SkipInstanceData:         true,
		SkipOfferConnections:     true,
	})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	appOffers := applications[0].Offers()
	c.Assert(appOffers, tc.HasLen, 1)
	appOffer := appOffers[0]
	c.Assert(appOffer.OfferUUID(), tc.Equals, stOffer.OfferUUID)
	c.Assert(appOffer.OfferName(), tc.Equals, "my-offer")
	c.Assert(appOffer.ApplicationName(), tc.Equals, app.Name())
	c.Assert(appOffer.ApplicationDescription(), tc.Equals, fmt.Sprintf("%s description", app.Name()))

	endpointsMap := appOffer.Endpoints()
	c.Assert(endpointsMap, tc.DeepEquals, map[string]string{
		"server":       serverSpace.Name(),
		"server-admin": adminSpace.Name(),
	})

	appACL := appOffer.ACL()
	c.Assert(appACL, tc.DeepEquals, map[string]string{
		"admin": "admin",
		"foo":   "consume",
	})
}

func (s *MigrationExportSuite) TestOfferConnections(c *tc.C) {
	stOffer, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		OfferUUID:       "offer-uuid",
		RelationId:      1,
		RelationKey:     "relation-key",
		SourceModelUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Username:        "fred",
	})
	c.Assert(err, tc.ErrorIsNil)

	// We only care for the offer connections
	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipActions:              true,
		SkipAnnotations:          true,
		SkipCloudImageMetadata:   true,
		SkipCredentials:          true,
		SkipIPAddresses:          true,
		SkipSettings:             true,
		SkipSSHHostKeys:          true,
		SkipStatusHistory:        true,
		SkipLinkLayerDevices:     true,
		SkipUnitAgentBinaries:    true,
		SkipMachineAgentBinaries: true,
		SkipRelationData:         true,
		SkipInstanceData:         true,
		SkipApplicationOffers:    true,
	})
	c.Assert(err, tc.ErrorIsNil)

	offers := model.OfferConnections()
	c.Assert(offers, tc.HasLen, 1)
	offer := offers[0]
	c.Assert(offer.OfferUUID(), tc.Equals, stOffer.OfferUUID())
	c.Assert(offer.RelationID(), tc.Equals, stOffer.RelationId())
	c.Assert(offer.RelationKey(), tc.Equals, stOffer.RelationKey())
	c.Assert(offer.SourceModelUUID(), tc.Equals, stOffer.SourceModelUUID())
	c.Assert(offer.UserName(), tc.Equals, stOffer.UserName())
}

func (s *MigrationExportSuite) TestExternalControllers(c *tc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
		OfferUUID:   "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	service := state.NewExternalControllers(s.State)
	stCtrl, err := service.Save(crossmodel.ControllerInfo{
		Addrs:         []string{"10.224.0.1:8080"},
		Alias:         "magic",
		CACert:        "magic-ca-cert",
		ControllerTag: names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
	}, s.Model.UUID(), "af5a9137-934c-4b0c-8317-643b69cf4971")
	c.Assert(err, tc.ErrorIsNil)

	// We only care for the external controllers
	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipActions:              true,
		SkipAnnotations:          true,
		SkipCloudImageMetadata:   true,
		SkipCredentials:          true,
		SkipIPAddresses:          true,
		SkipSettings:             true,
		SkipSSHHostKeys:          true,
		SkipStatusHistory:        true,
		SkipLinkLayerDevices:     true,
		SkipUnitAgentBinaries:    true,
		SkipMachineAgentBinaries: true,
		SkipRelationData:         true,
		SkipInstanceData:         true,
		SkipApplicationOffers:    true,
	})
	c.Assert(err, tc.ErrorIsNil)

	ctrls := model.ExternalControllers()
	c.Assert(ctrls, tc.HasLen, 1)
	ctrl := ctrls[0]
	c.Assert(ctrl.Addrs(), tc.DeepEquals, stCtrl.ControllerInfo().Addrs)
	c.Assert(ctrl.Alias(), tc.Equals, stCtrl.ControllerInfo().Alias)
	c.Assert(ctrl.CACert(), tc.Equals, stCtrl.ControllerInfo().CACert)
	c.Assert(ctrl.Models(), tc.DeepEquals, []string{s.Model.UUID(), "af5a9137-934c-4b0c-8317-643b69cf4971"})
}

func (s *MigrationExportSuite) TestUnits(c *tc.C) {
	s.assertMigrateUnits(c, s.State)
}

func (s *MigrationExportSuite) TestCAASUnits(c *tc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { caasSt.Close() })

	s.assertMigrateUnits(c, caasSt)
}

func (s *MigrationExportSuite) assertMigrateUnits(c *tc.C, st *state.State) {
	f := factory.NewFactory(st, s.StatePool)

	unit := f.MakeUnit(c, &factory.UnitParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	for _, version := range []string{"garnet", "amethyst", "pearl", "steven"} {
		err := unit.SetWorkloadVersion(version)
		c.Assert(err, tc.ErrorIsNil)
	}
	us := state.NewUnitState()
	us.SetCharmState(map[string]string{"payload": "b4dc0ffee"})
	us.SetRelationState(map[int]string{42: "magic"})
	us.SetUniterState("uniter state")
	us.SetStorageState("storage state")
	err := unit.SetState(us, state.UnitStateSizeLimits{})
	c.Assert(err, tc.ErrorIsNil)

	dbModel, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	if dbModel.Type() == state.ModelTypeCAAS {
		// need to set a cloud container status so that SetStatus for
		// the unit doesn't throw away the history writes.
		var updateUnits state.UpdateUnitsOperation
		updateUnits.Updates = []*state.UpdateUnitOperation{
			unit.UpdateOperation(state.UnitUpdateProperties{
				ProviderId: strPtr("provider-id"),
				Address:    strPtr("192.168.1.1"),
				Ports:      &[]string{"80"},
				CloudContainerStatus: &status.StatusInfo{
					Status:  status.Running,
					Message: "cloud container running",
				},
			})}
		app, err := unit.Application()
		c.Assert(err, tc.ErrorIsNil)
		err = app.UpdateUnits(&updateUnits)
		c.Assert(err, tc.ErrorIsNil)
	}

	err = dbModel.SetAnnotations(unit, testAnnotations)
	c.Assert(err, tc.ErrorIsNil)
	s.primeStatusHistory(c, unit, status.Active, addedHistoryCount)
	s.primeStatusHistory(c, unit.Agent(), status.Idle, addedHistoryCount)

	model, err := st.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	application := applications[0]
	units := application.Units()
	c.Assert(units, tc.HasLen, 1)

	exported := units[0]

	c.Assert(exported.Name(), tc.Equals, unit.Name())
	c.Assert(exported.Tag(), tc.Equals, unit.UnitTag())
	c.Assert(exported.Validate(), tc.ErrorIsNil)
	c.Assert(exported.MeterStatusCode(), tc.Equals, "")
	c.Assert(exported.MeterStatusInfo(), tc.Equals, "")
	c.Assert(exported.WorkloadVersion(), tc.Equals, "steven")
	c.Assert(exported.Annotations(), tc.DeepEquals, testAnnotations)
	c.Assert(exported.CharmState(), tc.DeepEquals, map[string]string{"payload": "b4dc0ffee"})
	c.Assert(exported.RelationState(), tc.DeepEquals, map[int]string{42: "magic"})
	c.Assert(exported.UniterState(), tc.Equals, "uniter state")
	c.Assert(exported.StorageState(), tc.Equals, "storage state")
	c.Assert(exported.MeterStatusState(), tc.Equals, "")
	obtainedConstraints := exported.Constraints()
	c.Assert(obtainedConstraints, tc.NotNil)
	c.Assert(obtainedConstraints.Architecture(), tc.Equals, "amd64")
	c.Assert(obtainedConstraints.Memory(), tc.Equals, 8*gig)

	workloadHistory := exported.WorkloadStatusHistory()
	if dbModel.Type() == state.ModelTypeCAAS {
		// Account for the extra cloud container status history addition.
		c.Assert(workloadHistory, tc.HasLen, expectedHistoryCount+1)
		c.Assert(workloadHistory[expectedHistoryCount].Message(), tc.Equals, "installing agent")
		c.Assert(workloadHistory[expectedHistoryCount].Value(), tc.Equals, "waiting")
		c.Assert(workloadHistory[expectedHistoryCount-1].Message(), tc.Equals, "cloud container running")
		c.Assert(workloadHistory[expectedHistoryCount-1].Value(), tc.Equals, "running")
	} else {
		c.Assert(workloadHistory, tc.HasLen, expectedHistoryCount)
	}
	s.checkStatusHistory(c, workloadHistory[:addedHistoryCount], status.Active)

	agentHistory := exported.AgentStatusHistory()
	c.Assert(agentHistory, tc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, agentHistory[:addedHistoryCount], status.Idle)

	versionHistory := exported.WorkloadVersionHistory()
	// There are extra entries at the start that we don't care about.
	c.Assert(len(versionHistory) >= 4, tc.IsTrue)
	versions := make([]string, 4)
	for i, s := range versionHistory[:4] {
		versions[i] = s.Message()
	}
	// The exporter reads history in reverse time order.
	c.Assert(versions, tc.DeepEquals, []string{"steven", "pearl", "amethyst", "garnet"})

	if dbModel.Type() == state.ModelTypeCAAS {
		containerInfo := exported.CloudContainer()
		c.Assert(containerInfo.ProviderId(), tc.Equals, "provider-id")
		c.Assert(containerInfo.Ports(), tc.DeepEquals, []string{"80"})
		addr := containerInfo.Address()
		c.Assert(addr, tc.NotNil)
		c.Assert(addr.Value(), tc.Equals, "192.168.1.1")
		c.Assert(addr.Scope(), tc.Equals, "local-machine")
		c.Assert(addr.Type(), tc.Equals, "ipv4")
		c.Assert(addr.Origin(), tc.Equals, "provider")
		_, err := unit.AgentTools()
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}

	if dbModel.Type() == state.ModelTypeIAAS {
		tools, err := unit.AgentTools()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(exported.Tools().Version(), tc.Equals, tools.Version)
	}
}

func (s *MigrationExportSuite) TestApplicationLeadership(c *tc.C) {
	s.makeApplicationWithUnits(c, "mysql", 2)
	s.makeApplicationWithUnits(c, "wordpress", 4)

	model, err := s.State.Export(map[string]string{
		"mysql":     "mysql/1",
		"wordpress": "wordpress/2",
	})
	c.Assert(err, tc.ErrorIsNil)

	leaders := make(map[string]string)
	for _, application := range model.Applications() {
		leaders[application.Name()] = application.Leader()
	}
	c.Assert(leaders, tc.DeepEquals, map[string]string{
		"mysql":     "mysql/1",
		"wordpress": "wordpress/2",
	})
}

func (s *MigrationExportSuite) TestUnitOpenPortRanges(c *tc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Machine: machine,
	})
	c.Assert(unit.AssignToMachine(machine), tc.ErrorIsNil)

	state.MustOpenUnitPortRange(c, s.State, machine, unit.Name(), allEndpoints, network.MustParsePortRange("1234-2345/tcp"))

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	machines := model.Machines()
	c.Assert(machines, tc.HasLen, 1)

	unitPortRanges := machines[0].OpenedPortRanges().ByUnit()[unit.Name()]
	c.Assert(unitPortRanges, tc.Not(tc.IsNil), tc.Commentf("opened port ranges for unit not included in exported model"))

	unitPortRangesByEndpoint := unitPortRanges.ByEndpoint()
	c.Assert(unitPortRangesByEndpoint, tc.HasLen, 1)
	c.Assert(unitPortRangesByEndpoint[allEndpoints], tc.HasLen, 1)

	portRange := unitPortRangesByEndpoint[allEndpoints][0]
	c.Assert(portRange.FromPort(), tc.Equals, 1234)
	c.Assert(portRange.ToPort(), tc.Equals, 2345)
	c.Assert(portRange.Protocol(), tc.Equals, "tcp")
}

func (s *MigrationExportSuite) TestEndpointBindings(c *tc.C) {
	oneSpace := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": oneSpace.Id()})

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	apps := model.Applications()
	c.Assert(apps, tc.HasLen, 1)
	wordpress := apps[0]

	bindings := wordpress.EndpointBindings()
	// There are empty values for every charm endpoint, but we only care about the
	// db endpoint.
	c.Assert(bindings["db"], tc.Equals, oneSpace.Id())
}

func (s *MigrationExportSuite) TestRemoteEntities(c *tc.C) {
	remotes := s.State.RemoteEntities()
	remoteCtrl := names.NewControllerTag("uuid-223412")

	err := remotes.ImportRemoteEntity(remoteCtrl, "aaa-bbb-ccc")
	c.Assert(err, tc.ErrorIsNil)

	mac, err := macaroon.New(nil, []byte(remoteCtrl.Id()), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	err = remotes.SaveMacaroon(remoteCtrl, mac)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	remoteEntities := model.RemoteEntities()
	c.Assert(remoteEntities, tc.HasLen, 1)

	entity := remoteEntities[0]
	c.Assert(entity.ID(), tc.Equals, names.NewControllerTag("uuid-223412").String())
	c.Assert(entity.Token(), tc.Equals, "aaa-bbb-ccc")
	c.Assert(entity.Macaroon(), tc.Equals, "")
}

func (s *MigrationExportSuite) TestRelationNetworks(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)

	_, err = state.NewRelationIngressNetworks(s.State).Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	relationNetwork := model.RelationNetworks()
	c.Assert(relationNetwork, tc.HasLen, 1)

	rin := relationNetwork[0]
	c.Assert(rin.RelationKey(), tc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), tc.DeepEquals, []string{"192.168.1.0/16"})
}

func (s *MigrationExportSuite) TestRelations(c *tc.C) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	msEp, wpEp := eps[0], eps[1]
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	mysql_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	ru, err := rel.Unit(wordpress_0)
	c.Assert(err, tc.ErrorIsNil)
	wordpressSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(wordpressSettings)
	c.Assert(err, tc.ErrorIsNil)

	ru, err = rel.Unit(mysql_0)
	c.Assert(err, tc.ErrorIsNil)
	mysqlSettings := map[string]interface{}{
		"name": "mysql/0",
	}
	err = ru.EnterScope(mysqlSettings)
	c.Assert(err, tc.ErrorIsNil)

	wordpressAppSettings := map[string]interface{}{
		"war": "worlds",
	}
	err = rel.UpdateApplicationSettings("wordpress", &fakeToken{}, wordpressAppSettings)
	c.Assert(err, tc.ErrorIsNil)

	mysqlAppSettings := map[string]interface{}{
		"million": "one",
	}
	err = rel.UpdateApplicationSettings("mysql", &fakeToken{}, mysqlAppSettings)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	rels := model.Relations()
	c.Assert(rels, tc.HasLen, 1)

	exRel := rels[0]
	c.Assert(exRel.Id(), tc.Equals, rel.Id())
	c.Assert(exRel.Key(), tc.Equals, rel.String())

	exEps := exRel.Endpoints()
	c.Assert(exEps, tc.HasLen, 2)

	checkEndpoint := func(
		exEndpoint description.Endpoint,
		unitName string,
		ep state.Endpoint,
		settings, appSettings map[string]interface{},
	) {
		c.Logf("%#v", exEndpoint)
		c.Check(exEndpoint.ApplicationName(), tc.Equals, ep.ApplicationName)
		c.Check(exEndpoint.Name(), tc.Equals, ep.Name)
		c.Check(exEndpoint.UnitCount(), tc.Equals, 1)
		c.Check(exEndpoint.Settings(unitName), tc.DeepEquals, settings)
		c.Check(exEndpoint.ApplicationSettings(), tc.DeepEquals, appSettings)
		c.Check(exEndpoint.Role(), tc.Equals, string(ep.Role))
		c.Check(exEndpoint.Interface(), tc.Equals, ep.Interface)
		c.Check(exEndpoint.Optional(), tc.Equals, ep.Optional)
		c.Check(exEndpoint.Limit(), tc.Equals, ep.Limit)
		c.Check(exEndpoint.Scope(), tc.Equals, string(ep.Scope))
	}
	checkEndpoint(exEps[0], mysql_0.Name(), msEp, mysqlSettings, mysqlAppSettings)
	checkEndpoint(exEps[1], wordpress_0.Name(), wpEp, wordpressSettings, wordpressAppSettings)

	// Make sure there is a status.
	status := exRel.Status()
	c.Check(status.Value(), tc.Equals, "joining")
}

func (s *MigrationExportSuite) TestSubordinateRelations(c *tc.C) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	mysql_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	logging := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))

	addSubordinate := func(app *state.Application, unit *state.Unit) {
		eps, err := s.State.InferEndpoints(app.Name(), logging.Name())
		c.Assert(err, tc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, tc.ErrorIsNil)
		pru, err := rel.Unit(unit)
		c.Assert(err, tc.ErrorIsNil)
		err = pru.EnterScope(nil)
		c.Assert(err, tc.ErrorIsNil)
		// Need to reload the doc to get the subordinates.
		err = unit.Refresh()
		c.Assert(err, tc.ErrorIsNil)
		subordinates := unit.SubordinateNames()
		c.Assert(subordinates, tc.HasLen, 1)
		loggingUnit, err := s.State.Unit(subordinates[0])
		c.Assert(err, tc.ErrorIsNil)
		sub, err := rel.Unit(loggingUnit)
		c.Assert(err, tc.ErrorIsNil)
		err = sub.EnterScope(nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	addSubordinate(mysql, mysql_0)
	addSubordinate(wordpress, wordpress_0)

	setTools := func(unit *state.Unit) {
		app, err := unit.Application()
		c.Assert(err, tc.ErrorIsNil)
		agentTools := version.Binary{
			Number:  jujuversion.Current,
			Arch:    arch.HostArch(),
			Release: app.CharmOrigin().Platform.OS,
		}
		err = unit.SetAgentVersion(agentTools)
		c.Assert(err, tc.ErrorIsNil)
	}

	units, err := logging.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 2)

	for _, unit := range units {
		setTools(unit)
	}

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	rels := model.Relations()
	c.Assert(rels, tc.HasLen, 2)
}

func (s *MigrationExportSuite) TestSpaces(c *tc.C) {
	s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	spaces := model.Spaces()
	c.Assert(spaces, tc.HasLen, 1)

	space := spaces[0]

	c.Assert(space.Id(), tc.Not(tc.Equals), "")
	c.Assert(space.Name(), tc.Equals, "one")
	c.Assert(space.ProviderID(), tc.Equals, "provider")
	c.Assert(space.Public(), tc.IsTrue)
}

func (s *MigrationExportSuite) TestMultipleSpaces(c *tc.C) {
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "one"})
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "two"})
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "three"})

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Spaces(), tc.HasLen, 3)
}

func (s *MigrationExportSuite) TestLinkLayerDevices(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	deviceArgs := state.LinkLayerDeviceArgs{
		Name:            "foo",
		Type:            network.EthernetDevice,
		VirtualPortType: network.OvsPort,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	devices := model.LinkLayerDevices()
	c.Assert(devices, tc.HasLen, 1)
	device := devices[0]
	c.Assert(device.Name(), tc.Equals, "foo")
	c.Assert(device.Type(), tc.Equals, string(network.EthernetDevice))
	c.Assert(device.VirtualPortType(), tc.Equals, "openvswitch", tc.Commentf("VirtualPortType was not exported correctly"))
}

func (s *MigrationExportSuite) TestLinkLayerDevicesSkipped(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: network.EthernetDevice,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipLinkLayerDevices: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	devices := model.LinkLayerDevices()
	c.Assert(devices, tc.HasLen, 0)
}

func (s *MigrationExportSuite) TestInstanceDataSkipped(c *tc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipInstanceData: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listMachines := model.Machines()

	instData := listMachines[0].Instance()
	c.Assert(instData, tc.Equals, nil)
}

func (s *MigrationExportSuite) TestMissingInstanceDataIgnored(c *tc.C) {
	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("18.04"),
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		IgnoreIncompleteModel: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listMachines := model.Machines()

	instData := listMachines[0].Instance()
	c.Assert(instData, tc.Equals, nil)
}

func (s *MigrationBaseSuite) TestMachineAgentBinariesSkipped(c *tc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipMachineAgentBinaries: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listMachines := model.Machines()
	tools := listMachines[0].Tools()
	c.Assert(tools, tc.Equals, nil)
}

func (s *MigrationBaseSuite) TestMissingMachineAgentBinariesIgnored(c *tc.C) {
	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("18.04"),
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		IgnoreIncompleteModel: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listMachines := model.Machines()
	tools := listMachines[0].Tools()
	c.Assert(tools, tc.Equals, nil)
}

func (s *MigrationBaseSuite) TestUnitAgentBinariesSkipped(c *tc.C) {
	dummyCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "dummy", Charm: dummyCharm})

	_, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipUnitAgentBinaries: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listApplications := model.Applications()
	unit := listApplications[0].Units()
	c.Assert(unit[0].Tools(), tc.Equals, nil)
}

func (s *MigrationBaseSuite) TestMissingUnitAgentBinariesIgnored(c *tc.C) {
	dummyCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "dummy", Charm: dummyCharm})

	_, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		IgnoreIncompleteModel: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	listApplications := model.Applications()
	unit := listApplications[0].Units()
	c.Assert(unit[0].Tools(), tc.Equals, nil)
}

func (s *MigrationBaseSuite) TestRelationScopeSkipped(c *tc.C) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipRelationData: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Relations(), tc.HasLen, 1)
}

func (s *MigrationBaseSuite) TestMissingRelationScopeIgnored(c *tc.C) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	model, err := s.State.ExportPartial(state.ExportConfig{
		IgnoreIncompleteModel: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Relations(), tc.HasLen, 1)
}

func (s *MigrationExportSuite) TestSubnets(c *tc.C) {
	sp, err := s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	sn := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        network.Id("foo"),
		ProviderNetworkId: network.Id("rust"),
		VLANTag:           64,
		AvailabilityZones: []string{"bar"},
		SpaceID:           sp.Id(),
		IsPublic:          true,
	}
	sn.SetFan("100.2.0.0/16", "253.0.0.0/8")

	expectedSubnet, err := s.State.AddSubnet(sn)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	subnets := model.Subnets()
	c.Assert(subnets, tc.HasLen, 1)
	subnet := subnets[0]
	c.Assert(subnet.CIDR(), tc.Equals, sn.CIDR)
	c.Assert(subnet.ID(), tc.Equals, expectedSubnet.ID())
	c.Assert(subnet.ProviderId(), tc.Equals, string(sn.ProviderId))
	c.Assert(subnet.ProviderNetworkId(), tc.Equals, string(sn.ProviderNetworkId))
	c.Assert(subnet.VLANTag(), tc.Equals, sn.VLANTag)
	c.Assert(subnet.AvailabilityZones(), tc.DeepEquals, sn.AvailabilityZones)
	c.Assert(subnet.SpaceID(), tc.Equals, sp.Id())
	c.Assert(subnet.FanLocalUnderlay(), tc.Equals, "100.2.0.0/16")
	c.Assert(subnet.FanOverlay(), tc.Equals, "253.0.0.0/8")
	c.Assert(subnet.IsPublic(), tc.Equals, sn.IsPublic)
}

func (s *MigrationExportSuite) TestIPAddresses(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	space, err := s.State.AddSpace("testme", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24", SpaceID: space.Id()})
	c.Assert(err, tc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: network.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, tc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:        "foo",
		ConfigMethod:      network.ConfigStatic,
		CIDRAddress:       "0.1.2.3/24",
		ProviderID:        "bar",
		DNSServers:        []string{"bam", "mam"},
		DNSSearchDomains:  []string{"weeee"},
		GatewayAddress:    "0.1.2.1",
		ProviderNetworkID: "p-net-id",
		ProviderSubnetID:  "p-sub-id",
		Origin:            network.OriginProvider,
	}
	err = machine.SetDevicesAddresses(args)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	addresses := model.IPAddresses()
	c.Assert(addresses, tc.HasLen, 1)
	addr := addresses[0]
	c.Assert(addr.Value(), tc.Equals, "0.1.2.3")
	c.Assert(addr.MachineID(), tc.Equals, machine.Id())
	c.Assert(addr.DeviceName(), tc.Equals, "foo")
	c.Assert(addr.ConfigMethod(), tc.Equals, string(network.ConfigStatic))
	c.Assert(addr.SubnetCIDR(), tc.Equals, "0.1.2.0/24")
	c.Assert(addr.ProviderID(), tc.Equals, "bar")
	c.Assert(addr.DNSServers(), tc.DeepEquals, []string{"bam", "mam"})
	c.Assert(addr.DNSSearchDomains(), tc.DeepEquals, []string{"weeee"})
	c.Assert(addr.GatewayAddress(), tc.Equals, "0.1.2.1")
	c.Assert(addr.ProviderNetworkID(), tc.Equals, "p-net-id")
	c.Assert(addr.ProviderSubnetID(), tc.Equals, "p-sub-id")
	c.Assert(addr.Origin(), tc.Equals, string(network.OriginProvider))

}

func (s *MigrationExportSuite) TestIPAddressesSkipped(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, tc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: network.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, tc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:       "foo",
		ConfigMethod:     network.ConfigStatic,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "bar",
		DNSServers:       []string{"bam", "mam"},
		DNSSearchDomains: []string{"weeee"},
		GatewayAddress:   "0.1.2.1",
	}
	err = machine.SetDevicesAddresses(args)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipIPAddresses: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	addresses := model.IPAddresses()
	c.Assert(addresses, tc.HasLen, 0)
}

func (s *MigrationExportSuite) TestSSHHostKeys(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := s.State.SetSSHHostKeys(machine.MachineTag(), []string{"bam", "mam"})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	keys := model.SSHHostKeys()
	c.Assert(keys, tc.HasLen, 1)
	key := keys[0]
	c.Assert(key.MachineID(), tc.Equals, machine.Id())
	c.Assert(key.Keys(), tc.DeepEquals, []string{"bam", "mam"})
}

func (s *MigrationExportSuite) TestSSHHostKeysSkipped(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := s.State.SetSSHHostKeys(machine.MachineTag(), []string{"bam", "mam"})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipSSHHostKeys: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys := model.SSHHostKeys()
	c.Assert(keys, tc.HasLen, 0)
}

func (s *MigrationExportSuite) TestCloudImageMetadata(c *tc.C) {
	storageSize := uint64(3)
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "test",
	}
	metadata := []cloudimagemetadata.Metadata{{attrs, 2, "1", 2}}

	err := s.State.CloudImageMetadataStorage.SaveMetadata(metadata)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	images := model.CloudImageMetadata()
	c.Assert(images, tc.HasLen, 1)
	image := images[0]
	c.Check(image.Stream(), tc.Equals, "stream")
	c.Check(image.Region(), tc.Equals, "region-test")
	c.Check(image.Version(), tc.Equals, "22.04")
	c.Check(image.Arch(), tc.Equals, "arch")
	c.Check(image.VirtType(), tc.Equals, "virtType-test")
	c.Check(image.RootStorageType(), tc.Equals, "rootStorageType-test")
	value, ok := image.RootStorageSize()
	c.Assert(ok, tc.IsTrue)
	c.Assert(value, tc.Equals, uint64(3))
	c.Check(image.Source(), tc.Equals, "test")
	c.Check(image.Priority(), tc.Equals, 2)
	c.Check(image.ImageId(), tc.Equals, "1")
	c.Check(image.DateCreated(), tc.Equals, int64(2))
}

func (s *MigrationExportSuite) TestCloudImageMetadataSkipped(c *tc.C) {
	storageSize := uint64(3)
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "test",
	}
	metadata := []cloudimagemetadata.Metadata{{attrs, 2, "1", 2}}

	err := s.State.CloudImageMetadataStorage.SaveMetadata(metadata)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipCloudImageMetadata: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	images := model.CloudImageMetadata()
	c.Assert(images, tc.HasLen, 0)
}

func (s *MigrationExportSuite) TestActions(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := m.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	a, err := m.EnqueueAction(operationID, machine.MachineTag(), "foo", nil, true, "group", nil)
	c.Assert(err, tc.ErrorIsNil)
	a, err = a.Begin()
	c.Assert(err, tc.ErrorIsNil)
	err = a.Log("hello")
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
	actions := model.Actions()
	c.Assert(actions, tc.HasLen, 1)
	action := actions[0]
	c.Check(action.Receiver(), tc.Equals, machine.Id())
	c.Check(action.Name(), tc.Equals, "foo")
	c.Check(action.Operation(), tc.Equals, operationID)
	c.Check(action.Parallel(), tc.IsTrue)
	c.Check(action.ExecutionGroup(), tc.Equals, "group")
	c.Check(action.Status(), tc.Equals, "running")
	c.Check(action.Message(), tc.Equals, "")
	logs := action.Logs()
	c.Assert(logs, tc.HasLen, 1)
	c.Assert(logs[0].Message(), tc.Equals, "hello")
	c.Assert(logs[0].Timestamp().IsZero(), tc.IsFalse)
}

func (s *MigrationExportSuite) TestActionsSkipped(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = m.EnqueueAction(operationID, machine.MachineTag(), "foo", nil, false, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	model, err := s.State.ExportPartial(state.ExportConfig{
		SkipActions: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	actions := model.Actions()
	c.Assert(actions, tc.HasLen, 0)
	operations := model.Operations()
	c.Assert(operations, tc.HasLen, 0)
}

func (s *MigrationExportSuite) TestOperations(c *tc.C) {
	m, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	operationID, err := m.EnqueueOperation("a test", 2)
	c.Assert(err, tc.ErrorIsNil)
	err = m.FailOperationEnqueuing(operationID, "fail", 1)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
	operations := model.Operations()
	c.Assert(operations, tc.HasLen, 1)
	op := operations[0]
	c.Check(op.Summary(), tc.Equals, "a test")
	c.Check(op.Fail(), tc.Equals, "fail")
	c.Check(op.Status(), tc.Equals, "pending")
	c.Check(op.SpawnedTaskCount(), tc.Equals, 1)
}

type goodToken struct{}

// Check implements leadership.Token
func (*goodToken) Check() error {
	return nil
}

func (s *MigrationExportSuite) TestVolumeAttachmentPlansLocalDisk(c *tc.C) {
	// Storage attachment plans aim to allow the development of external
	// storage backends like iSCSI to be attachable to machines. These
	// types of storage backends, need extra initialization in userspace
	// before they are usable. But this feature also aims to preserve the
	// old codepath, where no extra initialization is needed, and where the
	// providers are forced to guess the final device name that will appear on
	// the machine agents as a result of attaching a disk.
	// This test will ensure that given a local disk (the way it worked before
	// this feature was added), the information set by the provider in
	// VolumeAttachmentInfo is preserved. Different DeviceTypes may overwrite
	// this information, based on what they discover from the newly attached
	// device, after userspace initialization happens. For example, in the
	// case of an iSCSI device, there is no way to guess the final device name,
	// the WWN or any other kind of information about it, until we actually log
	// into the iSCSI target. That information is later sent by the machine
	// worker using SetVolumeAttachmentPlanBlockInfo.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}},
	})
	machineTag := machine.MachineTag()

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	// We know that the first volume is called "0/0" as it is the first volume
	// (volumes use sequences), and it is bound to machine 0.
	volTag := names.NewVolumeTag("0/0")
	err = sb.SetVolumeInfo(volTag, state.VolumeInfo{
		HardwareId: "magic",
		WWN:        "drbr",
		Size:       1500,
		VolumeId:   "volume id",
		Persistent: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	attachmentPlanInfo := state.VolumeAttachmentPlanInfo{
		DeviceType: storage.DeviceTypeLocal,
	}
	attachmentInfo := state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
		PlanInfo:   &attachmentPlanInfo,
	}
	err = sb.SetVolumeAttachmentInfo(machineTag, volTag, attachmentInfo)
	c.Assert(err, tc.ErrorIsNil)

	err = sb.CreateVolumeAttachmentPlan(machineTag, volTag, attachmentPlanInfo)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	volumes := model.Volumes()
	c.Assert(volumes, tc.HasLen, 1)
	volume := volumes[0]

	c.Check(volume.Tag(), tc.Equals, volTag)
	c.Check(volume.Provisioned(), tc.IsTrue)
	c.Check(volume.Size(), tc.Equals, uint64(1500))
	c.Check(volume.Pool(), tc.Equals, "loop")
	c.Check(volume.HardwareID(), tc.Equals, "magic")
	c.Check(volume.WWN(), tc.Equals, "drbr")
	c.Check(volume.VolumeID(), tc.Equals, "volume id")
	c.Check(volume.Persistent(), tc.IsTrue)
	attachments := volume.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsTrue)
	c.Check(attachment.ReadOnly(), tc.IsTrue)
	c.Check(attachment.DeviceName(), tc.Equals, "device name")
	c.Check(attachment.DeviceLink(), tc.Equals, "device link")
	c.Check(attachment.BusAddress(), tc.Equals, "bus address")

	attachmentPlans := volume.AttachmentPlans()
	c.Assert(attachmentPlans, tc.HasLen, 1)

	plan := attachmentPlans[0]
	c.Check(plan.Machine(), tc.Equals, machineTag)
	c.Check(plan.VolumePlanInfo(), tc.NotNil)
	c.Check(storage.DeviceType(plan.VolumePlanInfo().DeviceType()), tc.Equals, storage.DeviceTypeLocal)
	c.Check(plan.VolumePlanInfo().DeviceAttributes(), tc.DeepEquals, map[string]string(nil))

	// This should all be empty
	planBlockDeviceInfo := plan.BlockDevice()
	c.Check(planBlockDeviceInfo.Name(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.Label(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.UUID(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.HardwareID(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.WWN(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.BusAddress(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.FilesystemType(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.MountPoint(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.Links(), tc.IsNil)
	c.Check(planBlockDeviceInfo.InUse(), tc.Equals, false)
	c.Check(planBlockDeviceInfo.Size(), tc.Equals, uint64(0))
}

func (s *MigrationExportSuite) TestVolumeAttachmentPlansISCSIDisk(c *tc.C) {
	// An ISCSI disk will also set the plan block info back in state. This means
	// that once the machine agent logs into the target, and a disk appears on
	// the system, the machine agent fetches all relevant info about that disk
	// and sends it back to state. This info will take precedence when identifying
	// the attached disk, as this info is observed on the machine itself, not
	// guessed by the provider.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}},
	})
	machineTag := machine.MachineTag()

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	// We know that the first volume is called "0/0" as it is the first volume
	// (volumes use sequences), and it is bound to machine 0.
	volTag := names.NewVolumeTag("0/0")
	err = sb.SetVolumeInfo(volTag, state.VolumeInfo{
		HardwareId: "magic",
		WWN:        "drbr",
		Size:       1500,
		VolumeId:   "volume id",
		Persistent: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	deviceAttrs := map[string]string{
		"iqn":         "bogusIQN",
		"address":     "192.168.1.1",
		"port":        "9999",
		"chap-user":   "example",
		"chap-secret": "supersecretpassword",
	}

	attachmentPlanInfo := state.VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceTypeISCSI,
		DeviceAttributes: deviceAttrs,
	}
	attachmentInfo := state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
		PlanInfo:   &attachmentPlanInfo,
	}
	err = sb.SetVolumeAttachmentInfo(machineTag, volTag, attachmentInfo)
	c.Assert(err, tc.ErrorIsNil)

	err = sb.CreateVolumeAttachmentPlan(machineTag, volTag, attachmentPlanInfo)
	c.Assert(err, tc.ErrorIsNil)

	deviceLinks := []string{"/dev/sdb", "/dev/mapper/testDevice"}

	blockInfo := state.BlockDeviceInfo{
		WWN:         "testWWN",
		DeviceLinks: deviceLinks,
		HardwareId:  "test-id",
	}

	err = sb.SetVolumeAttachmentPlanBlockInfo(machineTag, volTag, blockInfo)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	volumes := model.Volumes()
	c.Assert(volumes, tc.HasLen, 1)
	volume := volumes[0]

	c.Check(volume.Tag(), tc.Equals, volTag)
	c.Check(volume.Provisioned(), tc.IsTrue)
	c.Check(volume.Size(), tc.Equals, uint64(1500))
	c.Check(volume.Pool(), tc.Equals, "loop")
	c.Check(volume.HardwareID(), tc.Equals, "magic")
	c.Check(volume.WWN(), tc.Equals, "drbr")
	c.Check(volume.VolumeID(), tc.Equals, "volume id")
	c.Check(volume.Persistent(), tc.IsTrue)
	attachments := volume.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsTrue)
	c.Check(attachment.ReadOnly(), tc.IsTrue)
	c.Check(attachment.DeviceName(), tc.Equals, "device name")
	c.Check(attachment.DeviceLink(), tc.Equals, "device link")
	c.Check(attachment.BusAddress(), tc.Equals, "bus address")

	attachmentPlans := volume.AttachmentPlans()
	c.Assert(attachmentPlans, tc.HasLen, 1)

	plan := attachmentPlans[0]
	c.Check(plan.Machine(), tc.Equals, machineTag)
	c.Check(plan.VolumePlanInfo(), tc.NotNil)
	c.Check(storage.DeviceType(plan.VolumePlanInfo().DeviceType()), tc.Equals, storage.DeviceTypeISCSI)
	c.Check(plan.VolumePlanInfo().DeviceAttributes(), tc.DeepEquals, deviceAttrs)

	// This should all be empty
	planBlockDeviceInfo := plan.BlockDevice()
	c.Check(planBlockDeviceInfo.Name(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.Label(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.UUID(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.HardwareID(), tc.Equals, blockInfo.HardwareId)
	c.Check(planBlockDeviceInfo.WWN(), tc.Equals, blockInfo.WWN)
	c.Check(planBlockDeviceInfo.BusAddress(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.FilesystemType(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.MountPoint(), tc.Equals, "")
	c.Check(planBlockDeviceInfo.Links(), tc.DeepEquals, blockInfo.DeviceLinks)
	c.Check(planBlockDeviceInfo.InUse(), tc.Equals, false)
	c.Check(planBlockDeviceInfo.Size(), tc.Equals, uint64(0))

}

func (s *MigrationExportSuite) TestVolumes(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}, {
			Volume: state.VolumeParams{Size: 4000},
		}},
	})
	machineTag := machine.MachineTag()

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	// We know that the first volume is called "0/0" as it is the first volume
	// (volumes use sequences), and it is bound to machine 0.
	volTag := names.NewVolumeTag("0/0")
	err = sb.SetVolumeInfo(volTag, state.VolumeInfo{
		HardwareId: "magic",
		WWN:        "drbr",
		Size:       1500,
		VolumeId:   "volume id",
		Persistent: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = sb.SetVolumeAttachmentInfo(machineTag, volTag, state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	volumes := model.Volumes()
	c.Assert(volumes, tc.HasLen, 2)
	provisioned, notProvisioned := volumes[0], volumes[1]

	c.Check(provisioned.Tag(), tc.Equals, volTag)
	c.Check(provisioned.Provisioned(), tc.IsTrue)
	c.Check(provisioned.Size(), tc.Equals, uint64(1500))
	c.Check(provisioned.Pool(), tc.Equals, "loop")
	c.Check(provisioned.HardwareID(), tc.Equals, "magic")
	c.Check(provisioned.WWN(), tc.Equals, "drbr")
	c.Check(provisioned.VolumeID(), tc.Equals, "volume id")
	c.Check(provisioned.Persistent(), tc.IsTrue)
	attachments := provisioned.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsTrue)
	c.Check(attachment.ReadOnly(), tc.IsTrue)
	c.Check(attachment.DeviceName(), tc.Equals, "device name")
	c.Check(attachment.DeviceLink(), tc.Equals, "device link")
	c.Check(attachment.BusAddress(), tc.Equals, "bus address")

	attachmentPlans := provisioned.AttachmentPlans()
	c.Assert(attachmentPlans, tc.HasLen, 0)

	c.Check(notProvisioned.Tag(), tc.Equals, names.NewVolumeTag("0/1"))
	c.Check(notProvisioned.Provisioned(), tc.IsFalse)
	c.Check(notProvisioned.Size(), tc.Equals, uint64(4000))
	c.Check(notProvisioned.Pool(), tc.Equals, "loop")
	c.Check(notProvisioned.HardwareID(), tc.Equals, "")
	c.Check(notProvisioned.VolumeID(), tc.Equals, "")
	c.Check(notProvisioned.Persistent(), tc.IsFalse)
	attachments = notProvisioned.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment = attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsFalse)
	c.Check(attachment.ReadOnly(), tc.IsFalse)
	c.Check(attachment.DeviceName(), tc.Equals, "")
	c.Check(attachment.DeviceLink(), tc.Equals, "")
	c.Check(attachment.BusAddress(), tc.Equals, "")

	// Make sure there is a status.
	status := provisioned.Status()
	c.Check(status.Value(), tc.Equals, "pending")
}

func (s *MigrationExportSuite) TestFilesystems(c *tc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Size: 1234},
			Attachment: state.FilesystemAttachmentParams{
				Location: "location",
				ReadOnly: true},
		}, {
			Filesystem: state.FilesystemParams{Size: 4000},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first filesystem is called "0/0" as it is the first
	// filesystem (filesystems use sequences), and it is bound to machine 0.
	fsTag := names.NewFilesystemTag("0/0")
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.SetFilesystemInfo(fsTag, state.FilesystemInfo{
		Size:         1500,
		FilesystemId: "filesystem id",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = sb.SetFilesystemAttachmentInfo(machineTag, fsTag, state.FilesystemAttachmentInfo{
		MountPoint: "/mnt/foo",
		ReadOnly:   true,
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	filesystems := model.Filesystems()
	c.Assert(filesystems, tc.HasLen, 2)
	provisioned, notProvisioned := filesystems[0], filesystems[1]

	c.Check(provisioned.Tag(), tc.Equals, fsTag)
	c.Check(provisioned.Volume(), tc.Equals, names.VolumeTag{})
	c.Check(provisioned.Storage(), tc.Equals, names.StorageTag{})
	c.Check(provisioned.Provisioned(), tc.IsTrue)
	c.Check(provisioned.Size(), tc.Equals, uint64(1500))
	c.Check(provisioned.Pool(), tc.Equals, "rootfs")
	c.Check(provisioned.FilesystemID(), tc.Equals, "filesystem id")
	attachments := provisioned.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsTrue)
	c.Check(attachment.ReadOnly(), tc.IsTrue)
	c.Check(attachment.MountPoint(), tc.Equals, "/mnt/foo")

	c.Check(notProvisioned.Tag(), tc.Equals, names.NewFilesystemTag("0/1"))
	c.Check(notProvisioned.Volume(), tc.Equals, names.VolumeTag{})
	c.Check(notProvisioned.Storage(), tc.Equals, names.StorageTag{})
	c.Check(notProvisioned.Provisioned(), tc.IsFalse)
	c.Check(notProvisioned.Size(), tc.Equals, uint64(4000))
	c.Check(notProvisioned.Pool(), tc.Equals, "rootfs")
	c.Check(notProvisioned.FilesystemID(), tc.Equals, "")
	attachments = notProvisioned.Attachments()
	c.Assert(attachments, tc.HasLen, 1)
	attachment = attachments[0]
	c.Check(attachment.Host(), tc.Equals, machineTag)
	c.Check(attachment.Provisioned(), tc.IsFalse)
	c.Check(attachment.ReadOnly(), tc.IsFalse)
	c.Check(attachment.MountPoint(), tc.Equals, "")

	// Make sure there is a status.
	status := provisioned.Status()
	c.Check(status.Value(), tc.Equals, "pending")
}

func (s *MigrationExportSuite) TestStorage(c *tc.C) {
	_, u, storageTag := s.makeUnitWithStorage(c)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	apps := model.Applications()
	c.Assert(apps, tc.HasLen, 1)
	constraints := apps[0].StorageDirectives()
	c.Assert(constraints, tc.HasLen, 2)
	cons, found := constraints["data"]
	c.Assert(found, tc.IsTrue)
	c.Check(cons.Pool(), tc.Equals, "modelscoped")
	c.Check(cons.Size(), tc.Equals, uint64(0x400))
	c.Check(cons.Count(), tc.Equals, uint64(1))
	cons, found = constraints["allecto"]
	c.Assert(found, tc.IsTrue)
	c.Check(cons.Pool(), tc.Equals, "loop")
	c.Check(cons.Size(), tc.Equals, uint64(0x400))
	c.Check(cons.Count(), tc.Equals, uint64(0))

	storages := model.Storages()
	c.Assert(storages, tc.HasLen, 1)

	storage := storages[0]

	c.Check(storage.Tag(), tc.Equals, storageTag)
	c.Check(storage.Kind(), tc.Equals, "block")
	owner, err := storage.Owner()
	c.Check(err, tc.ErrorIsNil)
	c.Check(owner, tc.Equals, u.Tag())
	c.Check(storage.Name(), tc.Equals, "data")
	c.Check(storage.Attachments(), tc.DeepEquals, []names.UnitTag{
		u.UnitTag(),
	})
}

func (s *MigrationExportSuite) TestStoragePools(c *tc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("test-pool", provider.LoopProviderType, map[string]interface{}{
		"value": 42,
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	pools := model.StoragePools()
	c.Assert(pools, tc.HasLen, 1)
	pool := pools[0]
	c.Assert(pool.Name(), tc.Equals, "test-pool")
	c.Assert(pool.Provider(), tc.Equals, "loop")
	c.Assert(pool.Attributes(), tc.DeepEquals, map[string]interface{}{
		"value": 42,
	})
}

func (s *MigrationExportSuite) TestPayloads(c *tc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	up, err := s.State.UnitPayloads(unit)
	c.Assert(err, tc.ErrorIsNil)
	original := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "something",
			Type: "special",
		},
		ID:     "42",
		Status: "running",
		Labels: []string{"foo", "bar"},
	}
	err = up.Track(original)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)

	units := applications[0].Units()
	c.Assert(units, tc.HasLen, 1)

	payloads := units[0].Payloads()
	c.Assert(payloads, tc.HasLen, 1)

	payload := payloads[0]
	c.Check(payload.Name(), tc.Equals, original.Name)
	c.Check(payload.Type(), tc.Equals, original.Type)
	c.Check(payload.RawID(), tc.Equals, original.ID)
	c.Check(payload.State(), tc.Equals, original.Status)
	c.Check(payload.Labels(), tc.DeepEquals, original.Labels)
}

func (s *MigrationExportSuite) TestResources(c *tc.C) {
	app := s.Factory.MakeApplication(c, nil)
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	st := s.State.Resources()

	setUnitResource := func(u *state.Unit) {
		_, reader, err := st.OpenResourceForUniter(u.Name(), "spam")
		c.Assert(err, tc.ErrorIsNil)
		defer reader.Close()
		_, err = io.ReadAll(reader) // Need to read the content to set the resource for the unit.
		c.Assert(err, tc.ErrorIsNil)
	}

	const body = "ham"
	const bodySize = int64(len(body))

	// Initially set revision 1 for the application.
	res1 := s.newResource(c, app.Name(), "spam", 1, body)
	res1, err := st.SetResource(app.Name(), res1.Username, res1.Resource, bytes.NewBufferString(body), state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	// Unit 1 gets revision 1.
	setUnitResource(unit1)

	// Now set revision 2 for the application.
	res2 := s.newResource(c, app.Name(), "spam", 2, body)
	res2, err = st.SetResource(app.Name(), res2.Username, res2.Resource, bytes.NewBufferString(body), state.IncrementCharmModifiedVersion)
	c.Assert(err, tc.ErrorIsNil)

	// Unit 2 gets revision 2.
	setUnitResource(unit2)

	// Revision 3 is in the charmstore.
	res3 := resourcetesting.NewCharmResource(c, "spam", body)
	res3.Revision = 3
	err = st.SetCharmStoreResources(app.Name(), []charmresource.Resource{res3}, time.Now())
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, tc.HasLen, 1)
	exApp := applications[0]

	exResources := exApp.Resources()
	c.Assert(exResources, tc.HasLen, 1)

	exResource := exResources[0]
	c.Check(exResource.Name(), tc.Equals, "spam")

	checkExRevBase := func(exRev description.ResourceRevision, res charmresource.Resource) {
		c.Check(exRev.Revision(), tc.Equals, res.Revision)
		c.Check(exRev.Type(), tc.Equals, res.Type.String())
		c.Check(exRev.Path(), tc.Equals, res.Path)
		c.Check(exRev.Description(), tc.Equals, res.Description)
		c.Check(exRev.Origin(), tc.Equals, res.Origin.String())
		c.Check(exRev.FingerprintHex(), tc.Equals, res.Fingerprint.Hex())
		c.Check(exRev.Size(), tc.Equals, bodySize)
	}

	checkExRev := func(exRev description.ResourceRevision, res resources.Resource) {
		checkExRevBase(exRev, res.Resource)
		c.Check(exRev.Timestamp().UTC(), tc.Equals, truncateDBTime(res.Timestamp))
		c.Check(exRev.Username(), tc.Equals, res.Username)
	}

	checkExRev(exResource.ApplicationRevision(), res2)

	csRev := exResource.CharmStoreRevision()
	checkExRevBase(csRev, res3)
	// These shouldn't be set for charmstore only revisions.
	c.Check(csRev.Timestamp(), tc.Equals, time.Time{})
	c.Check(csRev.Username(), tc.Equals, "")

	// Units
	units := exApp.Units()
	c.Assert(units, tc.HasLen, 2)

	checkUnitRes := func(exUnit description.Unit, unit *state.Unit, res resources.Resource) {
		c.Check(exUnit.Name(), tc.Equals, unit.Name())
		exResources := exUnit.Resources()
		c.Assert(exResources, tc.HasLen, 1)
		exRes := exResources[0]
		c.Check(exRes.Name(), tc.Equals, "spam")
		checkExRev(exRes.Revision(), res)
	}
	checkUnitRes(units[0], unit1, res1)
	checkUnitRes(units[1], unit2, res2)
}

func (s *MigrationExportSuite) newResource(c *tc.C, appName, name string, revision int, body string) resources.Resource {
	opened := resourcetesting.NewResource(c, nil, name, appName, body)
	res := opened.Resource
	res.Revision = revision
	return res
}

func (s *MigrationExportSuite) TestRemoteApplications(c *tc.C) {
	mac, err := newMacaroon("apimac")
	c.Assert(err, tc.IsNil)
	dbApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}, {
			Interface: "mysql-root",
			Name:      "db-admin",
			Limit:     5,
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}, {
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
		Spaces: []*environs.ProviderSpaceInfo{{
			CloudType: "ec2",
			ProviderAttributes: map[string]interface{}{
				"thing1":  23,
				"thing2":  "halberd",
				"network": "network-1",
			},
			SpaceInfo: network.SpaceInfo{
				Name:       "public",
				ProviderId: "juju-space-public",
				Subnets: []network.SubnetInfo{{
					ProviderId:        "juju-subnet-12",
					CIDR:              "1.2.3.0/24",
					AvailabilityZones: []string{"az1", "az2"},
					ProviderSpaceId:   "juju-space-public",
					ProviderNetworkId: "network-1",
				}},
			},
		}, {
			CloudType: "ec2",
			ProviderAttributes: map[string]interface{}{
				"thing1":  24,
				"thing2":  "bardiche",
				"network": "network-1",
			},
			SpaceInfo: network.SpaceInfo{
				Name:       "private",
				ProviderId: "juju-space-private",
				Subnets: []network.SubnetInfo{{
					ProviderId:        "juju-subnet-24",
					CIDR:              "1.2.4.0/24",
					AvailabilityZones: []string{"az1", "az2"},
					ProviderSpaceId:   "juju-space-private",
					ProviderNetworkId: "network-1",
				}},
			},
		}},
		Bindings: map[string]string{
			"db":       "private",
			"db-admin": "private",
			"logging":  "public",
		},
		// Macaroon not exported.
		Macaroon: mac,
	})
	c.Assert(err, tc.ErrorIsNil)
	state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	eps, err := s.State.InferEndpoints("gravy-rainbow", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	service := state.NewExternalControllers(s.State)
	_, err = service.Save(crossmodel.ControllerInfo{
		Addrs:         []string{"10.224.0.1:8080"},
		Alias:         "magic",
		CACert:        "magic-ca-cert",
		ControllerTag: s.Model.ControllerTag(),
	}, s.Model.UUID(), "af5a9137-934c-4b0c-8317-643b69cf4971")
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.RemoteApplications(), tc.HasLen, 1)
	app := model.RemoteApplications()[0]
	c.Check(app.Tag(), tc.Equals, names.NewApplicationTag("gravy-rainbow"))
	c.Check(app.Name(), tc.Equals, "gravy-rainbow")
	c.Check(app.OfferUUID(), tc.Equals, "offer-uuid")
	c.Check(app.URL(), tc.Equals, "me/model.rainbow")
	c.Check(app.SourceModelTag(), tc.Equals, s.Model.ModelTag())
	c.Check(app.IsConsumerProxy(), tc.IsFalse)
	c.Check(app.Bindings(), tc.DeepEquals, map[string]string{
		"db":       "private",
		"db-admin": "private",
		"logging":  "public",
	})

	c.Assert(app.Endpoints(), tc.HasLen, 3)
	ep := app.Endpoints()[0]
	c.Check(ep.Name(), tc.Equals, "db")
	c.Check(ep.Interface(), tc.Equals, "mysql")
	c.Check(ep.Role(), tc.Equals, "provider")
	ep = app.Endpoints()[1]
	c.Check(ep.Name(), tc.Equals, "db-admin")
	c.Check(ep.Interface(), tc.Equals, "mysql-root")
	c.Check(ep.Role(), tc.Equals, "provider")
	ep = app.Endpoints()[2]
	c.Check(ep.Name(), tc.Equals, "logging")
	c.Check(ep.Interface(), tc.Equals, "logging")
	c.Check(ep.Role(), tc.Equals, "provider")

	originalSpaces := dbApp.Spaces()
	actualSpaces := app.Spaces()
	c.Assert(actualSpaces, tc.HasLen, 2)
	checkSpaceMatches(c, actualSpaces[0], originalSpaces[0])
	checkSpaceMatches(c, actualSpaces[1], originalSpaces[1])

	c.Assert(model.Relations(), tc.HasLen, 1)
	rel := model.Relations()[0]
	c.Assert(rel.Key(), tc.Equals, "wordpress:db gravy-rainbow:db")
}

func checkSpaceMatches(c *tc.C, actual description.RemoteSpace, original state.RemoteSpace) {
	c.Check(actual.CloudType(), tc.Equals, original.CloudType)
	c.Check(actual.Name(), tc.Equals, original.Name)
	c.Check(actual.ProviderId(), tc.Equals, original.ProviderId)
	c.Check(actual.ProviderAttributes(), tc.DeepEquals, map[string]interface{}(original.ProviderAttributes))
	subnets := actual.Subnets()
	c.Assert(subnets, tc.HasLen, len(original.Subnets))
	for i, subnet := range subnets {
		c.Logf("subnet %d", i)
		checkSubnetMatches(c, subnet, original.Subnets[i])
	}
}

func checkSubnetMatches(c *tc.C, actual description.Subnet, original state.RemoteSubnet) {
	c.Check(actual.CIDR(), tc.Equals, original.CIDR)
	c.Check(actual.ProviderId(), tc.Equals, original.ProviderId)
	c.Check(actual.VLANTag(), tc.Equals, original.VLANTag)
	c.Check(actual.AvailabilityZones(), tc.DeepEquals, original.AvailabilityZones)
	c.Check(actual.ProviderSpaceId(), tc.Equals, original.ProviderSpaceId)
	c.Check(actual.ProviderNetworkId(), tc.Equals, original.ProviderNetworkId)
}

func (s *MigrationExportSuite) TestModelStatus(c *tc.C) {
	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(model.Status().Value(), tc.Equals, "available")
	c.Check(model.StatusHistory(), tc.HasLen, 1)
}

func (s *MigrationExportSuite) TestTooManyStatusHistories(c *tc.C) {
	// Check that we cap the history entries at 20.
	machine := s.Factory.MakeMachine(c, nil)
	s.primeStatusHistory(c, machine, status.Started, 21)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Machines(), tc.HasLen, 1)
	history := model.Machines()[0].StatusHistory()
	c.Assert(history, tc.HasLen, 20)
	s.checkStatusHistory(c, history, status.Started)
}

func (s *MigrationExportSuite) TestRelationWithNoStatus(c *tc.C) {
	// Importing from a model from before relations had status will
	// mean that there's no status to export - don't fail to export if
	// there isn't a status for a relation.
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	wordpress0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	mysql0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	ru, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)
	wordpressSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(wordpressSettings)
	c.Assert(err, tc.ErrorIsNil)

	ru, err = rel.Unit(mysql0)
	c.Assert(err, tc.ErrorIsNil)
	mysqlSettings := map[string]interface{}{
		"name": "mysql/0",
	}
	err = ru.EnterScope(mysqlSettings)
	c.Assert(err, tc.ErrorIsNil)

	state.RemoveRelationStatus(c, rel)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	rels := model.Relations()
	c.Assert(rels, tc.HasLen, 1)
	c.Assert(rels[0].Status(), tc.IsNil)
}

func (s *MigrationExportSuite) TestRemoteRelationSettingsForUnitsInCMR(c *tc.C) {
	mac, err := newMacaroon("apimac")
	c.Assert(err, tc.IsNil)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
		Spaces: []*environs.ProviderSpaceInfo{{
			CloudType:          "ec2",
			ProviderAttributes: map[string]interface{}{"network": "network-1"},
			SpaceInfo: network.SpaceInfo{
				Name:       "private",
				ProviderId: "juju-space-private",
				Subnets: []network.SubnetInfo{{
					ProviderId:        "juju-subnet-24",
					CIDR:              "1.2.4.0/24",
					AvailabilityZones: []string{"az1", "az2"},
					ProviderSpaceId:   "juju-space-private",
					ProviderNetworkId: "network-1",
				}},
			},
		}},
		Bindings: map[string]string{
			"db": "private",
		},
		// Macaroon not exported.
		Macaroon: mac,
	})
	c.Assert(err, tc.ErrorIsNil)

	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	eps, err := s.State.InferEndpoints("gravy-rainbow", "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	wordpress0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	localRU, err := rel.Unit(wordpress0)
	c.Assert(err, tc.ErrorIsNil)

	wordpressSettings := map[string]interface{}{"name": "wordpress/0"}
	err = localRU.EnterScope(wordpressSettings)
	c.Assert(err, tc.ErrorIsNil)

	remoteRU, err := rel.RemoteUnit("gravy-rainbow/0")
	c.Assert(err, tc.ErrorIsNil)

	gravySettings := map[string]interface{}{"name": "gravy-rainbow/0"}
	err = remoteRU.EnterScope(gravySettings)
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Relations(), tc.HasLen, 1)
	exRel := model.Relations()[0]
	c.Assert(exRel.Key(), tc.Equals, "wordpress:db gravy-rainbow:db")
	c.Assert(exRel.Endpoints(), tc.HasLen, 2)

	for _, exEp := range exRel.Endpoints() {
		if exEp.ApplicationName() == "wordpress" {
			c.Check(exEp.Settings(wordpress0.Name()), tc.DeepEquals, wordpressSettings)
		} else {
			c.Check(exEp.ApplicationName(), tc.Equals, "gravy-rainbow")
			c.Check(exEp.Settings("gravy-rainbow/0"), tc.DeepEquals, gravySettings)
		}
	}
}

func (s *MigrationExportSuite) TestSecrets(c *tc.C) {
	store := state.NewSecrets(s.State)
	backendStore := state.NewSecretBackends(s.State)
	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	createTime := time.Now().UTC().Round(time.Second)
	next := createTime.Add(time.Minute).Round(time.Second).UTC()
	expire := createTime.Add(2 * time.Hour).Round(time.Second).UTC()

	backendID, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
	})
	c.Assert(err, tc.ErrorIsNil)

	p := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	_, err = store.UpdateSecret(md.URI, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		AutoPrune:   ptr(true),
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	backendRefCount, err := s.State.ReadBackendRefCount(backendID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       owner.Tag(),
		Subject:     owner.Tag(),
		Role:        secrets.RoleManage,
	})
	c.Assert(err, tc.ErrorIsNil)

	consumer := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	err = s.State.SaveSecretConsumer(uri, consumer.Tag(), &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "remote-app", SourceModel: s.Model.ModelTag(), IsConsumerProxy: true})
	c.Assert(err, tc.ErrorIsNil)
	remoteConsumer := names.NewApplicationTag("remote-app")
	err = s.State.SaveSecretRemoteConsumer(uri, remoteConsumer, &secrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.UpdateModelConfig(map[string]interface{}{config.SecretBackendKey: "myvault"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	mCfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mCfg.SecretBackend(), tc.DeepEquals, "myvault")

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.SecretBackendID(), tc.Equals, backendID)

	allSecrets := model.Secrets()
	c.Assert(allSecrets, tc.HasLen, 1)
	secret := allSecrets[0]
	c.Assert(secret.Id(), tc.Equals, uri.ID)
	c.Assert(secret.Description(), tc.Equals, "my secret")
	c.Assert(secret.NextRotateTime(), tc.DeepEquals, ptr(next))
	c.Assert(secret.AutoPrune(), tc.DeepEquals, true)
	c.Assert(secret.LatestRevisionChecksum(), tc.Equals, "deadbeef")
	entity, err := secret.Owner()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Id(), tc.Equals, "mysql")
	access, ok := secret.ACL()["application-mysql"]
	c.Assert(ok, tc.IsTrue)
	c.Assert(access.Role(), tc.Equals, "manage")
	revisions := secret.Revisions()
	c.Assert(revisions, tc.HasLen, 2)
	c.Assert(revisions[0].Content(), tc.DeepEquals, map[string]string{"foo": "bar"})
	c.Assert(revisions[0].ExpireTime(), tc.DeepEquals, ptr(expire))
	c.Assert(revisions[1].ValueRef(), tc.NotNil)
	c.Assert(revisions[1].ValueRef().BackendID(), tc.DeepEquals, backendID)
	c.Assert(revisions[1].ValueRef().RevisionID(), tc.DeepEquals, "rev-id")
	consumers := secret.Consumers()
	c.Assert(consumers, tc.HasLen, 1)
	info := consumers[0]
	entity, err = info.Consumer()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Id(), tc.Equals, "wordpress")
	c.Assert(info.Label(), tc.Equals, "consumer label")
	c.Assert(info.CurrentRevision(), tc.Equals, 666)
	remoteConsumers := secret.RemoteConsumers()
	c.Assert(remoteConsumers, tc.HasLen, 1)
	rInfo := remoteConsumers[0]
	entity, err = rInfo.Consumer()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Id(), tc.Equals, "remote-app")
	c.Assert(rInfo.CurrentRevision(), tc.Equals, 666)
}

func (s *MigrationExportSuite) TestRemoteSecrets(c *tc.C) {
	store := state.NewSecrets(s.State)
	owner := s.Factory.MakeApplication(c, nil)
	consumer := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	localURI := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(localURI, p)
	c.Assert(err, tc.ErrorIsNil)

	// Create a local consumer to be sure it is excluded.
	err = s.State.SaveSecretConsumer(localURI, consumer.Tag(), &secrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	remoteUUID := "deadbeef-0bad-400d-8000-4b1d0d06f666"
	remoteURI := secrets.NewURI().WithSource(remoteUUID)
	err = s.State.SaveSecretConsumer(remoteURI, consumer.Tag(), &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 667,
		LatestRevision:  668,
	})
	c.Assert(err, tc.ErrorIsNil)

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	remote := model.RemoteSecrets()
	c.Assert(remote, tc.HasLen, 1)
	info := remote[0]
	c.Assert(info.ID(), tc.Equals, remoteURI.ID)
	c.Assert(info.SourceUUID(), tc.Equals, remoteURI.SourceUUID)
	entity, err := info.Consumer()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity.Id(), tc.Equals, "wordpress")
	c.Assert(info.Label(), tc.Equals, "consumer label")
	c.Assert(info.CurrentRevision(), tc.Equals, 667)
	c.Assert(info.LatestRevision(), tc.Equals, 668)
}

func (s *MigrationExportSuite) TestVirtualHostKeys(c *tc.C) {
	machineTag := names.NewMachineTag("0")
	state.AddVirtualHostKey(c, s.State, machineTag, []byte("foo"))

	model, err := s.State.Export(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)

	virtualHostKeys := model.VirtualHostKeys()
	c.Assert(virtualHostKeys, tc.HasLen, 1)

	exported := virtualHostKeys[0]
	c.Assert(exported.HostKey(), tc.DeepEquals, []byte("foo"))
	c.Assert(exported.ID(), tc.Equals, "machine-0-hostkey")
}
