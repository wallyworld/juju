// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type containerFactorySuite struct {
	testhelpers.IsolationSuite
}

func TestContainerFactorySuite(t *tctesting.T) {
	tc.Run(t, &containerFactorySuite{})
}

func (containerFactorySuite) TestNewContainerStartedIsNil(c *tc.C) {
	vm := new(containerFactory).New("some-kvm")

	raw, ok := vm.(*kvmContainer)
	c.Assert(ok, tc.IsTrue)

	// A new container instantiated in this way must have an "unknown"
	// started state, which will get queried and set at need.
	c.Assert(raw.started, tc.IsNil)
}
