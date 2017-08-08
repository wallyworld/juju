// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
)

var _ = gc.Suite(&crossmodelRelationsSuite{})

type crossmodelRelationsSuite struct {
	coretesting.BaseSuite

	resources     *common.Resources
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mockState
	mockStatePool *mockStatePool
	bakery        *mockBakeryService
	authContext   *commoncrossmodel.AuthContext
	api           *crossmodelrelations.CrossModelRelationsAPI

	watchedRelations params.Entities
}

func (s *crossmodelRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.bakery = &mockBakeryService{}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	s.mockStatePool = &mockStatePool{map[string]commoncrossmodel.Backend{coretesting.ModelTag.Id(): s.st}}
	fw := &mockFirewallState{}
	egressAddressWatcher := func(_ facade.Resources, fws firewall.State, relations params.Entities) (params.StringsWatchResults, error) {
		c.Assert(fw, gc.Equals, fws)
		s.watchedRelations = relations
		return params.StringsWatchResults{Results: make([]params.StringsWatchResult, len(relations.Entities))}, nil
	}
	relationStatusWatcher := func(st crossmodelrelations.CrossModelRelationsState, tag names.RelationTag) (state.RelationStatusWatcher, error) {
		c.Assert(s.st, gc.Equals, st)
		s.watchedRelations = params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
		w := &mockRelationStatusWatcher{changes: make(chan []watcher.RelationStatusChange, 1)}
		w.changes <- []watcher.RelationStatusChange{{}}
		return w, nil
	}
	var err error
	s.authContext, err = commoncrossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	api, err := crossmodelrelations.NewCrossModelRelationsAPI(
		s.st, fw, s.resources, s.authorizer, s.authContext, egressAddressWatcher, relationStatusWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *crossmodelRelationsSuite) TestPublishRelationsChanges(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	rel := newMockRelation(1)
	ru1 := newMockRelationUnit()
	ru2 := newMockRelationUnit()
	rel.units["db2/1"] = ru1
	rel.units["db2/2"] = ru2
	s.st.relations["db2:db django:db"] = rel
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishRelationChanges(params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{
			{
				Life:             params.Alive,
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2"}},
	})
	ru1.CheckCalls(c, []testing.StubCall{
		{"InScope", []interface{}{}},
		{"EnterScope", []interface{}{map[string]interface{}{"foo": "bar"}}},
	})
	ru2.CheckCalls(c, []testing.StubCall{
		{"LeaveScope", []interface{}{}},
	})
}

func (s *crossmodelRelationsSuite) assertRegisterRemoteRelations(c *gc.C) {
	app := &mockApplication{}
	app.eps = []state.Endpoint{{
		ApplicationName: "offeredapp",
		Relation:        charm.Relation{Name: "local"},
	}}
	s.st.applications["offeredapp"] = app
	s.st.offers = []crossmodel.ApplicationOffer{{
		OfferName:       "offered",
		ApplicationName: "offeredapp",
	}}
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("offer-url", "fred/prod.offered"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.RegisterRemoteRelations(params.RegisterRemoteRelationArgs{
		Relations: []params.RegisterRemoteRelationArg{{
			ApplicationToken:  "app-token",
			SourceModelTag:    coretesting.ModelTag.String(),
			RelationToken:     "rel-token",
			RemoteEndpoint:    params.RemoteEndpoint{Name: "remote"},
			OfferName:         "offered",
			LocalEndpointName: "local",
			Macaroons:         macaroon.Slice{mac},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result.Token, gc.Equals, "token-offeredapp")
	c.Check(result.Result.Macaroons, gc.HasLen, 1)
	declared := checkers.InferDeclared(result.Result.Macaroons)
	c.Assert(declared, jc.DeepEquals, checkers.Declared{
		"source-model-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"relation-key":      "offeredapp:local remote-apptoken:remote",
		"username":          "mary",
		"offer-url":         "fred/prod.offered",
	})
	cav := result.Result.Macaroons[0].Caveats()
	c.Check(cav, gc.HasLen, 5)
	c.Check(strings.HasPrefix(cav[0].Id, "time-before "), jc.IsTrue)
	c.Check(cav[1].Id, gc.Equals, "declared source-model-uuid deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(cav[2].Id, gc.Equals, "declared offer-url fred/prod.offered")
	c.Check(cav[3].Id, gc.Equals, "declared username mary")
	c.Check(cav[4].Id, gc.Equals, "declared relation-key offeredapp:local remote-apptoken:remote")

	expectedRemoteApp := s.st.remoteApplications["remote-apptoken"]
	expectedRemoteApp.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRemoteApp, jc.DeepEquals, &mockRemoteApplication{
		sourceModelUUID: coretesting.ModelTag.Id(), consumerproxy: true})
	expectedRel := s.st.relations["offeredapp:local remote-apptoken:remote"]
	expectedRel.Stub = testing.Stub{} // don't care about api calls
	c.Check(expectedRel, jc.DeepEquals, &mockRelation{id: 0, key: "offeredapp:local remote-apptoken:remote"})
	c.Check(s.st.remoteEntities, gc.HasLen, 2)
	c.Check(s.st.remoteEntities[names.NewApplicationTag("offeredapp")], gc.Equals, "token-offeredapp")
	c.Check(s.st.remoteEntities[names.NewRelationTag("offeredapp:local remote-apptoken:remote")], gc.Equals, "rel-token")
	c.Assert(s.st.offerConnections, gc.HasLen, 1)
	offerConnection := s.st.offerConnections[0]
	c.Assert(offerConnection, jc.DeepEquals, &mockOfferConnection{
		sourcemodelUUID: coretesting.ModelTag.Id(),
		relationId:      0,
		relationKey:     "offeredapp:local remote-apptoken:remote",
		username:        "mary",
		offerName:       "offered",
	})
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRegisterRemoteRelationsIdempotent(c *gc.C) {
	s.assertRegisterRemoteRelations(c)
	s.assertRegisterRemoteRelations(c)
}

func (s *crossmodelRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2"
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.RelationUnitSettings(params.RemoteRelationUnits{
		RelationUnits: []params.RemoteRelationUnit{{
			RelationToken: "token-db2",
			Unit:          "unit-django-0",
			Macaroons:     macaroon.Slice{mac},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.SettingsResult{{Settings: params.Settings{"key": "value"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *crossmodelRelationsSuite) TestPublishIngressNetworkChanges(c *gc.C) {
	s.st.remoteApplications["db2"] = &mockRemoteApplication{}
	s.st.remoteEntities[names.NewApplicationTag("db2")] = "token-db2"
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.PublishIngressNetworkChanges(params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{
			{
				ApplicationToken: "token-db2",
				RelationToken:    "token-db2:db django:db",
				Networks:         []string{"1.2.3.4/32"},
				Macaroons:        macaroon.Slice{mac},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = results.Combine()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
	// TODO(wallyworld) - add mre tests when implementation finished
}

func (s *crossmodelRelationsSuite) TestWatchEgressAddressesForRelations(c *gc.C) {
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-mysql:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
			{
				Token:     "token-postgresql:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
		},
	}
	results, err := s.api.WatchEgressAddressesForRelations(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(s.watchedRelations, jc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-postgresql:db django:db"}},
	})
	// TODO(wallyworld) - add mre tests when implementation finished
}

func (s *crossmodelRelationsSuite) TestWatchRelationsStatus(c *gc.C) {
	s.st.remoteEntities[names.NewRelationTag("db2:db django:db")] = "token-db2:db django:db"
	s.st.offerConnectionsByKey["db2:db django:db"] = &mockOfferConnection{
		offerName:       "hosted-db2",
		sourcemodelUUID: "source-model-uuid",
		relationKey:     "db2:db django:db",
		relationId:      1,
	}
	mac, err := s.bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.st.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", "db2:db django:db"),
			checkers.DeclaredCaveat("username", "mary"),
		})
	c.Assert(err, jc.ErrorIsNil)
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{
				Token:     "token-mysql:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
			{
				Token:     "token-db2:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
			{
				Token:     "token-postgresql:db django:db",
				Macaroons: macaroon.Slice{mac},
			},
		},
	}
	results, err := s.api.WatchRelationsStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Args))
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
	c.Assert(s.watchedRelations, jc.DeepEquals, params.Entities{
		Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}},
	)
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{"token-mysql:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{"token-postgresql:db django:db"}},
	})
}
