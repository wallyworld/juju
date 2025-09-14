// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

type errorsSuite struct {
}

func TestErrorsSuite(t *tctesting.T) {
	tc.Run(t, &errorsSuite{})
}

func (*errorsSuite) TestZoneIndependentErrorConforms(c *tc.C) {
	err := fmt.Errorf("fly screens on a submarine: %w", environs.ErrAvailabilityZoneIndependent)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), tc.IsTrue)

	err = fmt.Errorf("replace with solid doors: %w", err)
	err = environs.ZoneIndependentError(err)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), tc.IsTrue)

	err = fmt.Errorf("or stay on dry land: %w", err)
	c.Assert(errors.Is(err, environs.ErrAvailabilityZoneIndependent), tc.IsTrue)
}
