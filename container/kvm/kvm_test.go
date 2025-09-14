// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/kvm/mock"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	coretesting "github.com/juju/juju/internal/testing"
)

type KVMSuite struct {
	kvmtesting.TestSuite
	manager container.Manager
}

func TestKVMSuite(t *tctesting.T) {
	if runtime.GOOS != "linux" || !supportedArch() {
		t.Skip("KVM is currently only supported on linux architectures amd64, arm64, and ppc64el")
	}
	tc.Run(t, &KVMSuite{})
}

func (s *KVMSuite) SetUpTest(c *tc.C) {
	s.TestSuite.SetUpTest(c)
	var err error
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:      coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey: imagemetadata.ReleasedStream,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (*KVMSuite) TestManagerModelUUIDNeeded(c *tc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{container.ConfigModelUUID: ""})
	c.Assert(err, tc.ErrorMatches, "model UUID is required")
	c.Assert(manager, tc.IsNil)
}

func (*KVMSuite) TestManagerWarnsAboutUnknownOption(c *tc.C) {
	_, err := kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID: coretesting.ModelTag.Id(),
		"shazam":                  "Captain Marvel",
	})
	c.Assert(err, tc.ErrorIsNil)
	//c.Assert(c.GetTestLog(), tc.Contains, `INFO juju.container unused config option: "shazam" -> "Captain Marvel"`)
}

func (s *KVMSuite) TestListInitiallyEmpty(c *tc.C) {
	containers, err := s.manager.ListContainers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *tc.C, name string) kvm.Container {
	kvmContainer := s.ContainerFactory.New(name)

	nics := network.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: network.EthernetDevice,
		ConfigType:    network.ConfigDHCP,
	}}
	net := container.BridgeNetworkConfig(0, nics)
	c.Assert(kvmContainer.Start(kvm.StartParams{
		Version:      "12.10",
		Arch:         arch.HostArch(),
		UserDataFile: "userdata.txt",
		Network:      net}), tc.IsNil)
	return kvmContainer
}

func (s *KVMSuite) TestListMatchesManagerName(c *tc.C) {
	s.createRunningContainer(c, "juju-06f00d-match1")
	s.createRunningContainer(c, "juju-06f00d-match2")
	s.createRunningContainer(c, "testNoMatch")
	s.createRunningContainer(c, "other")
	containers, err := s.manager.ListContainers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.HasLen, 2)
	expectedIds := []instance.Id{"juju-06f00d-match1", "juju-06f00d-match2"}
	ids := []instance.Id{containers[0].Id(), containers[1].Id()}
	c.Assert(ids, tc.SameContents, expectedIds)
}

func (s *KVMSuite) TestListMatchesRunningContainers(c *tc.C) {
	running := s.createRunningContainer(c, "juju-06f00d-running")
	s.ContainerFactory.New("juju-06f00d-stopped")
	containers, err := s.manager.ListContainers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.HasLen, 1)
	c.Assert(string(containers[0].Id()), tc.Equals, running.Name())
}

func (s *KVMSuite) TestCreateContainer(c *tc.C) {
	inst := containertesting.CreateContainer(c, s.manager, "1/kvm/0")
	name := string(inst.Id())
	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	containertesting.AssertCloudInit(c, cloudInitFilename)
}

func (s *KVMSuite) TestCreateContainerNoDefaultImageMetadata(c *tc.C) {
	var err error
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:                        coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey:                   imagemetadata.ReleasedStream,
		config.ContainerImageMetadataDefaultsDisabledKey: "true",
	})
	c.Assert(err, tc.ErrorIsNil)
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, tc.ErrorIsNil)
	_, _, err = s.manager.CreateContainer(c.Context(), instanceConfig, constraints.Value{}, base.Base{}, nil, nil,
		func(settableStatus status.Status, info string, data map[string]interface{}) error { return nil })
	c.Assert(err, tc.ErrorMatches, `no image metadata source configured: default sources disabled`)
}

// This test will pass regular unit tests, but is intended for the
// race-checking CI job to assert concurrent creation safety.
func (s *KVMSuite) TestCreateContainerConcurrent(c *tc.C) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			_ = containertesting.CreateContainer(c, s.manager, fmt.Sprintf("1/kvm/%d", idx))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func (s *KVMSuite) TestDestroyContainer(c *tc.C) {
	inst := containertesting.CreateContainer(c, s.manager, "1/kvm/0")

	err := s.manager.DestroyContainer(inst.Id())
	c.Assert(err, tc.ErrorIsNil)

	name := string(inst.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), tc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), tc.IsDirectory)
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUsesReleaseSimpleStream(c *tc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, tc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, tc.Equals, "")
	c.Assert(startParams.Stream, tc.Equals, "released")
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUsesDailySimpleStream(c *tc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, tc.ErrorIsNil)

	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:      coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey: "daily",
	})
	c.Assert(err, tc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, tc.Equals, "http://cloud-images.ubuntu.com/daily")
	c.Assert(startParams.Stream, tc.Equals, "daily")
}

func (s *KVMSuite) TestCreateContainerUsesSetImageMetadataURL(c *tc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, tc.ErrorIsNil)

	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:           coretesting.ModelTag.Id(),
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org",
	})
	c.Assert(err, tc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, tc.Equals, "https://images.linuxcontainers.org")
}

func (s *KVMSuite) TestImageAcquisitionUsesSimpleStream(c *tc.C) {

	startParams := kvm.StartParams{
		Version:          "mocked-version",
		Arch:             "mocked-arch",
		Stream:           "released",
		ImageDownloadURL: "mocked-url",
	}
	mockedContainer := kvm.NewEmptyKvmContainer()

	// We are testing only the logging side-effect, so the error is ignored.
	_ = mockedContainer.EnsureCachedImage(startParams)

	//expectedArgs := fmt.Sprintf(
	//	"synchronise images for %s %s %s %s",
	//	startParams.Arch,
	//	startParams.Version,
	//	startParams.Stream,
	//	startParams.ImageDownloadURL,
	//)
	//c.Assert(c.GetTestLog(), tc.Contains, expectedArgs)
}

type ConstraintsSuite struct {
	coretesting.BaseSuite
}

func TestConstraintsSuite(t *tctesting.T) {
	tc.Run(t, &ConstraintsSuite{})
}

func (s *ConstraintsSuite) TestDefaults(c *tc.C) {
	testCases := []struct {
		cons     string
		expected kvm.StartParams
		infoLog  []string
	}{{
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=256M",
		expected: kvm.StartParams{
			Memory:   kvm.MinMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=4G",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cores=4",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: 4,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cores=0",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.MinCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "root-disk=512M",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.MinDisk,
		},
	}, {
		cons: "root-disk=4G",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: 4,
		},
	}, {
		cons: "arch=arm64",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`arch constraint of "arm64" being ignored as not supported`,
		},
	}, {
		cons: "container=lxd",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`container constraint of "lxd" being ignored as not supported`,
		},
	}, {
		cons: "cpu-power=100",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`cpu-power constraint of 100 being ignored as not supported`,
		},
	}, {
		cons: "tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}, {
		cons: "mem=4G cores=4 root-disk=20G arch=arm64 cpu-power=100 container=lxd tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: 4,
			RootDisk: 20,
		},
		infoLog: []string{
			`arch constraint of "arm64" being ignored as not supported`,
			`container constraint of "lxd" being ignored as not supported`,
			`cpu-power constraint of 100 being ignored as not supported`,
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}}

	for _, test := range testCases {
		c.Logf("testing %q", test.cons)

		var tw loggo.TestWriter
		c.Assert(loggo.RegisterWriter("constraint-tester", &tw), tc.IsNil)
		cons := constraints.MustParse(test.cons)
		params := kvm.ParseConstraintsToStartParams(cons)
		c.Check(params, tc.DeepEquals, test.expected)

		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
		mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
		mc.AddExpr(`_._`, tc.Ignore)
		var messages []loggo.Entry
		for _, m := range test.infoLog {
			messages = append(messages, loggo.Entry{
				Level: loggo.DEBUG, Message: m,
			})
		}
		c.Check(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), messages)
		_, _ = loggo.RemoveWriter("constraint-tester")
	}
}

// Test the output when no binary can be found.
func (s *KVMSuite) TestIsKVMSupportedKvmOkNotFound(c *tc.C) {
	// With no path, and no backup directory, we should fail.
	s.PatchEnvironment("PATH", "")
	s.PatchValue(kvm.KVMPath, "")

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "kvm-ok executable not found")
}

// Test the output when the binary is found, but errors out.
func (s *KVMSuite) TestIsKVMSupportedBinaryErrorsOut(c *tc.C) {
	// Clear path so real binary is not found.
	s.PatchEnvironment("PATH", "")

	// Create mocked binary which returns an error and give the test access.
	tmpDir := c.MkDir()
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash\nexit 127"), 0777)
	c.Assert(err, tc.ErrorIsNil)
	s.PatchValue(kvm.KVMPath, tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "exit status 127")
}

// Test the case where kvm-ok is not in the path, but is in the
// specified directory.
func (s *KVMSuite) TestIsKVMSupportedNoPath(c *tc.C) {
	// Create a mocked binary so that this test does not fail for
	// developers without kvm-ok.
	s.PatchEnvironment("PATH", "")
	tmpDir := c.MkDir()
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
	c.Assert(err, tc.ErrorIsNil)
	s.PatchValue(kvm.KVMPath, tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
}

// Test the case that kvm-ok is found in the path.
func (s *KVMSuite) TestIsKVMSupportedOnlyPath(c *tc.C) {
	// Create a mocked binary so that this test does not fail for
	// developers without kvm-ok.
	tmpDir := c.MkDir()
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
	c.Check(err, tc.ErrorIsNil)
	s.PatchEnvironment("PATH", tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KVMSuite) TestKVMPathIsCorrect(c *tc.C) {
	c.Assert(*kvm.KVMPath, tc.Equals, "/usr/sbin")
}
