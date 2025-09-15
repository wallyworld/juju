// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

type relationNetworksSuite struct {
	ConnSuite
	relationNetworks state.RelationNetworker
	direction        string
	relation         *state.Relation
}

type relationIngressNetworksSuite struct {
	relationNetworksSuite
}

type relationEgressNetworksSuite struct {
	relationNetworksSuite
}

func TestRelationIngressNetworksSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &relationIngressNetworksSuite{})
}
func TestRelationEgressNetworksSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &relationEgressNetworksSuite{})
}

func (s *relationNetworksSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationIngressNetworksSuite) SetUpTest(c *tc.C) {
	s.relationNetworksSuite.SetUpTest(c)
	s.direction = "ingress"
	s.relationNetworks = state.NewRelationIngressNetworks(s.State)
}

func (s *relationEgressNetworksSuite) SetUpTest(c *tc.C) {
	s.relationNetworksSuite.SetUpTest(c)
	s.direction = "egress"
	s.relationNetworks = state.NewRelationEgressNetworks(s.State)
}

func (s *relationNetworksSuite) TestSaveMissingRelation(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db something:database", false, []string{"192.168.1.0/32"})
	c.Assert(err, tc.ErrorMatches, ".*"+regexp.QuoteMeta(`"wordpress:db something:database" not found`))
}

func (s *relationNetworksSuite) TestSaveInvalidAddress(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1"})
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *relationNetworksSuite) assertSavedIngressInfo(c *tc.C, relationKey string, expectedCIDRS ...string) {
	coll, closer := state.GetCollection(s.State, "relationNetworks")
	defer closer()

	var raw bson.M
	err := coll.FindId(fmt.Sprintf("%v:%v:default", relationKey, s.direction)).One(&raw)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(raw["_id"], tc.Equals, fmt.Sprintf("%v:%v:%v:default", s.State.ModelUUID(), relationKey, s.direction))
	var cidrs []string
	for _, m := range raw["cidrs"].([]interface{}) {
		cidrs = append(cidrs, m.(string))
	}
	c.Assert(cidrs, tc.SameContents, expectedCIDRS)
}

func (s *relationNetworksSuite) TestSave(c *tc.C) {
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rin.RelationKey(), tc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), tc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationNetworksSuite) TestSaveAdminOverrides(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", true, []string{"10.2.0.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rin.RelationKey(), tc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), tc.DeepEquals, []string{"10.2.0.0/16"})
	result, err := s.relationNetworks.Networks("wordpress:db mysql:server")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.CIDRS(), tc.DeepEquals, []string{"10.2.0.0/16"})
}

func (s *relationNetworksSuite) TestSaveIdempotent(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rin.RelationKey(), tc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), tc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationNetworksSuite) TestNetworks(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	result, err := s.relationNetworks.Networks("wordpress:db mysql:server")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.CIDRS(), tc.DeepEquals, []string{"192.168.1.0/16"})
	_, err = s.relationNetworks.Networks("mediawiki:db mysql:server")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *relationNetworksSuite) TestUpdateCIDRs(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"10.0.0.1/16"})
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "10.0.0.1/16")
}

func (s *relationIngressNetworksSuite) TestCrossContanination(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	egress := state.NewRelationEgressNetworks(s.State)
	_, err = egress.Networks(s.relation.Tag().Id())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *relationEgressNetworksSuite) TestCrossContanination(c *tc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, tc.ErrorIsNil)
	ingress := state.NewRelationIngressNetworks(s.State)
	_, err = ingress.Networks(s.relation.Tag().Id())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

type relationRootNetworksSuite struct {
	relationNetworksSuite
}

func TestRelationRootNetworksSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &relationRootNetworksSuite{})
}

func (s *relationRootNetworksSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)

	s.direction = "ingress"
	s.relationNetworks = state.NewRelationIngressNetworks(s.State)
}

func (s *relationRootNetworksSuite) TestAllRelationNetworks(c *tc.C) {
	s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})

	relationNetworks := state.NewRelationNetworks(s.State)
	relations, err := relationNetworks.AllRelationNetworks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.HasLen, 1)
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}
