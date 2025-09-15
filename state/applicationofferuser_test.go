// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type ApplicationOfferUserSuite struct {
	ConnSuite
}

func TestApplicationOfferUserSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &ApplicationOfferUserSuite{})
}

func (s *ApplicationOfferUserSuite) makeOffer(c *tc.C, access permission.Access) (*crossmodel.ApplicationOffer, names.UserTag) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "someoffer",
		ApplicationName: "mysql",
		Owner:           "test-admin",
		HasRead:         []string{"everyone@external"},
	})
	c.Assert(err, tc.ErrorIsNil)

	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:   "validusername",
			Access: permission.ReadAccess,
		})

	// Initially no access.
	_, err = s.State.GetOfferAccess(offer.OfferUUID, user.UserTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = s.State.CreateOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user.UserTag(), access)
	c.Assert(err, tc.ErrorIsNil)
	return offer, user.UserTag()
}

func (s *ApplicationOfferUserSuite) assertAddOffer(c *tc.C, wantedAccess permission.Access) string {
	offer, user := s.makeOffer(c, wantedAccess)

	access, err := s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, wantedAccess)

	// Creator of offer has admin.
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("test-admin"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)

	// Everyone has read.
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("everyone@external"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.ReadAccess)
	return offer.OfferUUID
}

func (s *ApplicationOfferUserSuite) TestAddReadOnlyOfferUser(c *tc.C) {
	s.assertAddOffer(c, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) TestAddConsumeOfferUser(c *tc.C) {
	s.assertAddOffer(c, permission.ConsumeAccess)
}

func (s *ApplicationOfferUserSuite) TestGetOfferAccess(c *tc.C) {
	offerUUID := s.assertAddOffer(c, permission.ConsumeAccess)
	users, err := s.State.GetOfferUsers(offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.DeepEquals, map[string]permission.Access{
		"everyone@external": permission.ReadAccess,
		"test-admin":        permission.AdminAccess,
		"validusername":     permission.ConsumeAccess,
	})
}

func (s *ApplicationOfferUserSuite) TestAddAdminModelUser(c *tc.C) {
	s.assertAddOffer(c, permission.AdminAccess)
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccess(c *tc.C) {
	offer, user := s.makeOffer(c, permission.AdminAccess)
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user, permission.ReadAccess)
	c.Assert(err, tc.ErrorIsNil)

	access, err := s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.ReadAccess)
}

func (s *ApplicationOfferUserSuite) setupOfferRelation(c *tc.C, offerUUID, user string) *state.Relation {
	// Make a relation to the offer.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	mysql, err := s.State.Application("mysql")
	c.Assert(err, tc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: utils.MustNewUUID().String(),
		OfferUUID:       offerUUID,
		RelationKey:     rel.Tag().Id(),
		RelationId:      rel.Id(),
		Username:        user,
	})
	c.Assert(err, tc.ErrorIsNil)
	return rel
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccessSetsRelationSuspended(c *tc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())

	// Downgrade consume access and check the relation is suspended.
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user, permission.ReadAccess)
	c.Assert(err, tc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Suspended(), tc.IsTrue)
}

func (s *ApplicationOfferUserSuite) TestUpdateOfferAccessSetsRelationSuspendedRace(c *tc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())
	var rel2 *state.Relation

	defer state.SetBeforeHooks(c, s.State, func() {
		// Add another relation to the offered app.
		curl := "local:quantal/quantal-wordpress-3"
		wpch, err := s.State.Charm(curl)
		c.Assert(err, tc.ErrorIsNil)
		wordpress2 := s.AddTestingApplication(c, "wordpress2", wpch)
		wordpressEP, err := wordpress2.Endpoint("db")
		c.Assert(err, tc.ErrorIsNil)
		mysql, err := s.State.Application("mysql")
		c.Assert(err, tc.ErrorIsNil)
		mysqlEP, err := mysql.Endpoint("server")
		c.Assert(err, tc.ErrorIsNil)
		rel2, err = s.State.AddRelation(wordpressEP, mysqlEP)
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
			SourceModelUUID: utils.MustNewUUID().String(),
			OfferUUID:       offer.OfferUUID,
			RelationKey:     rel2.Tag().Id(),
			RelationId:      rel2.Id(),
			Username:        user.Name(),
		})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	// Downgrade consume access and check both relations are suspended.
	err := s.State.UpdateOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user, permission.ReadAccess)
	c.Assert(err, tc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Suspended(), tc.IsTrue)
	err = rel2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel2.Suspended(), tc.IsTrue)
}

func (s *ApplicationOfferUserSuite) TestCreateOfferAccessNoUserFails(c *tc.C) {
	app := s.Factory.MakeApplication(c, nil)
	offers := state.NewApplicationOffers(s.State)
	_, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "someoffer",
		ApplicationName: app.Name(),
		Owner:           "test-admin",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		names.NewUserTag("validusername"), permission.ReadAccess)
	c.Assert(err, tc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccess(c *tc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)

	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetOfferAccess(offer.OfferUUID, user)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccessNoUser(c *tc.C) {
	offer, _ := s.makeOffer(c, permission.ConsumeAccess)
	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), names.NewUserTag("fred"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationOfferUserSuite) TestRemoveOfferAccessSetsRelationSuspended(c *tc.C) {
	offer, user := s.makeOffer(c, permission.ConsumeAccess)
	rel := s.setupOfferRelation(c, offer.OfferUUID, user.Name())

	// Remove any access and check the relation is suspended.
	err := s.State.RemoveOfferAccess(names.NewApplicationOfferTag(offer.OfferUUID), user)
	c.Assert(err, tc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Suspended(), tc.IsTrue)
}
