// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

var (
	_ environs.NetworkingEnviron = (*environ)(nil)
)

func TestEnvironWhiteboxSuite(t *tctesting.T) {
	tc.Run(t, &environWhiteboxSuite{})
}

type environWhiteboxSuite struct{}

func (s *environWhiteboxSuite) TestSupportsContainerAddresses(c *tc.C) {
	callCtx := context.NewEmptyCloudCallContext()

	env := new(environ)
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supported, tc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), tc.IsFalse)
}
