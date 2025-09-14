// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/provider/dummy"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type SecretsSuite struct {
	testing.StateSuite
	store     state.SecretsStore
	owner     *state.Application
	ownerUnit *state.Unit
	relation  *state.Relation
}

func TestSecretsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsSuite{})
}

func (s *SecretsSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
	s.ownerUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.owner})
	app2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	ep1, err := s.owner.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	ep2, err := app2.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	s.relation = s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{ep1, ep2},
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestCreate(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(md, mc, &secrets.SecretMetadata{
		URI:                    uri,
		Version:                1,
		Description:            "my secret",
		Label:                  "foobar",
		RotatePolicy:           secrets.RotateDaily,
		NextRotateTime:         ptr(next),
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       ptr(expire),
		OwnerTag:               s.owner.Tag().String(),
		CreateTime:             now,
		UpdateTime:             now,
	})

	p.Label = nil
	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretsSuite) TestCreateUserSecret(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.Model.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Description: ptr("my secret"),
			Label:       ptr("label-1"),
			Params:      nil,
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(md, mc, &secrets.SecretMetadata{
		URI:                    uri,
		Version:                1,
		Description:            "my secret",
		Label:                  "label-1",
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		OwnerTag:               s.Model.Tag().String(),
		CreateTime:             now,
		UpdateTime:             now,
	})

	uri2 := secrets.NewURI()
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`user secret label "label-1" already exists`))
}

func (s *SecretsSuite) TestCreateBackendRef(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
			Checksum: "deadbeef",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.LatestRevisionChecksum, tc.Equals, "deadbeef")
	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)
	v, valueRef, err := s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v.EncodedValues(), tc.HasLen, 0)
	c.Assert(valueRef, tc.NotNil)
	c.Assert(*valueRef, tc.DeepEquals, secrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	})
}

func (s *SecretsSuite) assertCreateDuplicateLabelApplicationOwned(c *tc.C, label string) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr(label),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := secrets.NewURI()
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing application owner label should not be used for owner label for its units.
	uri3 := secrets.NewURI()
	p.Owner = s.ownerUnit.Tag()
	_, err = s.store.CreateSecret(uri3, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing application owner label should not be used for consumer label for its units.
	cmd := &secrets.SecretConsumerMetadata{
		Label:           label,
		CurrentRevision: md.LatestRevision,
	}
	err = s.State.SaveSecretConsumer(uri, s.ownerUnit.Tag(), cmd)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)
}

func (s *SecretsSuite) TestCreateDuplicateLabelApplicationOwned(c *tc.C) {
	s.assertCreateDuplicateLabelApplicationOwned(c, "foobar")
}

func (s *SecretsSuite) TestCreateDuplicateLabelApplicationOwnedSpecialChars(c *tc.C) {
	s.assertCreateDuplicateLabelApplicationOwned(c, `\U.++foo`)
}

func (s *SecretsSuite) assertCreateDuplicateLabelUnitOwned(c *tc.C, label string) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr(label),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := secrets.NewURI()
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing unit owner label should not be used for owner label for the application.
	uri3 := secrets.NewURI()
	p.Owner = s.owner.Tag()
	_, err = s.store.CreateSecret(uri3, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing unit owner label should not be used for consumer label for the application.
	cmd := &secrets.SecretConsumerMetadata{
		Label:           label,
		CurrentRevision: md.LatestRevision,
	}
	err = s.State.SaveSecretConsumer(uri, s.owner.Tag(), cmd)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)
}

func (s *SecretsSuite) TestCreateDuplicateLabelUnitOwned(c *tc.C) {
	s.assertCreateDuplicateLabelUnitOwned(c, "foobar")
}

func (s *SecretsSuite) TestCreateDuplicateLabelUnitOwnedSpecialChars(c *tc.C) {
	s.assertCreateDuplicateLabelUnitOwned(c, `\U.++foo`)
}

func (s *SecretsSuite) assertCreateDuplicateLabelUnitConsumed(c *tc.C, label string) {
	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   names.NewApplicationTag("wordpress"),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Params:      nil,
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	cmd := &secrets.SecretConsumerMetadata{
		Label:           label,
		CurrentRevision: md.LatestRevision,
	}
	err = s.State.SaveSecretConsumer(uri, s.ownerUnit.Tag(), cmd)
	c.Assert(err, tc.ErrorIsNil)

	// Existing unit consumer label should not be used for owner label.
	uri2 := secrets.NewURI()
	p.Owner = s.ownerUnit.Tag()
	p.Label = ptr(label)
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing unit consumer label should not be used for owner label for the application.
	uri3 := secrets.NewURI()
	p.Owner = s.owner.Tag()
	p.Label = ptr(label)
	_, err = s.store.CreateSecret(uri3, p)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	// Existing unit consumer label should not be used for consumer label for the application.
	cmd = &secrets.SecretConsumerMetadata{
		Label:           label,
		CurrentRevision: md.LatestRevision,
	}
	err = s.State.SaveSecretConsumer(uri, s.owner.Tag(), cmd)
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)
}

func (s *SecretsSuite) TestCreateDuplicateLabelUnitConsumed(c *tc.C) {
	s.assertCreateDuplicateLabelUnitConsumed(c, "foobar")
}

func (s *SecretsSuite) TestCreateDuplicateLabelUnitConsumedSpecialChars(c *tc.C) {
	s.assertCreateDuplicateLabelUnitConsumed(c, `\U.++foo`)
}

func (s *SecretsSuite) TestCreateDyingOwner(c *tc.C) {
	err := s.owner.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorMatches, `cannot create secret for owner "application-mysql" which is not alive`)
}

func (s *SecretsSuite) TestGetValueNotFound(c *tc.C) {
	uri, _ := secrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	_, _, err := s.store.GetSecretValue(uri, 666)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetValue(c *tc.C) {
	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	val, backendId, err := s.store.GetSecretValue(md.URI, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendId, tc.IsNil)
	c.Assert(val.EncodedValues(), tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *SecretsSuite) TestListByOwner(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	another := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mariadb"}),
	})
	now2 := s.Clock.Now().Round(time.Second).UTC()
	uri2 := secrets.NewURI()
	p2 := state.CreateSecretParams{
		Version: 1,
		Owner:   another.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri2, p2)
	c.Assert(err, tc.ErrorIsNil)

	// Create another secret to ensure it is excluded.
	uri3 := secrets.NewURI()
	p.Owner = names.NewApplicationTag("wordpress")
	_, err = s.store.CreateSecret(uri3, p)
	c.Assert(err, tc.ErrorIsNil)

	expectedList := []*secrets.SecretMetadata{{
		URI:                    uri,
		RotatePolicy:           secrets.RotateDaily,
		NextRotateTime:         ptr(next),
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       ptr(expire),
		Version:                1,
		OwnerTag:               s.owner.Tag().String(),
		Description:            "my secret",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now,
	}, {
		URI:                    uri2,
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		Version:                1,
		OwnerTag:               another.Tag().String(),
		CreateTime:             now2,
		UpdateTime:             now2,
	}}
	list, err := s.store.ListSecrets(state.SecretsFilter{
		OwnerTags: []names.Tag{s.owner.Tag(), names.NewApplicationTag("mariadb")},
	})
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)

	sortMD := func(l []*secrets.SecretMetadata) {
		sort.Slice(l, func(i, j int) bool {
			return l[i].URI.String() < l[j].URI.String()
		})
	}
	sortMD(list)
	sortMD(expectedList)
	c.Assert(list, mc, expectedList)
}

func (s *SecretsSuite) TestListByURI(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	// Create another secret to ensure it is excluded.
	uri2 := secrets.NewURI()
	p.Owner = names.NewApplicationTag("wordpress")
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(err, tc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{
		URI: uri,
	})
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(list, mc, []*secrets.SecretMetadata{{
		URI:                    uri,
		RotatePolicy:           secrets.RotateDaily,
		NextRotateTime:         ptr(next),
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       ptr(expire),
		Version:                1,
		OwnerTag:               s.owner.Tag().String(),
		Description:            "my secret",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now,
	}})
}

func (s *SecretsSuite) TestListByLabel(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	expire := now.Add(time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	// Create another secret to ensure it is excluded.
	uri2 := secrets.NewURI()
	p.Label = ptr("another")
	_, err = s.store.CreateSecret(uri2, p)
	c.Assert(err, tc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{
		Label: ptr("foobar"),
	})
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(list, mc, []*secrets.SecretMetadata{{
		URI:                    uri,
		RotatePolicy:           secrets.RotateDaily,
		NextRotateTime:         ptr(next),
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       ptr(expire),
		Version:                1,
		OwnerTag:               s.owner.Tag().String(),
		Description:            "my secret",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now,
	}})
}

func (s *SecretsSuite) TestListByConsumer(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	subject := names.NewApplicationTag("wordpress")

	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Description: ptr("my secret"),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Create another secret to ensure it is excluded.
	uri2 := secrets.NewURI()
	cp.Owner = names.NewApplicationTag("wordpress")
	_, err = s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{
		ConsumerTags: []names.Tag{subject},
	})
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(list, mc, []*secrets.SecretMetadata{{
		URI:                    uri,
		LatestRevision:         1,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		Version:                1,
		OwnerTag:               s.owner.Tag().String(),
		Description:            "my secret",
		CreateTime:             now,
		UpdateTime:             now,
	}})
}

func (s *SecretsSuite) TestListModelSecrets(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := secrets.NewURI()
	p2 := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef:    &secrets.ValueRef{BackendID: "backend-id"},
			Checksum:    "deadbeef",
		},
	}
	_, err = s.store.CreateSecret(uri2, p2)
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	caasSt := s.newCAASState(c)
	caasSecrets := state.NewSecrets(caasSt)
	uri3 := secrets.NewURI()
	p3 := state.CreateSecretParams{
		Version: 1,
		Owner:   names.NewApplicationTag("wordpress"),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef:    &secrets.ValueRef{BackendID: caasSt.ModelUUID()},
			Checksum:    "deadbeef2",
		},
	}
	_, err = caasSecrets.CreateSecret(uri3, p3)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.store.ListModelSecrets(true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]set.Strings{
		s.State.ControllerUUID(): set.NewStrings(uri.ID),
		"backend-id":             set.NewStrings(uri2.ID),
		caasSt.ModelUUID():       set.NewStrings(uri3.ID),
	})

	result, err = s.store.ListModelSecrets(false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]set.Strings{
		s.State.ControllerUUID(): set.NewStrings(uri.ID),
		"backend-id":             set.NewStrings(uri2.ID),
	})
}

func (s *SecretsSuite) newCAASState(c *tc.C) *state.State {
	cfg := jujutesting.CustomModelConfig(c, jujutesting.Attrs{
		"name": "caasmodel",
		"uuid": utils.MustNewUUID().String(),
	})
	_, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:        state.ModelTypeCAAS,
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       s.Owner,
		StorageProviderRegistry: storage.ChainedProviderRegistry{
			dummy.StorageProviders(),
			provider.CommonStorageProviders(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { st.Close() })
	s.Factory = factory.NewFactory(st, s.StatePool)
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	return st
}

func (s *SecretsSuite) TestUpdateNothing(c *tc.C) {
	up := state.UpdateSecretParams{}
	uri := secrets.NewURI()
	_, err := s.store.UpdateSecret(uri, up)
	c.Assert(err, tc.ErrorMatches, "must specify a new value or metadata to update a secret")
}

func (s *SecretsSuite) TestUpdateAll(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		Description:    ptr("big secret"),
		Label:          ptr("new label"),
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(next),
		Data:           newData,
		Checksum:       "deadbeef",
	})
}

func (s *SecretsSuite) TestUpdateRotateInterval(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(next),
	})
}

func (s *SecretsSuite) TestUpdateExpiry(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(next),
	})

	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(time.Time{}),
	})
}

func (s *SecretsSuite) TestUpdateDuplicateLabel(c *tc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Description: ptr("description"),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	uri2 := secrets.NewURI()
	cp.Label = ptr("label2")
	_, err = s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Label:       ptr("label2"),
	})
	c.Assert(errors.Is(err, state.LabelExists), tc.IsTrue)

	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Label:       ptr("label"),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateData(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})
}

func (s *SecretsSuite) TestUpdateAutoPrune(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.AutoPrune, tc.IsFalse)
	c.Assert(md.LatestRevision, tc.Equals, 1)
	s.assertUpdatedSecret(
		c, md,
		1, // Update AutoPrune should not increment the revision.
		state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			AutoPrune:   ptr(true),
		},
	)
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevision(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})
	cmd, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd, tc.DeepEquals, &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
		LatestRevision:  2,
	})
}

func (s *SecretsSuite) TestUpdateOwnerLabel(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Label:       ptr("foobar2"),
	})
	// Ensure it can be reset back to an older value.
	s.assertUpdatedSecret(c, md, 1, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Label:       ptr("foobar"),
	})
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevisionConcurrentAdd(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)

	state.SetBeforeHooks(c, s.State, func() {
		err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
		c.Assert(err, tc.ErrorIsNil)
	})

	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})
	cmd, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, tc.Equals, 2)
	cmd, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, tc.Equals, 2)
}

func (s *SecretsSuite) TestUpdateDataSetsLatestConsumerRevisionConcurrentRemove(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 1,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mysql/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)

	state.SetBeforeHooks(c, s.State, func() {
		consColl, closer := state.GetCollection(s.State, "secretConsumers")
		defer closer()
		err := consColl.Writeable().RemoveId(state.DocID(s.State, fmt.Sprintf("%s#unit-mysql-0", uri.ID)))
		c.Assert(err, tc.ErrorIsNil)

		err = state.IncSecretConsumerRefCount(s.State, uri, 1)
		c.Assert(err, tc.ErrorIsNil)
	})

	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})
	cmd, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.LatestRevision, tc.Equals, 2)
	_, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mysql/0"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) assertUpdatedSecret(c *tc.C, original *secrets.SecretMetadata, expectedRevision int, update state.UpdateSecretParams) {
	expected := *original
	expected.LatestRevision = expectedRevision
	if update.RotatePolicy != nil {
		expected.RotatePolicy = *update.RotatePolicy
		expected.NextRotateTime = update.NextRotateTime
	}
	if update.Description != nil {
		expected.Description = *update.Description
	}
	if update.AutoPrune != nil {
		expected.AutoPrune = *update.AutoPrune
	}
	if update.Label != nil {
		expected.Label = *update.Label
	}
	if update.ExpireTime != nil && !update.ExpireTime.IsZero() {
		expected.LatestExpireTime = update.ExpireTime
	}
	if update.Data != nil || update.ValueRef != nil {
		expected.LatestRevisionChecksum = update.Checksum
	}

	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	expected.UpdateTime = updated
	md, err := s.store.UpdateSecret(original.URI, update)
	c.Assert(err, tc.ErrorIsNil)

	list, err := s.store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`(*_[_]).CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`(*_[_]).UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(list, mc, []*secrets.SecretMetadata{&expected})
	expectedData := map[string]string{"foo": "bar"}
	if update.Data != nil {
		expectedData = update.Data
	}
	val, valueRef, err := s.store.GetSecretValue(md.URI, expectedRevision)
	c.Assert(err, tc.ErrorIsNil)
	if update.ValueRef != nil {
		c.Assert(valueRef, tc.NotNil)
		c.Assert(*update.ValueRef, tc.DeepEquals, *valueRef)
	} else {
		c.Assert(valueRef, tc.IsNil)
		c.Assert(val.EncodedValues(), tc.DeepEquals, expectedData)
	}
	if update.ExpireTime != nil {
		revs, err := s.store.ListSecretRevisions(md.URI)
		c.Assert(err, tc.ErrorIsNil)
		for _, r := range revs {
			if r.ExpireTime == nil && update.ExpireTime.IsZero() {
				return
			}
			if r.ExpireTime != nil && r.ExpireTime.Equal(update.ExpireTime.Round(time.Second).UTC()) {
				return
			}
		}
		c.Fatalf("expire time not set for secret revision %d", expectedRevision)
		md, err := s.store.GetSecret(original.URI)
		c.Assert(err, tc.ErrorIsNil)
		if update.ExpireTime.IsZero() {
			c.Assert(md.LatestExpireTime, tc.IsNil)
		} else {
			c.Assert(md.LatestExpireTime, tc.Equals, update.ExpireTime.Round(time.Second).UTC())
		}
	}
	if update.NextRotateTime != nil {
		nextTime := state.GetSecretNextRotateTime(c, s.State, md.URI.ID)
		c.Assert(nextTime, tc.Equals, *update.NextRotateTime)
	}
}

func (s *SecretsSuite) TestUpdateConcurrent(c *tc.C) {
	uri := secrets.NewURI()

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)

	state.SetBeforeHooks(c, s.State, func() {
		up := state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateYearly),
			NextRotateTime: ptr(next),
			Params:         nil,
			Data:           map[string]string{"foo": "baz", "goodbye": "world"},
			Checksum:       "deadbeef",
		}
		md, err = s.store.UpdateSecret(md.URI, up)
		c.Assert(err, tc.ErrorIsNil)
	})
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 3, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		RotatePolicy:   ptr(secrets.RotateHourly),
		NextRotateTime: ptr(next),
		Data:           newData,
		Checksum:       "deadbeef",
	})
}

func (s *SecretsSuite) TestChangeSecretBackendExternalToExternal(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "old-backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("old-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	_, err = backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "new-backend-id",
		Name:        "bar",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err = s.State.ReadBackendRefCount("new-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  "old-backend-id",
				RevisionID: "rev-id",
			},
			Checksum: "deadbeef",
		},
	}
	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	backendRefCount, err = s.State.ReadBackendRefCount("old-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	val, valRef, err := s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.IsEmpty(), tc.IsTrue)
	c.Assert(valRef, tc.DeepEquals, &secrets.ValueRef{
		BackendID:  "old-backend-id",
		RevisionID: "rev-id",
	})

	err = s.store.ChangeSecretBackend(state.ChangeSecretBackendParams{
		URI:      uri,
		Token:    &fakeToken{},
		Revision: 1,
		ValueRef: &secrets.ValueRef{
			BackendID:  "new-backend-id",
			RevisionID: "rev-id",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	backendRefCount, err = s.State.ReadBackendRefCount("old-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	backendRefCount, err = s.State.ReadBackendRefCount("new-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	val, valRef, err = s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.IsEmpty(), tc.IsTrue)
	c.Assert(valRef, tc.DeepEquals, &secrets.ValueRef{
		BackendID:  "new-backend-id",
		RevisionID: "rev-id",
	})
}

func (s *SecretsSuite) TestChangeSecretBackendInternalToExternal(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)

	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "new-backend-id",
		Name:        "bar",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)

	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.ReadBackendRefCount(s.Model.UUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.State.ReadBackendRefCount(s.State.ControllerUUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	backendRefCount, err := s.State.ReadBackendRefCount("new-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	val, valRef, err := s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, secrets.NewSecretValue(map[string]string{"foo": "bar"}))
	c.Assert(valRef, tc.IsNil)

	err = s.store.ChangeSecretBackend(state.ChangeSecretBackendParams{
		URI:      uri,
		Token:    &fakeToken{},
		Revision: 1,
		ValueRef: &secrets.ValueRef{
			BackendID:  "new-backend-id",
			RevisionID: "rev-id",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.ReadBackendRefCount(s.Model.UUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.State.ReadBackendRefCount(s.State.ControllerUUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	backendRefCount, err = s.State.ReadBackendRefCount("new-backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	val, valRef, err = s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.IsEmpty(), tc.IsTrue)
	c.Assert(valRef, tc.DeepEquals, &secrets.ValueRef{
		BackendID:  "new-backend-id",
		RevisionID: "rev-id",
	})
}

func (s *SecretsSuite) TestChangeSecretBackendExternalToInternal(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
			Checksum: "deadbeef",
		},
	}
	_, err = s.store.CreateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.ReadBackendRefCount(s.Model.UUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.State.ReadBackendRefCount(s.State.ControllerUUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	val, valRef, err := s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.IsEmpty(), tc.IsTrue)
	c.Assert(valRef, tc.DeepEquals, &secrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	})

	err = s.store.ChangeSecretBackend(state.ChangeSecretBackendParams{
		URI:      uri,
		Token:    &fakeToken{},
		Data:     map[string]string{"foo": "bar"},
		Revision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.ReadBackendRefCount(s.Model.UUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.State.ReadBackendRefCount(s.State.ControllerUUID())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	val, valRef, err = s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, secrets.NewSecretValue(map[string]string{"foo": "bar"}))
	c.Assert(valRef, tc.IsNil)
}

func (s *SecretsSuite) TestSecretGrants(c *tc.C) {
	uri := secrets.NewURI()

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			Label:          strPtr("label-1"),
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Check(err, tc.ErrorIsNil)
	c.Check(md.URI, tc.DeepEquals, uri)

	subject := names.NewApplicationTag("wordpress")
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)

	access, err := s.store.SecretGrants(uri, secrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.DeepEquals, []secrets.AccessInfo{
		{
			Target: subject.String(),
			Scope:  "relation-wordpress.db#mysql.server",
			Role:   secrets.RoleView,
		},
	})
}

func (s *SecretsSuite) TestGetSecret(c *tc.C) {
	uri := secrets.NewURI()

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			Label:          strPtr("label-1"),
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Check(err, tc.ErrorIsNil)
	c.Check(md.URI, tc.DeepEquals, uri)

	md, err = s.store.GetSecret(uri)
	c.Check(err, tc.ErrorIsNil)
	c.Check(md.URI, tc.DeepEquals, uri)
}

func (s *SecretsSuite) TestListSecretRevisions(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
	})

	backendStore := state.NewSecretBackends(s.State)
	backendID, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	updateTime := s.Clock.Now().Round(time.Second).UTC()
	s.assertUpdatedSecret(c, md, 3, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef",
	})
	updateTime2 := s.Clock.Now().Round(time.Second).UTC()
	r, err := s.store.ListSecretRevisions(uri)
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(r, mc, []*secrets.SecretRevisionMetadata{{
		Revision:   1,
		CreateTime: now,
		UpdateTime: now,
	}, {
		Revision:   2,
		CreateTime: updateTime,
		UpdateTime: updateTime,
	}, {
		Revision: 3,
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		BackendName: ptr("myvault"),
		CreateTime:  updateTime2,
		UpdateTime:  updateTime2,
	}})
}

func (s *SecretsSuite) TestListUnusedSecretRevisions(c *tc.C) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})

	backendStore := state.NewSecretBackends(s.State)
	backendID, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.assertUpdatedSecret(c, md, 3, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef2",
	})
	r, err := s.store.ListUnusedSecretRevisions(uri)
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(r, mc, []int{
		1, 2,
		// The latest revision `3` is still in use, so it's not returned.
	})
}

func (s *SecretsSuite) TestGetSecretRevision(c *tc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	newData := map[string]string{"foo": "bar", "hello": "world"}
	s.assertUpdatedSecret(c, md, 2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        newData,
		Checksum:    "deadbeef",
	})
	r, err := s.store.GetSecretRevision(uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	updateTime := s.Clock.Now().Round(time.Second).UTC()
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, tc.Almost, tc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, tc.Almost, tc.ExpectedValue)
	c.Assert(r, mc, &secrets.SecretRevisionMetadata{
		Revision:   2,
		CreateTime: updateTime,
		UpdateTime: updateTime,
	})
}

func (s *SecretsSuite) TestGetSecretConsumerAndGetSecretConsumerURI(c *tc.C) {
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Label:       strPtr("owner-label"),
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	uri := secrets.NewURI()
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	md := &secrets.SecretConsumerMetadata{
		Label:           "consumer-label",
		CurrentRevision: 666,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), md)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretConsumer(nil, names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorMatches, `empty URI`)

	md2, err := s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(md2, tc.DeepEquals, md)

	uri3, err := s.State.GetURIByConsumerLabel("consumer-label", names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(uri3, tc.DeepEquals, uri)

	_, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mysql/0"))
	c.Check(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SecretsSuite) TestGetSecretConsumerCrossModelURI(c *tc.C) {
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Label:       strPtr("owner-label"),
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	uri := secrets.NewURI().WithSource("deadbeef-1bad-500d-9000-4b1d0d06f00d")
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	md := &secrets.SecretConsumerMetadata{
		Label:           "consumer-label",
		CurrentRevision: 666,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), md)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretConsumer(nil, names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorMatches, `empty URI`)

	md2, err := s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(md2, tc.DeepEquals, md)

	uri3, err := s.State.GetURIByConsumerLabel("consumer-label", names.NewUnitTag("mariadb/0"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(uri3, tc.DeepEquals, uri)
}

func (s *SecretsSuite) TestSaveSecretConsumer(c *tc.C) {
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	uri := secrets.NewURI()
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.LatestRevision, tc.Equals, 1)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 1), tc.IsFalse)

	cmd := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: md.LatestRevision,
		LatestRevision:  md.LatestRevision,
	}
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, cmd)
	c.Assert(md2.LatestRevision, tc.Equals, 1)
	c.Assert(md2.CurrentRevision, tc.Equals, 1)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 1), tc.IsFalse)

	// secret revison ++, but not obsolete.
	md, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar", "baz": "qux"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.LatestRevision, tc.Equals, 2)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 1), tc.IsFalse)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 2), tc.IsFalse)

	// consumer latest revison ++, but not obsolete.
	cmd.LatestRevision = md.LatestRevision
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)
	md2, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, cmd)
	c.Assert(md2.LatestRevision, tc.Equals, 2)
	c.Assert(md2.CurrentRevision, tc.Equals, 1)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 1), tc.IsFalse)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 2), tc.IsFalse)

	// consumer current revison ++, then obsolete.
	cmd.CurrentRevision = md.LatestRevision
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
	c.Assert(err, tc.ErrorIsNil)
	md2, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, cmd)
	c.Assert(md2.LatestRevision, tc.Equals, 2)
	c.Assert(md2.CurrentRevision, tc.Equals, 2)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 1), tc.IsTrue)
	c.Assert(s.State.IsSecretRevisionObsolete(c, uri, 2), tc.IsFalse)
}

func (s *SecretsSuite) TestSaveSecretConsumerConcurrent(c *tc.C) {
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	uri := secrets.NewURI()
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	md := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 666,
	}
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), &secrets.SecretConsumerMetadata{CurrentRevision: 668})
		c.Assert(err, tc.ErrorIsNil)
	})
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), md)
	c.Assert(err, tc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, md)
}

func (s *SecretsSuite) TestSaveSecretConsumerDifferentModel(c *tc.C) {
	uri := secrets.NewURI().WithSource("some-uuid")
	md := &secrets.SecretConsumerMetadata{
		Label:           "foobar",
		CurrentRevision: 666,
	}
	err := s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), md)
	c.Assert(err, tc.ErrorIsNil)
	md2, err := s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, md)
	md.CurrentRevision = 668
	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), md)
	c.Assert(err, tc.ErrorIsNil)
	md2, err = s.State.GetSecretConsumer(uri, names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md2, tc.DeepEquals, md)
}

func (s *SecretsSuite) TestSecretGrantAccess(c *tc.C) {
	uri := secrets.NewURI()
	subject := names.NewApplicationTag("wordpress")
	err := s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, subject)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleView)
}

func (s *SecretsSuite) TestSecretGrantCrossModelOffer(c *tc.C) {
	s.assertSecretGrantCrossModelOffer(c, true, false)
}

func (s *SecretsSuite) TestSecretGrantCrossModelConsumer(c *tc.C) {
	s.assertSecretGrantCrossModelOffer(c, false, false)
}

func (s *SecretsSuite) TestSecretGrantCrossModelConsumerUnit(c *tc.C) {
	s.assertSecretGrantCrossModelOffer(c, false, true)
}

func (s *SecretsSuite) TestSecretGrantCrossModelUnit(c *tc.C) {
	s.assertSecretGrantCrossModelOffer(c, true, true)
}

func (s *SecretsSuite) assertSecretGrantCrossModelOffer(c *tc.C, offer, unit bool) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "remote-wordpress",
		SourceModel:     names.NewModelTag("source-model"),
		IsConsumerProxy: offer,
		OfferUUID:       "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	mysqlEP, err := s.owner.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)

	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	subject := rwordpress.Tag()
	if unit {
		subject = names.NewUnitTag(rwordpress.Name() + "/0")
	}
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	if offer && !unit {
		c.Assert(err, tc.ErrorIsNil)
		access, err := s.State.SecretAccess(uri, rwordpress.Tag())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(access, tc.Equals, secrets.RoleView)
	} else {
		c.Assert(err, tc.Satisfies, errors.IsNotSupported)
	}
}

func (s *SecretsSuite) TestSecretGrantAccessDyingScope(c *tc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure destroy only sets relation to dying.
	wordpress, err := s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := s.relation.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.relation.DestroyWithForce(true, time.Second)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     wordpress.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorMatches, `cannot grant access to secret in scope of "relation-wordpress.db#mysql.server" which is not alive`)
}

func (s *SecretsSuite) TestSecretGrantAccessDyingSubject(c *tc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure destroy only sets app to dying.
	wordpress, err := s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := s.relation.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	err = wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     wordpress.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorMatches, `cannot grant access to secret in scope of "relation-wordpress.db#mysql.server" which is not alive`)
}

func (s *SecretsSuite) TestSecretRevokeAccess(c *tc.C) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	subject := names.NewApplicationTag("wordpress")
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, subject)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleView)

	err = s.State.RevokeSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Subject:     subject,
	})
	c.Assert(err, tc.ErrorIsNil)
	access, err = s.State.SecretAccess(uri, subject)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleNone)

	err = s.State.RevokeSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Subject:     subject,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SecretsSuite) TestSecretAccessScope(c *tc.C) {
	uri := secrets.NewURI()
	subject := names.NewApplicationTag("wordpress")

	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       s.relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)
	scope, err := s.State.SecretAccessScope(uri, subject)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, s.relation.Tag())
}

func (s *SecretsSuite) TestDelete(c *tc.C) {
	subject := names.NewApplicationTag("wordpress")
	create := func(label string) *secrets.URI {
		uri := secrets.NewURI()
		now := s.Clock.Now().Round(time.Second).UTC()
		next := now.Add(time.Minute).Round(time.Second).UTC()
		cp := state.CreateSecretParams{
			Version: 1,
			Owner:   s.owner.Tag(),
			UpdateSecretParams: state.UpdateSecretParams{
				LeaderToken:    &fakeToken{},
				RotatePolicy:   ptr(secrets.RotateDaily),
				NextRotateTime: ptr(next),
				Label:          ptr(label),
				Data:           map[string]string{"foo": "bar"},
				Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
			},
		}
		_, err := s.store.CreateSecret(uri, cp)
		c.Assert(err, tc.ErrorIsNil)
		cmd := &secrets.SecretConsumerMetadata{
			Label:           "consumer-" + label,
			CurrentRevision: 1,
		}
		err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), cmd)
		c.Assert(err, tc.ErrorIsNil)
		err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
			LeaderToken: &fakeToken{},
			Scope:       s.relation.Tag(),
			Subject:     subject,
			Role:        secrets.RoleView,
		})
		c.Assert(err, tc.ErrorIsNil)
		return uri
	}
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri1 := create("label1")
	up := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef",
	}
	_, err = s.store.UpdateSecret(uri1, up)
	c.Assert(err, tc.ErrorIsNil)

	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	uri2 := create("label2")

	external, err := s.store.DeleteSecret(uri1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(external, tc.DeepEquals, []secrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	_, _, err = s.store.GetSecretValue(uri1, 1)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	external, err = s.store.DeleteSecret(uri1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(external, tc.HasLen, 0)

	// Check that other secret info remains intact.
	secretRevisionsCollection, closer := state.GetCollection(s.State, "secretRevisions")
	defer closer()
	n, err := secretRevisionsCollection.FindId(uri2.ID + "/1").Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)
	n, err = secretRevisionsCollection.Find(nil).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)

	secretRotateCollection, closer := state.GetCollection(s.State, "secretRotate")
	defer closer()
	n, err = secretRotateCollection.FindId(uri2.ID).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)
	n, err = secretRotateCollection.Find(nil).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)

	secretConsumersCollection, closer := state.GetCollection(s.State, "secretConsumers")
	defer closer()
	n, err = secretConsumersCollection.FindId(uri2.ID + "#unit-mariadb-0").Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)
	n, err = secretConsumersCollection.Find(nil).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)

	secretPermissionsCollection, closer := state.GetCollection(s.State, "secretPermissions")
	defer closer()
	n, err = secretPermissionsCollection.FindId(uri2.ID + "#application-wordpress").Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)
	n, err = secretPermissionsCollection.Find(nil).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)

	refCountsCollection, closer := state.GetCollection(s.State, "refcounts")
	defer closer()
	n, err = refCountsCollection.FindId(uri2.ID + "#consumer").Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 1)
	n, err = refCountsCollection.FindId(uri1.ID + "#consumer").Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(n, tc.Equals, 0)

	// Check we can now reuse the label.
	create("label1")
}

func (s *SecretsSuite) TestDeleteRevisions(c *tc.C) {
	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err := s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)

	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar2"},
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 1)

	external, err := s.store.DeleteSecret(uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(external, tc.HasLen, 0)
	_, _, err = s.store.GetSecretValue(uri, 1)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	val, _, err := s.store.GetSecretValue(uri, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val.EncodedValues(), tc.DeepEquals, map[string]string{"foo": "bar2"})
	_, ref, err := s.store.GetSecretValue(uri, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.NotNil)
	c.Assert(ref, tc.DeepEquals, &secrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	})

	external, err = s.store.DeleteSecret(uri, 1, 2, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(external, tc.DeepEquals, []secrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	_, _, err = s.store.GetSecretValue(uri, 3)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.store.GetSecret(uri)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	backendRefCount, err = s.State.ReadBackendRefCount("backend-id")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendRefCount, tc.Equals, 0)
}

func (s *SecretsSuite) TestSecretRotated(c *tc.C) {
	uri := secrets.NewURI()

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.State.SecretRotated(uri, next2)
	c.Assert(err, tc.ErrorIsNil)

	nextTime := state.GetSecretNextRotateTime(c, s.State, md.URI.ID)
	c.Assert(nextTime, tc.Equals, next2)
}

func (s *SecretsSuite) TestSecretRotatedConcurrent(c *tc.C) {
	uri := secrets.NewURI()

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	later := now.Add(time.Hour).Round(time.Second).UTC()
	later2 := now.Add(2 * time.Hour).Round(time.Second).UTC()
	state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SecretRotated(uri, later)
		c.Assert(err, tc.ErrorIsNil)
	})

	err = s.State.SecretRotated(uri, later2)
	c.Assert(err, tc.ErrorIsNil)

	nextTime := state.GetSecretNextRotateTime(c, s.State, md.URI.ID)
	c.Assert(nextTime, tc.Equals, later)
}

type SecretsRotationWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	ownerApp  *state.Application
	ownerUnit *state.Unit
}

func TestSecretsRotationWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsRotationWatcherSuite{})
}

func (s *SecretsRotationWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.ownerApp = s.Factory.MakeApplication(c, nil)
	s.ownerUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.ownerApp})
}

func (s *SecretsRotationWatcherSuite) setupWatcher(c *tc.C) (state.SecretsTriggerWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerApp.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	w, err := s.State.WatchSecretsRotationChanges(
		[]names.Tag{s.ownerApp.Tag(), s.ownerUnit.Tag()})
	c.Assert(err, tc.ErrorIsNil)

	wc := testing.NewSecretsTriggerWatcherC(c, w)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             md.URI,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsRotationWatcherSuite) TestWatchInitialEvent(c *tc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsRotationWatcherSuite) TestWatchSingleUpdate(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(2 * time.Hour).Round(time.Second).UTC()
	err := s.State.SecretRotated(uri, next)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchDelete(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI: md.URI,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdatesSameSecret(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	err := s.State.SecretRotated(uri, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next,
	})
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.State.SecretRotated(uri, next2)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next2,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdatesSameSecretDeleted(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.State.SecretRotated(uri, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next,
	})
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI: md.URI,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchMultipleUpdates(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.State.SecretRotated(uri, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next,
	})

	uri2 := secrets.NewURI()
	next2 := now.Add(time.Minute).Round(time.Second).UTC()
	md2, err := s.store.CreateSecret(uri2, state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerApp.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateHourly),
			NextRotateTime: ptr(next2),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             md2.URI,
		NextTriggerTime: next2,
	})

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI: md.URI,
	})
	wc.AssertNoChange()
}

func (s *SecretsRotationWatcherSuite) TestWatchRestartChangeOwners(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next1 := now.Add(time.Minute).Round(time.Second).UTC()
	next2 := now.Add(time.Minute).Round(time.Second).UTC()

	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateHourly),
			NextRotateTime: ptr(next2),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	next3 := now.Add(time.Minute).Round(time.Second).UTC()
	anotherUnit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.ownerApp})

	uri3 := secrets.NewURI()
	cp = state.CreateSecretParams{
		Version: 1,
		Owner:   anotherUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateHourly),
			NextRotateTime: ptr(next3),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri3, cp)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri2,
		NextTriggerTime: next2,
	})

	wc.AssertNoChange()
	testing.AssertStop(c, w)

	w, err = s.State.WatchSecretsRotationChanges(
		[]names.Tag{s.ownerApp.Tag(), anotherUnit.Tag()})
	c.Assert(err, tc.ErrorIsNil)

	wc = testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: next1,
	}, watcher.SecretTriggerChange{
		URI:             uri3,
		NextTriggerTime: next3,
	})
	wc.AssertNoChange()
}

type SecretsExpiryWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	ownerApp  *state.Application
	ownerUnit *state.Unit
}

func TestSecretsExpiryWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsExpiryWatcherSuite{})
}

func (s *SecretsExpiryWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.ownerApp = s.Factory.MakeApplication(c, nil)
	s.ownerUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.ownerApp})
}

func (s *SecretsExpiryWatcherSuite) setupWatcher(c *tc.C) (state.SecretsTriggerWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerApp.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ExpireTime:  ptr(next),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	w, err := s.State.WatchSecretRevisionsExpiryChanges(
		[]names.Tag{s.ownerApp.Tag(), s.ownerUnit.Tag()})
	c.Assert(err, tc.ErrorIsNil)

	wc := testing.NewSecretsTriggerWatcherC(c, w)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             md.URI,
		Revision:        1,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsExpiryWatcherSuite) TestWatchInitialEvent(c *tc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsExpiryWatcherSuite) TestWatchSingleUpdate(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(2 * time.Hour).Round(time.Second).UTC()

	s.Clock.Advance(time.Hour)
	updated := s.Clock.Now().Round(time.Second).UTC()
	update := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(next),
	}
	md, err := s.store.UpdateSecret(uri, update)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(md.LatestExpireTime, tc.NotNil)
	c.Assert(*md.LatestExpireTime, tc.Equals, next)

	revs, err := s.store.ListSecretRevisions(md.URI)
	c.Assert(err, tc.ErrorIsNil)
	for _, r := range revs {
		if r.ExpireTime != nil && r.ExpireTime.Equal(update.ExpireTime.Round(time.Second).UTC()) {
			c.Assert(r.UpdateTime, tc.Almost, updated)
			return
		}
	}
	c.Fatalf("expire time not set for secret revision %d", 2)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		Revision:        3,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
}

func (s *SecretsExpiryWatcherSuite) TestWatchSetExpiryToNil(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(time.Time{}),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:      md.URI,
		Revision: 1,
	})
	wc.AssertNoChange()
}

func (s *SecretsExpiryWatcherSuite) TestWatchMultipleUpdates(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	md, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(time.Time{}),
	})
	c.Assert(err, tc.ErrorIsNil)

	next := now.Add(2 * time.Hour).Round(time.Second).UTC()
	update := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ExpireTime:  ptr(next),
	}
	_, err = s.store.UpdateSecret(uri, update)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:      md.URI,
		Revision: 1,
	}, watcher.SecretTriggerChange{
		URI:             md.URI,
		Revision:        1,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
}

func (s *SecretsExpiryWatcherSuite) TestWatchRemoveSecret(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	_, err := s.store.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:      uri,
		Revision: 1,
	})
	wc.AssertNoChange()

	uri2 := secrets.NewURI()
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ExpireTime:  ptr(next),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri2,
		Revision:        1,
		NextTriggerTime: next,
	})
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri2)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:      uri2,
		Revision: 1,
	})
	wc.AssertNoChange()
}

func (s *SecretsExpiryWatcherSuite) TestWatchRemoveRevision(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	triggerTime := now.Add(time.Minute).Round(time.Second).UTC()
	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		Revision:        1,
		NextTriggerTime: triggerTime,
	})
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri, 1)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:      uri,
		Revision: 1,
	})
	wc.AssertNoChange()
}

func (s *SecretsExpiryWatcherSuite) TestWatchRestartChangeOwners(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next1 := now.Add(time.Minute).Round(time.Second).UTC()
	next2 := now.Add(time.Minute).Round(time.Second).UTC()

	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ExpireTime:  ptr(next2),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	next3 := now.Add(time.Minute).Round(time.Second).UTC()

	anotherUnit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.ownerApp})
	uri3 := secrets.NewURI()
	cp = state.CreateSecretParams{
		Version: 1,
		Owner:   anotherUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ExpireTime:  ptr(next3),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri3, cp)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri2,
		Revision:        1,
		NextTriggerTime: next2,
	})

	wc.AssertNoChange()
	testing.AssertStop(c, w)

	w, err = s.State.WatchSecretRevisionsExpiryChanges(
		[]names.Tag{s.ownerApp.Tag(), anotherUnit.Tag()})
	c.Assert(err, tc.ErrorIsNil)

	wc = testing.NewSecretsTriggerWatcherC(c, w)
	defer testing.AssertStop(c, w)

	wc.AssertChange(watcher.SecretTriggerChange{
		URI:             uri,
		Revision:        1,
		NextTriggerTime: next1,
	}, watcher.SecretTriggerChange{
		URI:             uri3,
		Revision:        1,
		NextTriggerTime: next3,
	})
	wc.AssertNoChange()
}

type SecretsConsumedWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	owner *state.Application
}

func TestSecretsConsumedWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsConsumedWatcherSuite{})
}

func (s *SecretsConsumedWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
}

func (s *SecretsConsumedWatcherSuite) TestWatcherInitialEvent(c *tc.C) {
	w, err := s.State.WatchConsumedSecretsChanges(names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()

	testing.AssertStop(c, w)
}

func (s *SecretsConsumedWatcherSuite) setupWatcher(c *tc.C) (state.StringsWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := secrets.NewURI()
	cp = state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.UpdateSecret(uri2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SaveSecretConsumer(uri, names.NewUnitTag("mariadb/0"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
		LatestRevision:  1,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SaveSecretConsumer(uri2, names.NewUnitTag("mariadb/0"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
		LatestRevision:  2,
	})
	c.Assert(err, tc.ErrorIsNil)

	w, err := s.State.WatchConsumedSecretsChanges(names.NewUnitTag("mariadb/0"))
	c.Assert(err, tc.ErrorIsNil)
	wc := testing.NewStringsWatcherC(c, w)

	// No event until rev > 1, so just the one change.
	wc.AssertChange(uri2.String())
	return w, uri
}

func (s *SecretsConsumedWatcherSuite) TestWatcherStartStop(c *tc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsConsumedWatcherSuite) TestWatchSingleUpdate(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}

func (s *SecretsConsumedWatcherSuite) TestWatchMultipleSecrets(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo2": "bar"},
			Checksum:    "deadbeef",
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SaveSecretConsumer(uri2, names.NewUnitTag("mariadb/0"), &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, tc.ErrorIsNil)
	// No event until rev > 1.
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo2": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri2.String())
	wc.AssertNoChange()
}

func (s *SecretsConsumedWatcherSuite) TestWatchConsumedDeleted(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("baz"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}

type SecretsRemoteConsumerWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	owner *state.Application
}

func TestSecretsRemoteConsumerWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsRemoteConsumerWatcherSuite{})
}

func (s *SecretsRemoteConsumerWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.owner = s.Factory.MakeApplication(c, nil)
}

func (s *SecretsRemoteConsumerWatcherSuite) TestWatcherInitialEvent(c *tc.C) {
	w, err := s.State.WatchRemoteConsumedSecretsChanges("remote-app")
	c.Assert(err, tc.ErrorIsNil)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()

	testing.AssertStop(c, w)
}

func (s *SecretsRemoteConsumerWatcherSuite) setupWatcher(c *tc.C) (state.StringsWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	uri2 := secrets.NewURI()
	cp = state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.UpdateSecret(uri2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SaveSecretRemoteConsumer(uri, names.NewUnitTag("remote-app/0"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
		LatestRevision:  1,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SaveSecretRemoteConsumer(uri2, names.NewUnitTag("remote-app/0"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
		LatestRevision:  2,
	})
	c.Assert(err, tc.ErrorIsNil)

	w, err := s.State.WatchRemoteConsumedSecretsChanges("remote-app")
	c.Assert(err, tc.ErrorIsNil)
	wc := testing.NewStringsWatcherC(c, w)

	// No event until rev > 1, so just the one change.
	wc.AssertChange(uri2.String())
	return w, uri
}

func (s *SecretsRemoteConsumerWatcherSuite) TestWatcherStartStop(c *tc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretsRemoteConsumerWatcherSuite) TestWatchSingleUpdate(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	_, err := s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}

func (s *SecretsRemoteConsumerWatcherSuite) TestWatchMultipleSecrets(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo2": "bar"},
			Checksum:    "deadbeef",
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SaveSecretRemoteConsumer(uri2, names.NewUnitTag("remote-app/0"), &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, tc.ErrorIsNil)
	// No event until rev > 1.
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
		Checksum:    "deadbeef2",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri.String())
	wc.AssertNoChange()

	_, err = s.store.UpdateSecret(uri2, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo2": "bar2"},
		Checksum:    "deadbeef3",
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(uri2.String())
	wc.AssertNoChange()
}

func (s *SecretsRemoteConsumerWatcherSuite) TestWatchConsumedDeleted(c *tc.C) {
	w, uri := s.setupWatcher(c)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.State.SaveSecretRemoteConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	err = s.State.SaveSecretRemoteConsumer(uri, names.NewApplicationTag("baz"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}

type SecretsObsoleteWatcherSuite struct {
	testing.StateSuite
	store state.SecretsStore

	ownerApp  *state.Application
	ownerUnit *state.Unit
}

func TestSecretsObsoleteWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretsObsoleteWatcherSuite{})
}

func (s *SecretsObsoleteWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.store = state.NewSecrets(s.State)
	s.ownerApp = s.Factory.MakeApplication(c, nil)
	s.ownerUnit = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.ownerApp})
}

func (s *SecretsObsoleteWatcherSuite) setupWatcher(c *tc.C, forAutoPrune bool) (state.StringsWatcher, *secrets.URI) {
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerApp.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	if forAutoPrune {
		cp.Owner = names.NewModelTag(s.State.ModelUUID())
	}
	_, err := s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	var w state.StringsWatcher
	if forAutoPrune {
		w, err = s.store.WatchRevisionsToPrune(
			[]names.Tag{names.NewModelTag(s.State.ModelUUID())},
		)
	} else {
		w, err = s.store.WatchObsolete(
			[]names.Tag{s.ownerApp.Tag(), s.ownerUnit.Tag()},
		)
	}
	c.Assert(err, tc.ErrorIsNil)

	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()
	return w, uri
}

func (s *SecretsObsoleteWatcherSuite) TestWatcherStartStop(c *tc.C) {
	w, _ := s.setupWatcher(c, false)
	testing.AssertStop(c, w)
}

func (s *SecretsObsoleteWatcherSuite) TestWatchObsoleteRevisions(c *tc.C) {
	w, uri := s.setupWatcher(c, false)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	p := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar2"},
		Checksum:    "deadbeef",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo2"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 2,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// The previous consumer of rev 1 now uses rev 2; rev 1 is orphaned.
	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 2,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String() + "/1")
	wc.AssertNoChange()

	// The latest added revision is never obsolete.
	p = state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar3"},
		Checksum:    "deadbeef2",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String() + "/1")
	wc.AssertNoChange()

	// New revision 4 added, so rev 3 is now also obsolete.
	p = state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar4"},
		Checksum:    "deadbeef3",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String()+"/1", uri.String()+"/3")
	wc.AssertNoChange()
}

func (s *SecretsObsoleteWatcherSuite) TestWatchObsoleteRevisionsToPrune(c *tc.C) {
	w, uri := s.setupWatcher(c, true)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	p := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar2"},
		Checksum:    "deadbeef",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	// No change because AutoPrune is not turned on.
	wc.AssertNoChange()

	// The previous consumer of rev 1 now uses rev 2; rev 1 is orphaned.
	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 2,
	})
	c.Assert(err, tc.ErrorIsNil)
	// No change because AutoPrune is not turned on.
	wc.AssertNoChange()

	// turn on AutoPrune.
	p = state.UpdateSecretParams{
		AutoPrune:   ptr(true),
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar3"},
		Checksum:    "deadbeef2",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String() + "/1")
	wc.AssertNoChange()

	// New revision 4 added, so rev 3 is now also obsolete.
	p = state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar4"},
		Checksum:    "deadbeef3",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String()+"/1", uri.String()+"/3")
	wc.AssertNoChange()

	// The previous consumer of rev 1 now uses rev 2; rev 1 is orphaned.
	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 4,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String()+"/1", uri.String()+"/2", uri.String()+"/3")
	wc.AssertNoChange()
}

func (s *SecretsObsoleteWatcherSuite) TestWatchOwnedDeleted(c *tc.C) {
	w, uri := s.setupWatcher(c, false)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	owner2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	uri2 := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner2.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err := s.store.CreateSecret(uri2, cp)
	c.Assert(err, tc.ErrorIsNil)

	uri3 := secrets.NewURI()
	cp = state.CreateSecretParams{
		Version: 1,
		Owner:   s.ownerUnit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	_, err = s.store.CreateSecret(uri3, cp)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.store.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String())
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri2)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	_, err = s.store.DeleteSecret(uri3)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri3.String())
	wc.AssertNoChange()
}

func (s *SecretsObsoleteWatcherSuite) TestWatchDeletedSupercedesObsolete(c *tc.C) {
	w, uri := s.setupWatcher(c, false)
	wc := testing.NewStringsWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	p := state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        map[string]string{"foo": "bar2"},
		Checksum:    "deadbeef2",
	}
	_, err = s.store.UpdateSecret(uri, p)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo2"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 2,
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// The previous consumer of rev 1 now uses rev 2; rev 1 is orphaned.
	err = s.State.SaveSecretConsumer(uri, names.NewApplicationTag("foo"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 2,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Deleting the secret removes any pending orphaned changes.
	_, err = s.store.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(uri.String())
	wc.AssertNoChange()
}
