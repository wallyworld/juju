// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"context"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/internal/worker/lease"
)

type storeSuite struct {
	testing.ControllerSuite

	store *lease.Store
}

func TestStoreSuite(t *tctesting.T) {
	tc.Run(t, &storeSuite{})
}

func (s *storeSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.store = lease.NewStore(lease.StoreConfig{
		TrackedDB: s.TrackedDB(),
		Logger:    lease.StubLogger{},
	})
}

func (s *storeSuite) TestClaimLeaseSuccessAndLeaseQueries(c *tc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	// Add 2 leases.
	err := s.store.ClaimLease(c.Context(), pgKey, pgReq)
	c.Assert(err, tc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(c.Context(), mmKey, mmReq)
	c.Assert(err, tc.ErrorIsNil)

	// Check all the leases.
	leases, err := s.store.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 2)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), tc.IsTrue)
	c.Check(leases[mmKey].Holder, tc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), tc.IsTrue)

	// Check with a filter.
	leases, err = s.store.Leases(c.Context(), pgKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")

	// Add a lease from a different group,
	// and check that the group returns the application leases.
	err = s.store.ClaimLease(c.Context(),
		corelease.Key{
			Namespace: "singular-controller",
			ModelUUID: "controller-model-uuid",
			Lease:     "singular",
		},
		corelease.Request{
			Holder:   "machine/0",
			Duration: time.Minute,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	leases, err = s.store.LeaseGroup(c.Context(), "application-leadership", "model-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 2)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")
	c.Check(leases[mmKey].Holder, tc.Equals, "mattermost/0")
}

func (s *storeSuite) TestClaimLeaseAlreadyHeld(c *tc.C) {
	key := corelease.Key{
		Namespace: "singular-controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.ClaimLease(c.Context(), key, req)
	c.Assert(errors.Is(err, corelease.ErrHeld), tc.IsTrue)
}

func (s *storeSuite) TestExtendLeaseSuccess(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	leases, err := s.store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)

	// Save the expiry for later comparison.
	originalExpiry := leases[key].Expiry

	req.Duration = 2 * time.Minute
	err = s.store.ExtendLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	leases, err = s.store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)

	// Check that we extended.
	c.Check(leases[key].Expiry.After(originalExpiry), tc.IsTrue)
}

func (s *storeSuite) TestExtendLeaseNotHeldInvalid(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ExtendLease(c.Context(), key, req)
	c.Assert(errors.Is(err, corelease.ErrInvalid), tc.IsTrue)
}

func (s *storeSuite) TestRevokeLeaseSuccess(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.RevokeLease(c.Context(), key, req.Holder)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storeSuite) TestRevokeLeaseNotHeldInvalid(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(c.Context(), key, "not-the-holder")
	c.Assert(errors.Is(err, corelease.ErrInvalid), tc.IsTrue)
}

func (s *storeSuite) TestPinUnpinLeaseAndPinQueries(c *tc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), pgKey, pgReq)
	c.Assert(err, tc.ErrorIsNil)

	// One entity pins the lease.
	err = s.store.PinLease(c.Context(), pgKey, "machine/6")
	c.Assert(err, tc.ErrorIsNil)

	// The same lease/entity is a no-op without error.
	err = s.store.PinLease(c.Context(), pgKey, "machine/6")
	c.Assert(err, tc.ErrorIsNil)

	// Another entity pinning the same lease.
	err = s.store.PinLease(c.Context(), pgKey, "machine/7")
	c.Assert(err, tc.ErrorIsNil)

	pins, err := s.store.Pinned(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pins, tc.HasLen, 1)
	c.Check(pins[pgKey], tc.SameContents, []string{"machine/6", "machine/7"})

	// Unpin and check the leases.
	err = s.store.UnpinLease(c.Context(), pgKey, "machine/7")
	c.Assert(err, tc.ErrorIsNil)

	pins, err = s.store.Pinned(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pins, tc.HasLen, 1)
	c.Check(pins[pgKey], tc.SameContents, []string{"machine/6"})
}

func (s *storeSuite) TestLeaseOperationCancellation(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(ctx, key, req)
	c.Assert(err, tc.ErrorMatches, "context canceled")
}
