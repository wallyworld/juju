// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	tctesting "testing"
	"time"

	"github.com/juju/collections/transform"
	jujuos "github.com/juju/os/v2"
	jujuseries "github.com/juju/os/v2/series"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type SupportedSeriesLinuxSuite struct {
	testhelpers.IsolationSuite
}

func TestSupportedSeriesLinuxSuite(t *tctesting.T) {
	tc.Run(t, &SupportedSeriesLinuxSuite{})
}

func (s *SupportedSeriesLinuxSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&LocalSeriesVersionInfo, func() (jujuos.OSType, map[string]jujuseries.SeriesVersionInfo, error) {
		return jujuos.Ubuntu, map[string]jujuseries.SeriesVersionInfo{
			"hairy": {},
		}, nil
	})
}

func (s *SupportedSeriesLinuxSuite) TestWorkloadBases(c *tc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	s.PatchValue(&UbuntuDistroInfo, tmpFile.Name())

	bases, err := WorkloadBases(time.Time{}, Base{}, "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bases, tc.DeepEquals, transform.Slice([]string{
		"centos@7", "centos@9", "genericlinux@genericlinux", "kubernetes@kubernetes",
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}
