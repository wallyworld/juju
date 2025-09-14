// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
)

type registrySuite struct {
	testhelpers.IsolationSuite
}

func TestRegistrySuite(t *tctesting.T) {
	tc.Run(t, &registrySuite{})
}

func (*registrySuite) TestProvider(c *tc.C) {
	_, err := provider.Provider("bad")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = provider.Provider("controller")
	c.Assert(err, tc.ErrorIsNil)
}
