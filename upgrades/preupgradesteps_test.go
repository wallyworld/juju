// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"os/exec"
	tctesting "testing"

	"github.com/dustin/go-humanize"
	pkgmgr "github.com/juju/packaging/v4/manager"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/upgrades"
)

type preupgradechecksSuite struct {
	testing.BaseSuite
}

func TestPreupgradechecksSuite(t *tctesting.T) {
	tc.Run(t, &preupgradechecksSuite{})
}

func (s *preupgradechecksSuite) TestCheckFreeDiskSpace(c *tc.C) {
	// Expect an impossibly large amount of free disk.
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(humanize.PiByte/humanize.MiByte))
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, false, false)
	c.Assert(err, tc.ErrorMatches, `not enough free disk space on "/" for upgrade: .* available, require 1073741824MiB`)
}

func (s *preupgradechecksSuite) TestUpdateDistroInfo(c *tc.C) {
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(0))
	expectedAptCommandArgs := [][]string{
		{"update"},
		{"install", "distro-info"},
	}

	commandChan := s.HookCommandOutput(&pkgmgr.CommandOutput, nil, nil)
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, true, false)
	c.Assert(err, tc.ErrorIsNil)

	var commands []*exec.Cmd
	for i := 0; i < cap(expectedAptCommandArgs)+1; i++ {
		select {
		case cmd := <-commandChan:
			commands = append(commands, cmd)
		default:
			break
		}
	}
	if len(commands) != len(expectedAptCommandArgs) {
		c.Fatalf("expected %d commands, got %d", len(expectedAptCommandArgs), len(commands))
	}

	assertAptCommand := func(cmd *exec.Cmd, tailArgs ...string) {
		args := cmd.Args
		c.Assert(len(args), tc.GreaterThan, len(tailArgs))
		c.Assert(args[0], tc.Equals, "apt-get")
		c.Assert(args[len(args)-len(tailArgs):], tc.DeepEquals, tailArgs)
	}
	assertAptCommand(commands[0], "update")
	assertAptCommand(commands[1], "install", "distro-info")
}
