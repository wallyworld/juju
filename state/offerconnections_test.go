// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type offerConnectionsSuite struct {
	ConnSuite

	suspendedRel *state.Relation
	activeRel    *state.Relation
}

func TestOfferConnectionsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &offerConnectionsSuite{})
}

func (s *offerConnectionsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wpCh := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", wpCh)
	s.AddTestingApplication(c, "wordpress2", wpCh)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	s.activeRel, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	err = s.activeRel.SetStatus(status.StatusInfo{Status: status.Joined})
	c.Assert(err, tc.ErrorIsNil)

	eps, err = s.State.InferEndpoints("wordpress2", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	s.suspendedRel, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	err = s.suspendedRel.SetStatus(status.StatusInfo{Status: status.Suspended})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *offerConnectionsSuite) TestAddOfferConnection(c *tc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.suspendedRel.Id(),
		RelationKey:     s.suspendedRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(oc.SourceModelUUID(), tc.Equals, testing.ModelTag.Id())
	c.Assert(oc.RelationId(), tc.Equals, s.suspendedRel.Id())
	c.Assert(oc.RelationKey(), tc.Equals, s.suspendedRel.Tag().Id())
	c.Assert(oc.OfferUUID(), tc.Equals, "offer-uuid")
	c.Assert(oc.UserName(), tc.Equals, "fred")

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), tc.Equals, 2)
	c.Assert(rc.ActiveConnectionCount(), tc.Equals, 1)

	all, err := s.State.OfferConnections("offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all, tc.HasLen, 2)
	c.Assert(all[0].SourceModelUUID(), tc.Equals, testing.ModelTag.Id())
	c.Assert(all[0].RelationId(), tc.Equals, s.suspendedRel.Id())
	c.Assert(all[0].RelationKey(), tc.Equals, s.suspendedRel.Tag().Id())
	c.Assert(all[0].OfferUUID(), tc.Equals, "offer-uuid")
	c.Assert(all[0].UserName(), tc.Equals, "fred")
	c.Assert(all[0].String(),
		tc.Equals, fmt.Sprintf(`connection to "offer-uuid" by "fred" for relation %d`, s.suspendedRel.Id()))
	c.Assert(all[1].SourceModelUUID(), tc.Equals, testing.ModelTag.Id())
	c.Assert(all[1].RelationId(), tc.Equals, s.activeRel.Id())
	c.Assert(all[1].RelationKey(), tc.Equals, s.activeRel.Tag().Id())
	c.Assert(all[1].OfferUUID(), tc.Equals, "offer-uuid")
	c.Assert(all[1].UserName(), tc.Equals, "fred")
	c.Assert(all[1].String(),
		tc.Equals, fmt.Sprintf(`connection to "offer-uuid" by "fred" for relation %d`, s.activeRel.Id()))
}

func (s *offerConnectionsSuite) TestAddOfferConnectionNotFound(c *tc.C) {
	// Note: missing RelationKey to trigger a not-found error.
	_, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), tc.Equals, 1)
	c.Assert(rc.ActiveConnectionCount(), tc.Equals, 0)
}

func (s *offerConnectionsSuite) TestAddOfferConnectionTwice(c *tc.C) {
	_, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *offerConnectionsSuite) TestOfferConnectionForRelation(c *tc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.OfferConnectionForRelation("some-key")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	obtained, err := s.State.OfferConnectionForRelation(s.activeRel.Tag().Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.RelationId(), tc.Equals, oc.RelationId())
	c.Assert(obtained.RelationKey(), tc.Equals, oc.RelationKey())
	c.Assert(obtained.OfferUUID(), tc.Equals, oc.OfferUUID())
}

func (s *offerConnectionsSuite) TestOfferConnectionsForUser(c *tc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	obtained, err := s.State.OfferConnectionsForUser("mary")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 0)
	obtained, err = s.State.OfferConnectionsForUser("fred")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 1)
	c.Assert(obtained[0].OfferUUID(), tc.Equals, oc.OfferUUID())
	c.Assert(obtained[0].UserName(), tc.Equals, oc.UserName())
}

func (s *offerConnectionsSuite) TestAllOfferConnections(c *tc.C) {
	obtained, err := s.State.AllOfferConnections()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 0)

	oc1, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid1",
	})
	c.Assert(err, tc.ErrorIsNil)

	oc2, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.suspendedRel.Id(),
		RelationKey:     s.suspendedRel.Tag().Id(),
		Username:        "mary",
		OfferUUID:       "offer-uuid2",
	})
	c.Assert(err, tc.ErrorIsNil)

	obtained, err = s.State.AllOfferConnections()
	c.Assert(err, tc.ErrorIsNil)

	// Get strings for comparison. Comparing pointers is no good.
	obtainedStr := make([]string, len(obtained))
	for i, v := range obtained {
		obtainedStr[i] = v.String()
	}
	c.Assert(obtainedStr, tc.SameContents, []string{oc1.String(), oc2.String()})

}
