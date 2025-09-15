// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"encoding/json"
	tctesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/mgo/v3"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/mongo"
)

type ConfigSuite struct {
	jujutesting.BaseSuite
	testhelpers.Stub

	collectionGetter func(name string) (mongo.Collection, func())
	collection       mockCollection
	closeCollection  func()

	bakeryDocResult bakeryConfigDoc
}

func TestConfigSuite(t *tctesting.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.collection = mockCollection{
		Stub: &s.Stub,
		one: func(q *mockQuery, result *interface{}) error {
			id := q.id.(string)
			if id != "bakeryConfig" {
				return mgo.ErrNotFound
			}
			*(*result).(*bakeryConfigDoc) = s.bakeryDocResult
			return nil
		},
	}
	s.closeCollection = func() {
		s.AddCall("Close")
		s.PopNoErr()
	}
	s.collectionGetter = func(collection string) (mongo.Collection, func()) {
		s.AddCall("GetCollection", collection)
		s.PopNoErr()
		return &s.collection, s.closeCollection
	}
}

func (s *ConfigSuite) TestInitialiseBakeryConfigOp(c *tc.C) {
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	op, err := bakeryConfig.InitialiseBakeryConfigOp()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.C, tc.Equals, "test")

	doc, ok := op.Insert.(*bakeryConfigDoc)
	c.Assert(ok, tc.IsTrue)
	var key bakery.KeyPair
	err = json.Unmarshal([]byte(doc.LocalUsersKey), &key)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal([]byte(doc.OffersThirdPartyKey), &key)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ConfigSuite) TestLocalUsersKey(c *tc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetLocalUsersKey()
	c.Assert(err, tc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, tc.ErrorIsNil)

	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              string(keyBytes),
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestLocalUsersThirdPartyKey(c *tc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetLocalUsersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, tc.ErrorIsNil)

	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    string(keyBytes),
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestExternalUsersThirdPartyKey(c *tc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
		OffersThirdPartyKey:        "x",
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetExternalUsersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, tc.ErrorIsNil)

	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: string(keyBytes),
			OffersThirdPartyKey:        "x",
		}}},
		{"Close", nil},
	})
}

func (s *ConfigSuite) TestOffersThirdPartyKey(c *tc.C) {
	s.bakeryDocResult = bakeryConfigDoc{
		LocalUsersKey:              "x",
		LocalUsersThirdPartyKey:    "x",
		ExternalUsersThirdPartyKey: "x",
		OffersThirdPartyKey:        `{"public":"XXy70HKjZ6SbrW0h6zb5xkQYzUAvarTDFrl4//7wgUo=","private":"AwHI3v9AQjbAzhZx0JBjqaPYhVJ5Ksi+PWog4rNwS9Y="}`,
	}
	bakeryConfig := NewBakeryConfig("test", s.collectionGetter)
	key, err := bakeryConfig.GetOffersThirdPartyKey()
	c.Assert(err, tc.ErrorIsNil)
	keyBytes, err := json.Marshal(key)
	c.Assert(err, tc.ErrorIsNil)

	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetCollection", []interface{}{"test"}},
		{"FindId", []interface{}{"bakeryConfig"}},
		{"One", []interface{}{&bakeryConfigDoc{
			LocalUsersKey:              "x",
			LocalUsersThirdPartyKey:    "x",
			ExternalUsersThirdPartyKey: "x",
			OffersThirdPartyKey:        string(keyBytes),
		}}},
		{"Close", nil},
	})
}
