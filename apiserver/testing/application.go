// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/charm/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

func AssertPrincipalApplicationDeployed(c *tc.C, st *state.State, applicationName string, curl string, forced bool, bundle charm.Charm, cons constraints.Value) *state.Application {
	app, err := st.Application(applicationName)
	c.Assert(err, tc.ErrorIsNil)
	charm, force, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(force, tc.Equals, forced)
	c.Assert(charm.URL(), tc.DeepEquals, curl)
	// When charms are read from state, storage properties are
	// always deserialised as empty slices if empty or nil, so
	// update bundle to match (bundle comes from parsing charm
	// metadata yaml where nil means nil).
	for name, bundleMeta := range bundle.Meta().Storage {
		if bundleMeta.Properties == nil {
			bundleMeta.Properties = []string{}
			bundle.Meta().Storage[name] = bundleMeta
		}
	}
	c.Assert(charm.Meta(), tc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), tc.DeepEquals, bundle.Config())

	appCons, err := app.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appCons, tc.DeepEquals, cons)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		units, err := app.AllUnits()
		c.Assert(err, tc.ErrorIsNil)
		for _, unit := range units {
			mid, err := unit.AssignedMachineId()
			if !a.HasNext() {
				c.Assert(err, tc.ErrorIsNil)
			} else if err != nil {
				continue
			}
			machine, err := st.Machine(mid)
			c.Assert(err, tc.ErrorIsNil)
			machineCons, err := machine.Constraints()
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(machineCons, tc.DeepEquals, cons)
		}
		break
	}
	return app
}
