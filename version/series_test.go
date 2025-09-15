// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type seriesSuite struct {
	testhelpers.IsolationSuite
}

func TestSeriesSuite(t *tctesting.T) {
	tc.Run(t, &seriesSuite{})
}

func (s *seriesSuite) TestDefaultSupportedLTS(c *tc.C) {
	name := DefaultSupportedLTS()
	c.Assert(name, tc.Equals, "noble")
}
