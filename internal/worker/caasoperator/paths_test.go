// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"path/filepath"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasoperator"
)

type PathsSuite struct {
	testhelpers.IsolationSuite
}

func TestPathsSuite(t *tctesting.T) {
	tc.Run(t, &PathsSuite{})
}

func relPathFunc(base string) func(parts ...string) string {
	return func(parts ...string) string {
		allParts := append([]string{base}, parts...)
		return filepath.Join(allParts...)
	}
}

func (s *PathsSuite) TestPaths(c *tc.C) {
	dataDir := c.MkDir()
	paths := caasoperator.NewPaths(dataDir, names.NewApplicationTag("foo"))

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "application-foo"))
	c.Assert(paths, tc.DeepEquals, caasoperator.Paths{
		ToolsDir: relData("tools"),
		State: caasoperator.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			OperationsFile:  relAgent("state", "operator"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestContextInterface(c *tc.C) {
	paths := caasoperator.Paths{
		ToolsDir: "/path/to/tools",
		State: caasoperator.StatePaths{
			CharmDir:        "/path/to/charm",
			MetricsSpoolDir: "/path/to/spool/metrics",
		},
	}
	c.Assert(paths.GetToolsDir(), tc.Equals, "/path/to/tools")
	c.Assert(paths.GetCharmDir(), tc.Equals, "/path/to/charm")
	c.Assert(paths.GetMetricsSpoolDir(), tc.Equals, "/path/to/spool/metrics")
}
