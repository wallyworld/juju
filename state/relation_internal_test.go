// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"
)

type RelationSuite struct{}

func TestRelationSuite(t *tctesting.T) {
	tc.Run(t, &RelationSuite{})
}

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *tc.C) {
	rel := charm.Relation{
		Interface: "ifce",
		Name:      "group",
		Role:      charm.RolePeer,
		Scope:     charm.ScopeGlobal,
	}
	eps := []Endpoint{{
		ApplicationName: "jeff",
		Relation:        rel,
	}, {
		ApplicationName: "mike",
		Relation:        rel,
	}, {
		ApplicationName: "mike",
		Relation:        rel,
	}}
	r := &Relation{nil, relationDoc{Endpoints: eps}}
	relatedEps, err := r.RelatedEndpoints("mike")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relatedEps, tc.DeepEquals, eps)
}
