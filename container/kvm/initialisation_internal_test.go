// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"os"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/packaging"
	"github.com/juju/juju/packaging/dependency"
)

type initialisationInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestInitialisationInternalSuite(t *tctesting.T) {
	tc.Run(t, &initialisationInternalSuite{})
}

func (initialisationInternalSuite) TestCreatePool(c *tc.C) {
	tmpDir, err := os.MkdirTemp("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, tc.ErrorIsNil)
	pathfinder := func(_ paths.OS) string {
		return tmpDir
	}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(nil, pathfinder, stub.Run, chown)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(stub.Calls(), tc.DeepEquals, []string{
		fmt.Sprintf(" virsh pool-define-as juju-pool dir - - - - %s/kvm/guests", tmpDir),
		" virsh pool-build juju-pool",
		" virsh pool-start juju-pool",
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestStartPool(c *tc.C) {
	tmpDir, err := os.MkdirTemp("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, tc.ErrorIsNil)
	pathfinder := func(_ paths.OS) string {
		return tmpDir
	}
	poolInfo := &libvirtPool{Name: "juju-pool", Autostart: "no", State: "inactive"}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(poolInfo, pathfinder, stub.Run, chown)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(stub.Calls(), tc.DeepEquals, []string{
		" virsh pool-build juju-pool",
		" virsh pool-start juju-pool",
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestAutoStartPool(c *tc.C) {
	tmpDir, err := os.MkdirTemp("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, tc.ErrorIsNil)
	pathfinder := func(_ paths.OS) string {
		return tmpDir
	}
	poolInfo := &libvirtPool{Name: "juju-pool", Autostart: "no", State: "running"}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(poolInfo, pathfinder, stub.Run, chown)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(stub.Calls(), tc.DeepEquals, []string{
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestRequiredPackagesAMD64(c *tc.C) {
	got, err := dependency.KVM("amd64").PackageList(base.MustParseBaseFromString("ubuntu@20.04"))
	c.Assert(err, tc.IsNil)
	assertPkgListMatches(c, got, []string{"qemu-kvm", "qemu-utils", "genisoimage", "libvirt-daemon-system", "libvirt-clients"})
}

func (initialisationInternalSuite) TestRequiredPackagesARM64(c *tc.C) {
	got, err := dependency.KVM("amd64").PackageList(base.MustParseBaseFromString("ubuntu@20.04"))
	c.Assert(err, tc.IsNil)
	assertPkgListMatches(c, got, []string{"qemu-kvm", "qemu-utils", "genisoimage", "libvirt-daemon-system", "libvirt-clients"})
}

func assertPkgListMatches(c *tc.C, got []packaging.Package, exp []string) {
	c.Check(len(got), tc.Equals, len(exp))
	for i := 0; i < len(got); i++ {
		c.Assert(got[i].Name, tc.Equals, exp[i])
	}
}
