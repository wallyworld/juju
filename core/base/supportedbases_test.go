// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type BasesSuite struct {
	testhelpers.IsolationSuite
}

func TestBasesSuite(t *tctesting.T) {
	tc.Run(t, &BasesSuite{})
}

func (s *BasesSuite) TestWorkloadBases(c *tc.C) {
	tests := []struct {
		name          string
		requestedBase Base
		imageStream   string
		err           string
		expectedBase  []Base
	}{{
		name:          "no base",
		requestedBase: Base{},
		imageStream:   Daily,
		expectedBase: []Base{
			MustParseBaseFromString("centos@7/stable"),
			MustParseBaseFromString("centos@9/stable"),
			MustParseBaseFromString("genericlinux@genericlinux/stable"),
			MustParseBaseFromString("kubernetes@kubernetes"),
			MustParseBaseFromString("ubuntu@20.04/stable"),
			MustParseBaseFromString("ubuntu@22.04/stable"),
			MustParseBaseFromString("ubuntu@24.04/stable"),
		},
	}, {
		name:          "requested base",
		requestedBase: MustParseBaseFromString("ubuntu@22.04"),
		imageStream:   Daily,
		expectedBase: []Base{
			MustParseBaseFromString("centos@7/stable"),
			MustParseBaseFromString("centos@9/stable"),
			MustParseBaseFromString("genericlinux@genericlinux/stable"),
			MustParseBaseFromString("kubernetes@kubernetes"),
			MustParseBaseFromString("ubuntu@20.04/stable"),
			MustParseBaseFromString("ubuntu@22.04/stable"),
			MustParseBaseFromString("ubuntu@24.04/stable"),
		},
	}}
	for _, test := range tests {
		c.Logf("test %q", test.name)

		result, err := WorkloadBases(time.Now(), test.requestedBase, test.imageStream)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.expectedBase)
	}
}
