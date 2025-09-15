// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"path/filepath"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/caasoperator"
)

type LocalStateFileSuite struct{}

func TestLocalStateFileSuite(t *tctesting.T) {
	tc.Run(t, &LocalStateFileSuite{})
}

func (s *LocalStateFileSuite) TestState(c *tc.C) {
	path := filepath.Join(c.MkDir(), "operator")
	file := caasoperator.NewStateFile(path)
	_, err := file.Read()
	c.Assert(err, tc.Equals, caasoperator.ErrNoStateFile)

	localSt := caasoperator.LocalState{
		CharmURL:             charm.MustParseURL("ch:quantal/application-name-123"),
		CharmModifiedVersion: 123,
	}
	err = file.Write(&localSt)
	c.Assert(err, tc.ErrorIsNil)
	st, err := file.Read()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.DeepEquals, &localSt)
}
