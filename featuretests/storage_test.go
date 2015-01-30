// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
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

var tstType = "tsttype"

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
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Name(), gc.DeepEquals, pname)
}

func (s *apiStorageSuite) TestListVolumes(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	bdi := state.BlockDeviceInfo{DeviceName: "nice"}
	err = machine.SetMachineBlockDevices(bdi)
	c.Assert(err, jc.ErrorIsNil)

	found, err := s.storageClient.ListVolumes(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Attachments, gc.HasLen, 1)
}

func (s *apiStorageSuite) TestListVolumesEmpty(c *gc.C) {
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

	//register a new storage provider
	tstProviderType := jujustorage.ProviderType(tstType)
	jujustorage.RegisterEnvironStorageProviders("dummy", tstProviderType)
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
	// TODO(anastasiamac) update when s.Factory.MakePool or similar is available
	context := runPoolList(c, []string{""})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := "[]"
	c.Assert(obtained, gc.Equals, expected)
}

func runPoolCreate(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdstorage.PoolCreateCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdStorageSuite) TestCreatePoolCmdStack(c *gc.C) {
	// TODO(anastasiamac) update when s.Factory.MakePool or similar is available
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
	dname := "ftPool"

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	bdi := state.BlockDeviceInfo{DeviceName: dname}
	err = machine.SetMachineBlockDevices(bdi)
	c.Assert(err, jc.ErrorIsNil)

	context := runVolumeList(c, []string{})

	expected := "" +
		"- attachments:\n" +
		"  - volume: disk-0\n" +
		"    storage: \"\"\n" +
		"    assigned: false\n" +
		"    machine: \"0\"\n" +
		"    attached: true\n" +
		"    device-name: ftPool\n" +
		"    uuid: \"\"\n" +
		"    label: \"\"\n" +
		"    size: 0\n" +
		"    in-use: false\n" +
		"    file-system-type: \"\"\n" +
		"    provisioned: true\n"

	c.Assert(testing.Stdout(context), gc.Equals, expected)
}
