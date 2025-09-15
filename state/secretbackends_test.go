// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type SecretBackendsSuite struct {
	testing.StateSuite
	storage state.SecretBackendsStorage
	store   state.SecretsStore
}

func TestSecretBackendsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretBackendsSuite{})
}

func (s *SecretBackendsSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.storage = state.NewSecretBackends(s.State)
	s.store = state.NewSecrets(s.State)
}

func (s *SecretBackendsSuite) TestCreate(c *tc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(p)
	c.Assert(id, tc.Not(tc.Equals), "")
	c.Assert(err, tc.ErrorIsNil)
	backend, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backend.ID, tc.NotNil)
	backend.ID = ""
	c.Assert(backend, tc.DeepEquals, &secrets.SecretBackend{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	})
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(name, tc.Equals, "myvault")
	c.Assert(nextTime, tc.Equals, next)

	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)

	p.Name = "another"
	p.ID = id
	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretBackendsSuite) TestGetNotFound(c *tc.C) {
	_, err := s.storage.GetSecretBackend("myvault")
	c.Check(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SecretBackendsSuite) TestList(c *tc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	config := map[string]interface{}{"foo.key": "bar"}
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	p2 := state.CreateSecretBackendParams{
		Name:        "myk8s",
		BackendType: "kubernetes",
		Config:      config,
	}
	_, err = s.storage.CreateSecretBackend(p2)
	c.Assert(err, tc.ErrorIsNil)
	backends, err := s.storage.ListSecretBackends()
	c.Assert(err, tc.ErrorIsNil)
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.ID`, tc.NotNil)
	c.Assert(backends, mc, []*secrets.SecretBackend{{
		Name:        "myk8s",
		BackendType: "kubernetes",
		Config:      config,
	}, {
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}})
}

func (s *SecretBackendsSuite) TestRemove(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, tc.Satisfies, errors.IsNotFound)
	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsFails(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	sp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secrets := state.NewSecrets(s.State)
	_, err = secrets.CreateSecret(uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, tc.Satisfies, errors.IsNotSupported)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 1)
}

func (s *SecretBackendsSuite) TestRemoveWithRevisionsForce(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	sp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secrets := state.NewSecrets(s.State)
	_, err = secrets.CreateSecret(uri, sp)
	c.Assert(err, tc.ErrorIsNil)

	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 1)

	err = s.storage.DeleteSecretBackend("myvault", true)
	c.Assert(err, tc.ErrorIsNil)
	_, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.storage.GetSecretBackend("myvault")
	c.Check(err, tc.Satisfies, errors.IsNotFound)
}

func (s *SecretBackendsSuite) TestDeleteSecretUpdatesRefCount(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secretStore := state.NewSecrets(s.State)
	_, err = secretStore.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = secretStore.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  b.ID,
			RevisionID: "rev-id2",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 2)

	_, err = secretStore.DeleteSecret(uri)
	c.Assert(err, tc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 0)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestDeleteRevisionsUpdatesRefCount(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  b.ID,
				RevisionID: "rev-id",
			},
		},
	}
	secretStore := state.NewSecrets(s.State)
	_, err = secretStore.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)
	_, err = secretStore.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		ValueRef: &secrets.ValueRef{
			BackendID:  b.ID,
			RevisionID: "rev-id2",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	count, err := state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 2)

	_, err = secretStore.DeleteSecret(uri, 1)
	c.Assert(err, tc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 1)

	_, err = secretStore.DeleteSecret(uri, 2)
	c.Assert(err, tc.ErrorIsNil)

	count, err = state.SecretBackendRefCount(s.State, b.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 0)

	err = s.storage.DeleteSecretBackend("myvault", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SecretBackendsSuite) TestUpdate(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	u := state.UpdateSecretBackendParams{
		ID:                  b.ID,
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, tc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.DeepEquals, &secrets.SecretBackend{
		ID:                  b.ID,
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	})
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, b.ID)
	c.Assert(name, tc.Equals, "myvault")
	c.Assert(nextTime, tc.Equals, next)
}

func (s *SecretBackendsSuite) TestUpdateName(c *tc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, tc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.DeepEquals, &secrets.SecretBackend{
		ID:                  b.ID,
		Name:                "myvault2",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	})
	name, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, b.ID)
	c.Assert(name, tc.Equals, "myvault2")
	c.Assert(nextTime, tc.Equals, next)
}

func (s *SecretBackendsSuite) TestUpdateNameForInUseBackend(c *tc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef:    &secrets.ValueRef{BackendID: b.ID},
		},
	}
	_, err = s.store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, tc.ErrorMatches, `cannot rename a secret backend that is in use`)
}

func (s *SecretBackendsSuite) TestUpdateNameDuplicate(c *tc.C) {
	p := state.CreateSecretBackendParams{
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	p.Name = "myvault2"
	_, err = s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)

	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:         b.ID,
		NameChange: ptr("myvault2"),
		Config:     map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
}

func (s *SecretBackendsSuite) TestUpdateResetRotationInterval(c *tc.C) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	p := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	_, err := s.storage.CreateSecretBackend(p)
	c.Assert(err, tc.ErrorIsNil)
	b, err := s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)

	u := state.UpdateSecretBackendParams{
		ID:                  b.ID,
		TokenRotateInterval: ptr(0 * time.Second),
		Config:              map[string]interface{}{"foo": "bar2"},
	}
	err = s.storage.UpdateSecretBackend(u)
	c.Assert(err, tc.ErrorIsNil)
	b, err = s.storage.GetSecretBackend("myvault")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.DeepEquals, &secrets.SecretBackend{
		ID:          b.ID,
		Name:        "myvault",
		BackendType: "vault",
		Config:      map[string]interface{}{"foo": "bar2"},
	})
}

func (s *SecretBackendsSuite) TestSecretBackendRotated(c *tc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, tc.ErrorIsNil)
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.storage.SecretBackendRotated(id, next2)
	c.Assert(err, tc.ErrorIsNil)

	_, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(nextTime, tc.Equals, next2)
}

func (s *SecretBackendsSuite) TestSecretBackendRotatedConcurrent(c *tc.C) {
	config := map[string]interface{}{"foo.key": "bar"}
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              config,
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, tc.ErrorIsNil)

	later := now.Add(time.Hour).Round(time.Second).UTC()
	later2 := now.Add(2 * time.Hour).Round(time.Second).UTC()
	state.SetBeforeHooks(c, s.State, func() {
		err := s.storage.SecretBackendRotated(id, later)
		c.Assert(err, tc.ErrorIsNil)
	})

	err = s.storage.SecretBackendRotated(id, later2)
	c.Assert(err, tc.ErrorIsNil)

	_, nextTime := state.GetSecretBackendNextRotateInfo(c, s.State, id)
	c.Assert(nextTime, tc.Equals, later)
}

type SecretBackendWatcherSuite struct {
	testing.StateSuite
	storage state.SecretBackendsStorage
}

func TestSecretBackendWatcherSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SecretBackendWatcherSuite{})
}

func (s *SecretBackendWatcherSuite) SetUpTest(c *tc.C) {
	s.StateSuite.SetUpTest(c)
	s.storage = state.NewSecretBackends(s.State)
}

func (s *SecretBackendWatcherSuite) setupWatcher(c *tc.C) (state.SecretBackendRotateWatcher, string) {
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	cp := state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next),
		Config:              map[string]interface{}{"foo.key": "bar"},
	}
	id, err := s.storage.CreateSecretBackend(cp)
	c.Assert(err, tc.ErrorIsNil)
	w, err := s.State.WatchSecretBackendRotationChanges()
	c.Assert(err, tc.ErrorIsNil)

	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
	return w, id
}

func (s *SecretBackendWatcherSuite) TestWatchInitialEvent(c *tc.C) {
	w, _ := s.setupWatcher(c)
	testing.AssertStop(c, w)
}

func (s *SecretBackendWatcherSuite) TestWatchSingleUpdate(c *tc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer testing.AssertStop(c, w)

	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(2 * time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchDelete(c *tc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer testing.AssertStop(c, w)

	err := s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(0 * time.Second),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdatesSameBackend(c *tc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Minute).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	next2 := now.Add(time.Hour).Round(time.Second).UTC()
	err = s.storage.SecretBackendRotated(id, next2)
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next2,
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdatesSameBackendDeleted(c *tc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})
	err = s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(time.Duration(0)),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}

func (s *SecretBackendWatcherSuite) TestWatchMultipleUpdates(c *tc.C) {
	w, id := s.setupWatcher(c)
	wc := testing.NewSecretBackendRotateWatcherC(c, w)
	defer testing.AssertStop(c, w)

	// TODO(quiescence): these two changes should be one event.
	now := s.Clock.Now().Round(time.Second).UTC()
	next := now.Add(time.Hour).Round(time.Second).UTC()
	err := s.storage.SecretBackendRotated(id, next)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id,
		Name:            "myvault",
		NextTriggerTime: next,
	})

	next2 := now.Add(time.Minute).Round(time.Second).UTC()
	id2, err := s.storage.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault2",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		NextRotateTime:      ptr(next2),
		Config:              map[string]interface{}{"foo.key": "bar"},
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:              id2,
		Name:            "myvault2",
		NextTriggerTime: next2,
	})

	err = s.storage.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  id,
		TokenRotateInterval: ptr(time.Duration(0)),
	})
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange(watcher.SecretBackendRotateChange{
		ID:   id,
		Name: "myvault",
	})
	wc.AssertNoChange()
}
