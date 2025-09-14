// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/internal/testhelpers"
)

func TestStoppedSuite(t *tctesting.T) {
	tc.Run(t, &stoppedSuite{})
}

type stoppedSuite struct {
	testhelpers.IsolationSuite
}

func (*stoppedSuite) TestIsErrStopped(c *tc.C) {
	c.Assert(multiwatcher.NewErrStopped(), tc.Satisfies, multiwatcher.IsErrStopped)
	err := multiwatcher.ErrStoppedf("something")
	c.Assert(err, tc.Satisfies, multiwatcher.IsErrStopped)
	c.Assert(err.Error(), tc.Equals, "something was stopped")
}
