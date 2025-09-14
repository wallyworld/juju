// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/stateenvirons"
)

type environSuite struct {
	testing.JujuConnSuite
}

func TestEnvironSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &environSuite{})
}

func (s *environSuite) TestGetEnvironment(c *tc.C) {
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(s.Model)
	c.Assert(err, tc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(env.Config().UUID(), tc.DeepEquals, config.UUID())
	c.Check(env, tc.Not(tc.Equals), s.Environ)
}
