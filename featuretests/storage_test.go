// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/cmd/envcmd"
	cmdstorage "github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
	"github.com/juju/juju/testing"
)

var (
	tstType  = "tsttype"
	testPool = "block"
)

type apiStorageSuite struct {
	jujutesting.JujuConnSuite
	storageClient *storage.Client
}

var _ = gc.Suite(&apiStorageSuite{})

func (s *apiStorageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })

	s.storageClient = storage.NewClient(conn)
	c.Assert(s.storageClient, gc.NotNil)

	//register a new storage provider
	tstProviderType := jujustorage.ProviderType(tstType)
	jujustorage.RegisterEnvironStorageProviders("dummy", tstProviderType)
}

func (s *apiStorageSuite) TearDownTest(c *gc.C) {
	s.storageClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiStorageSuite) TestStorageShow(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakeStorage or similar is available
	storageTag := names.NewStorageTag("shared-fs/0")
	found, err := s.storageClient.Show([]names.StorageTag{storageTag})
	c.Assert(err.Error(), gc.Matches, ".*permission denied.*")
	c.Assert(found, gc.HasLen, 0)
}

func (s *apiStorageSuite) TestListPools(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakePool or similar is available
	found, err := s.storageClient.ListPools(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

func (s *apiStorageSuite) TestCreatePool(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakePool or similar is available
	pname := "pname"
	pcfg := map[string]interface{}{"just": "checking"}

	err := s.storageClient.CreatePool(pname, tstType, pcfg)
	c.Assert(err, jc.ErrorIsNil)

	assertPoolByName(c, s.State, pname)
}

func assertPoolByName(c *gc.C, st *state.State, pname string) {
	stsetts := state.NewStateSettings(st)
	poolManager := pool.NewPoolManager(stsetts)

	found, err := poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(found) > 0, jc.IsTrue)

	namesSet := make(set.Strings)
	for _, one := range found {
		namesSet.Add(one.Name())
	}
	c.Assert(namesSet.Contains(pname), jc.IsTrue)
}

type volumeSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	storageClient *storage.Client
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) SetUpTest(c *gc.C) {
	// TODO(anastasiamac 2015-01-30) mock it
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)

	setupTestPool(c, s.State)
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	st, err := juju.NewAPIFromName(cfg.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient = storage.NewClient(st)
	c.Assert(s.storageClient, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.storageClient.Close() })
}

func setupTestPool(c *gc.C, s *state.State) {
	cfg, err := s.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	//register a new storage provider
	tstProviderType := jujustorage.ProviderType(tstType)
	jujustorage.RegisterEnvironStorageProviders("dummy", tstProviderType)

	stsetts := state.NewStateSettings(s)
	poolManager := pool.NewPoolManager(stsetts)
	poolManager.Create(testPool, tstProviderType, map[string]interface{}{"it": "works"})

	jujustorage.RegisterDefaultPool(cfg.Type(), jujustorage.StorageKindBlock, testPool)
}

func makeStorageCons(pool string, size, count uint64) state.StorageConstraints {
	return state.StorageConstraints{Pool: pool, Size: size, Count: count}
}

func createUnitForTest(c *gc.C, s *jujutesting.JujuConnSuite) string {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(testPool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	devices, err := machine.BlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 1)

	return machineId
}

func (s *volumeSuite) TestListVolumes(c *gc.C) {
	createUnitForTest(c, &s.JujuConnSuite)

	found, err := s.storageClient.ListVolumes(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Attachments, gc.HasLen, 1)
}

func (s *volumeSuite) TestListVolumesEmpty(c *gc.C) {
	found, err := s.storageClient.ListVolumes(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

type cmdStorageSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&cmdStorageSuite{})

func (s *cmdStorageSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Storage)

	setupTestPool(c, s.State)
}

func runShow(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.ShowCommand{}), args...)
	c.Assert(err.Error(), gc.Matches, ".*permission denied.*")
	return context
}

func (s *cmdStorageSuite) TestStorageShowCmdStack(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakeStorage or similar is available
	context := runShow(c, []string{"shared-fs/0"})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}

func runPoolList(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolListCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestListPoolsCmdStack(c *gc.C) {
	context := runPoolList(c, []string{""})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := "- name: block  type: tsttype  config:    it: works"
	c.Assert(obtained, gc.Equals, expected)
}

func runPoolCreate(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestCreatePoolCmdStack(c *gc.C) {
	pname := "ftPool"
	context := runPoolCreate(c, []string{"-t", tstType, pname, "smth=one"})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
	assertPoolByName(c, s.State, pname)
}

func runVolumeList(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.VolumeListCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestListVolumeCmdStack(c *gc.C) {
	createUnitForTest(c, &s.RepoSuite.JujuConnSuite)

	context := runVolumeList(c, []string{})

	expected := "" +
		"- attachments:\n" +
		"  - volume: disk-0\n" +
		"    storage: storage-data-0\n" +
		"    assigned: true\n" +
		"    machine: machine-0\n" +
		"    attached: false\n" +
		"    device-name: \"\"\n" +
		"    uuid: \"\"\n" +
		"    label: \"\"\n" +
		"    size: 0\n" +
		"    in-use: false\n" +
		"    file-system-type: \"\"\n" +
		"    provisioned: false\n"
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}
