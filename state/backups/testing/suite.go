// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state/backups"
)

// BaseSuite is the  base suite for backups testing.
type BaseSuite struct {
	testhelpers.IsolationSuite
	// Meta is a Metadata with standard test values.
	Meta *backups.Metadata
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Meta = NewMetadata()
}
