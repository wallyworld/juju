// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationSuite struct {
	firewallerSuite

	apiRelation *firewaller.Relation
}

func TestRelationSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &relationSuite{})
}

func (s *relationSuite) SetUpTest(c *tc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiRelation, err = s.firewaller.Relation(s.relations[0].Tag().(names.RelationTag))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationSuite) TearDownTest(c *tc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *relationSuite) TestRelation(c *tc.C) {
	_, err := s.firewaller.Relation(names.NewRelationTag("foo:db bar:db"))
	c.Assert(err, tc.ErrorMatches, `relation "foo:db bar:db" not found`)
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)

	apiRelation0, err := s.firewaller.Relation(s.relations[0].Tag().(names.RelationTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiRelation0, tc.NotNil)
}

func (s *relationSuite) TestTag(c *tc.C) {
	c.Assert(s.apiRelation.Tag(), tc.Equals, names.NewRelationTag(s.relations[0].String()))
}

func (s *relationSuite) TestLife(c *tc.C) {
	c.Assert(s.apiRelation.Life(), tc.Equals, life.Alive)
}
