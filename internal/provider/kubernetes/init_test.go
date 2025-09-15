// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testing"
)

type initSuite struct {
	testing.BaseSuite
}

func TestInitSuite(t *tctesting.T) {
	tc.Run(t, &initSuite{})
}

func (s *initSuite) TestLabelSelectorGlobalResourcesLifecycle(c *tc.C) {
	c.Assert(
		provider.CompileLifecycleApplicationRemovalSelector().String(), tc.DeepEquals,
		`juju-resource-lifecycle notin (model,persistent)`,
	)
	c.Assert(
		provider.CompileLifecycleModelTeardownSelector().String(), tc.DeepEquals,
		`juju-resource-lifecycle notin (persistent)`,
	)
}
