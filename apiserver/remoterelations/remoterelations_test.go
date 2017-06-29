// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoterelations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *remoterelations.RemoteRelationsAPI
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	api, err := remoterelations.NewRemoteRelationsAPI(s.st, common.NewControllerConfig(s.st), s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.st.remoteApplicationsWatcher.changes <- applicationNames
	result, err := s.api.WatchRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	db2RelationsWatcher := newMockStringsWatcher()
	db2RelationsWatcher.changes <- []string{"db2:db django:db"}
	s.st.applicationRelationsWatchers["db2"] = db2RelationsWatcher

	results, err := s.api.WatchRemoteApplicationRelations(params.Entities{[]params.Entity{
		{"application-db2"},
		{"application-hadoop"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.StringsWatchResult{{
		StringsWatcherId: "1",
		Changes:          []string{"db2:db django:db"},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "hadoop" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid application tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"WatchRemoteApplicationRelations", []interface{}{"hadoop"}},
	})
}

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	relationsIds := []string{"1", "2"}
	s.st.remoteRelationsWatcher.changes <- relationsIds
	result, err := s.api.WatchRemoteRelations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, relationsIds)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *remoteRelationsSuite) TestWatchLocalRelationUnits(c *gc.C) {
	djangoRelationUnitsWatcher := newMockRelationUnitsWatcher()
	djangoRelationUnitsWatcher.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{"django/0": {Version: 1}},
	}
	djangoRelation := newMockRelation(123)
	djangoRelation.endpointUnitsWatchers["django"] = djangoRelationUnitsWatcher
	djangoRelation.endpoints = []state.Endpoint{{
		ApplicationName: "db2",
	}, {
		ApplicationName: "django",
	}}

	s.st.relations["django:db db2:db"] = djangoRelation
	s.st.applications["django"] = newMockApplication("django")

	results, err := s.api.WatchLocalRelationUnits(params.Entities{[]params.Entity{
		{"relation-django:db#db2:db"},
		{"relation-hadoop:db#db2:db"},
		{"machine-42"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RelationUnitsWatchResult{{
		RelationUnitsWatcherId: "1",
		Changes: params.RelationUnitsChange{
			Changed: map[string]params.UnitSettings{
				"django/0": {
					Version: 1,
				},
			},
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `relation "hadoop:db db2:db" not found`,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid relation tag`,
		},
	}})

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"django:db db2:db"}},
		{"Application", []interface{}{"db2"}},
		{"Application", []interface{}{"django"}},
		{"KeyRelation", []interface{}{"hadoop:db db2:db"}},
	})

	djangoRelation.CheckCalls(c, []testing.StubCall{
		{"Endpoints", []interface{}{}},
		{"WatchUnits", []interface{}{"django"}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntities(c *gc.C) {
	result, err := s.api.ImportRemoteEntities(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{ModelTag: coretesting.ModelTag.String(), Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.ErrorResult{})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ImportRemoteEntity", []interface{}{coretesting.ModelTag, names.ApplicationTag{Name: "django"}, "token"}},
	})
}

func (s *remoteRelationsSuite) TestImportRemoteEntitiesTwice(c *gc.C) {
	_, err := s.api.ImportRemoteEntities(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{ModelTag: coretesting.ModelTag.String(), Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ImportRemoteEntities(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{ModelTag: coretesting.ModelTag.String(), Tag: "application-django", Token: "token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
	s.st.CheckCalls(c, []testing.StubCall{
		{"ImportRemoteEntity", []interface{}{coretesting.ModelTag, names.ApplicationTag{Name: "django"}, "token"}},
		{"ImportRemoteEntity", []interface{}{coretesting.ModelTag, names.ApplicationTag{Name: "django"}, "token"}},
	})
}

func (s *remoteRelationsSuite) TestRemoveRemoteEntities(c *gc.C) {
	result, err := s.api.RemoveRemoteEntities(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{ModelTag: coretesting.ModelTag.String(), Tag: "application-django"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.ErrorResult{})
	s.st.CheckCalls(c, []testing.StubCall{
		{"RemoveRemoteEntity", []interface{}{coretesting.ModelTag, names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestExportEntities(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.RemoteEntityIdResult{
		Result: &params.RemoteEntityId{ModelUUID: coretesting.ModelTag.Id(), Token: "token-django"},
	})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestExportEntitiesTwice(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	_, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ExportEntities(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	c.Assert(result.Results[0].Error.Code, gc.Equals, params.CodeAlreadyExists)
	c.Assert(result.Results[0].Result, jc.DeepEquals, &params.RemoteEntityId{
		ModelUUID: coretesting.ModelTag.Id(), Token: "token-django"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
		{"ExportLocalEntity", []interface{}{names.ApplicationTag{Name: "django"}}},
	})
}

func (s *remoteRelationsSuite) TestGetTokens(c *gc.C) {
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.GetTokens(params.GetTokenArgs{
		Args: []params.GetTokenArg{{ModelTag: coretesting.ModelTag.String(), Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.DeepEquals, params.StringResult{Result: "token-application-django"})
	s.st.CheckCalls(c, []testing.StubCall{
		{"GetToken", []interface{}{coretesting.ModelTag, names.NewApplicationTag("django")}},
	})
}

func (s *remoteRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	s.st.applications["django"] = newMockApplication("django")
	result, err := s.api.RelationUnitSettings(params.RelationUnits{
		RelationUnits: []params.RelationUnit{{Relation: "relation-db2.db#django.db", Unit: "unit-django-0"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.SettingsResult{{Settings: params.Settings{"key": "value"}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
	})
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *gc.C) {
	s.st.remoteApplications["django"] = newMockRemoteApplication("django", "me/model.riak")
	result, err := s.api.RemoteApplications(params.Entities{Entities: []params.Entity{{Tag: "application-django"}}})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := macaroon.New(nil, "test", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteApplicationResult{{
		Result: &params.RemoteApplication{
			Name:      "django",
			OfferName: "django-alias",
			Life:      "alive",
			ModelUUID: "model-uuid",
			Macaroon:  mac,
		}}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"RemoteApplication", []interface{}{"django"}},
	})
}

func (s *remoteRelationsSuite) TestRelations(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.units["django/0"] = djangoRelationUnit
	db2Relation.endpoints = []state.Endpoint{
		{
			ApplicationName: "django",
			Relation: charm.Relation{
				Name:      "db",
				Interface: "db2",
				Role:      "provides",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		}, {
			ApplicationName: "db2",
			Relation: charm.Relation{
				Name:      "data",
				Interface: "db2",
				Role:      "requires",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.st.relations["db2:db django:db"] = db2Relation
	app := newMockApplication("django")
	s.st.applications["django"] = app
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp
	result, err := s.api.Relations(params.Entities{Entities: []params.Entity{{Tag: "relation-db2.db#django.db"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.RemoteRelationResult{{
		Result: &params.RemoteRelation{
			Id:   123,
			Life: "alive",
			Key:  "db2:db django:db",
			RemoteApplicationName: "db2",
			RemoteEndpointName:    "data",
			ApplicationName:       "django",
			SourceModelUUID:       "model-uuid",
			Endpoint: params.RemoteEndpoint{
				Name:      "db",
				Role:      "provides",
				Interface: "db2",
				Limit:     1,
				Scope:     "global",
			}},
	}})
	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"RemoteApplication", []interface{}{"django"}},
		{"Application", []interface{}{"django"}},
		{"RemoteApplication", []interface{}{"db2"}},
	})
}

func (s *remoteRelationsSuite) TestConsumeRemoteRelationChange(c *gc.C) {
	djangoRelationUnit := newMockRelationUnit()
	djangoRelationUnit.settings["key"] = "value"
	db2Relation := newMockRelation(123)
	db2Relation.remoteUnits["django/0"] = djangoRelationUnit
	s.st.relations["db2:db django:db"] = db2Relation
	app := newMockApplication("django")
	s.st.applications["django"] = app
	remoteApp := newMockRemoteApplication("db2", "url")
	s.st.remoteApplications["db2"] = remoteApp

	_, err := s.api.ImportRemoteEntities(params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{
			{ModelTag: coretesting.ModelTag.String(), Tag: "application-django", Token: "app-token"},
			{ModelTag: coretesting.ModelTag.String(), Tag: "relation-db2:db#django:db", Token: "rel-token"},
		}})
	c.Assert(err, jc.ErrorIsNil)
	s.st.ResetCalls()

	change := params.RemoteRelationChangeEvent{
		RelationId:    params.RemoteEntityId{ModelUUID: coretesting.ModelTag.Id(), Token: "rel-token"},
		ApplicationId: params.RemoteEntityId{ModelUUID: coretesting.ModelTag.Id(), Token: "app-token"},
		Life:          params.Alive,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   0,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
	}
	changes := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	result, err := s.api.ConsumeRemoteRelationChange(changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)

	settings, err := db2Relation.remoteUnits["django/0"].Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, map[string]interface{}{"foo": "bar"})

	s.st.CheckCalls(c, []testing.StubCall{
		{"GetRemoteEntity", []interface{}{names.NewModelTag(coretesting.ModelTag.Id()), "rel-token"}},
		{"KeyRelation", []interface{}{"db2:db django:db"}},
		{"GetRemoteEntity", []interface{}{names.NewModelTag(coretesting.ModelTag.Id()), "app-token"}},
	})
}

func (s *remoteRelationsSuite) TestControllerAPIInfoForModels(c *gc.C) {
	controllerInfo := &mockControllerInfo{
		uuid: "some uuid",
		info: crossmodel.ControllerInfo{
			Addrs:  []string{"1.2.3.4/32"},
			CACert: coretesting.CACert,
		},
	}
	s.st.controllerInfo[coretesting.ModelTag.Id()] = controllerInfo
	result, err := s.api.ControllerAPIInfoForModels(
		params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Addresses, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].CACert, gc.Equals, coretesting.CACert)
}
