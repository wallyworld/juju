// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type VirtualHostKeysSuite struct {
	ConnSuite
}

func TestVirtualHostKeysSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &VirtualHostKeysSuite{})
}

func (s *VirtualHostKeysSuite) TestMachineVirtualHostKey(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key.HostKey(), tc.Not(tc.HasLen), 0)

	// check the same result with the info utility.
	info, err := virtualhostname.NewInfoMachineTarget(s.State.ModelUUID(), machine.Id())
	c.Assert(err, tc.ErrorIsNil)
	key, err = s.State.HostKeyForVirtualHostname(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key.HostKey(), tc.Not(tc.HasLen), 0)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

// TestCAASUnitVirtualHostKey verifies that a CAAS unit has a host key when created.
func (s *VirtualHostKeysSuite) TestCAASUnitVirtualHostKey(c *tc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { _ = caasSt.Close() })

	f := factory.NewFactory(caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "ubuntu", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "ubuntu", Charm: ch, NumUnits: 1})

	unitName := "ubuntu/0"

	unitNames, err := app.UnitNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)

	units, err := app.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	unit := units[0]

	key, err := caasSt.UnitVirtualHostKey(unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(key.HostKey()), tc.Matches, "(?s)-----BEGIN OPENSSH PRIVATE KEY-----\n.*")

	// check you get the same result via hostname.
	info, err := virtualhostname.NewInfoUnitTarget(caasSt.ModelUUID(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	keyViaHostname, err := caasSt.HostKeyForVirtualHostname(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key.HostKey(), tc.DeepEquals, keyViaHostname.HostKey())

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	_, err = caasSt.UnitVirtualHostKey(unitName)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

// TestCAASUnitVirtualHostKeyOnScale verifies that a CAAS unit has a host key when scaled.
func (s *VirtualHostKeysSuite) TestCAASUnitVirtualHostKeyOnScale(c *tc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *tc.C) { _ = caasSt.Close() })

	f := factory.NewFactory(caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "ubuntu", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "ubuntu", Charm: ch})

	unitName := "ubuntu/0"
	providerId := "ubuntu-0"

	// output of utils.AgentPasswordHash("juju")
	passwordHash := "v+jK3ht5NEdKeoQBfyxmlYe0"

	p := state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			UnitName:       &unitName,
			ProviderId:     &providerId,
			PasswordHash:   &passwordHash,
			VirtualHostKey: []byte("foo"),
		},
		OrderedScale: true,
	}

	err := app.SetScale(1, 0, true)
	c.Assert(err, tc.ErrorIsNil)

	unit, err := app.UpsertCAASUnit(p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit, tc.NotNil)

	key, err := caasSt.UnitVirtualHostKey(unit.UnitTag().Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(key.HostKey()), tc.Equals, "foo")

	// check you get the same result via hostname.
	info, err := virtualhostname.NewInfoUnitTarget(caasSt.ModelUUID(), unit.Name())
	c.Assert(err, tc.ErrorIsNil)
	key, err = caasSt.HostKeyForVirtualHostname(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(key.HostKey()), tc.Equals, "foo")

	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	_, err = caasSt.UnitVirtualHostKey(unit.UnitTag().Id())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitVirtualHostKeyDoesNotExist(c *tc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	application := s.AddTestingApplication(c, "wordpress", charm)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = unit.AssignToNewMachine()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.UnitVirtualHostKey(unit.Tag().Id())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *VirtualHostKeysSuite) TestIAASUnitWithPlacement(c *tc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "quantal", "wordpress")
	app := s.AddTestingApplication(c, "wordpress", ch)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, tc.ErrorIsNil)

	id, err := u.AssignedMachineId()
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.Machine(id)
	c.Assert(err, tc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key.HostKey(), tc.Not(tc.HasLen), 0)
}

// TestMissingHostKeyDoesNotBlock verifies that removing
// a machine that does not have a host key won't fail.
func (s *VirtualHostKeysSuite) TestMissingHostKeyDoesNotBlock(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	key, err := s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, tc.ErrorIsNil)

	state.RemoveVirtualHostKey(c, s.State, key)
	_, err = s.State.MachineVirtualHostKey(machine.Id())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
}
