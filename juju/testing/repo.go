// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sort"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	unitassignerapi "github.com/juju/juju/api/agent/unitassigner"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

type RepoSuite struct {
	JujuConnSuite
}

func (s *RepoSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *RepoSuite) AssertApplication(c *tc.C, name string, expectCurl string, unitCount, relCount int) (*state.Application, []*state.Relation) {
	app, err := s.State.Application(name)
	c.Assert(err, tc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.Equals, expectCurl)
	s.AssertCharmUploaded(c, expectCurl)

	units, err := app.AllUnits()
	c.Logf("Application units: %+v", units)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, unitCount)
	s.AssertUnitMachines(c, units)
	rels, err := app.Relations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rels, tc.HasLen, relCount)
	return app, rels
}

func (s *RepoSuite) AssertCharmUploaded(c *tc.C, curl string) {
	ch, err := s.State.Charm(curl)
	c.Assert(err, tc.ErrorIsNil)

	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	r, _, err := storage.Get(ch.StoragePath())
	c.Assert(err, tc.ErrorIsNil)
	defer r.Close()

	digest, _, err := utils.ReadSHA256(r)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.BundleSha256(), tc.Equals, digest)
}

func (s *RepoSuite) AssertUnitMachines(c *tc.C, units []*state.Unit) {
	tags := make([]names.UnitTag, len(units))
	expectUnitNames := make([]string, len(units))
	for i, u := range units {
		expectUnitNames[i] = u.Name()
		tags[i] = u.UnitTag()
	}

	// manually assign all units to machines.  This replaces work normally done
	// by the unitassigner code.
	errs, err := unitassignerapi.New(s.APIState).AssignUnits(tags)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, make([]error, len(units)))

	sort.Strings(expectUnitNames)

	machines, err := s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, len(units))

	unitNames := []string{}
	for _, m := range machines {
		mUnits, err := m.Units()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mUnits, tc.HasLen, 1)
		unitNames = append(unitNames, mUnits[0].Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, tc.DeepEquals, expectUnitNames)
}
