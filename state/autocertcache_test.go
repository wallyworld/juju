// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/context"

	"github.com/juju/juju/internal/testing"
	statetesting "github.com/juju/juju/state/testing"
)

type autocertCacheSuite struct {
	statetesting.StateSuite
}

func TestAutocertCacheSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &autocertCacheSuite{})
}

func (s *autocertCacheSuite) TestCachePutGet(c *tc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, tc.ErrorIsNil)
	err = cache.Put(ctx, "b", []byte("bval"))
	c.Assert(err, tc.ErrorIsNil)

	// Check that we can get the existing entries.
	data, err := cache.Get(ctx, "a")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "aval")

	data, err = cache.Get(ctx, "b")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "bval")
}

func (s *autocertCacheSuite) TestGetNonexistentEntry(c *tc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	// Getting a non-existent entry must return ErrCacheMiss.
	data, err := cache.Get(ctx, "c")
	c.Assert(err, tc.Equals, autocert.ErrCacheMiss)
	c.Assert(data, tc.IsNil)
}

func (s *autocertCacheSuite) TestDelete(c *tc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, tc.ErrorIsNil)
	err = cache.Put(ctx, "b", []byte("bval"))
	c.Assert(err, tc.ErrorIsNil)

	// Check that we can delete an entry.
	err = cache.Delete(ctx, "b")
	c.Assert(err, tc.ErrorIsNil)

	data, err := cache.Get(ctx, "b")
	c.Assert(err, tc.Equals, autocert.ErrCacheMiss)
	c.Assert(data, tc.IsNil)

	// Check that the non-deleted entry is still there.
	data, err = cache.Get(ctx, "a")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "aval")
}

func (s *autocertCacheSuite) TestDeleteNonexistentEntry(c *tc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Delete(ctx, "a")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *autocertCacheSuite) TestPutExistingEntry(c *tc.C) {
	ctx := context.Background()
	cache := s.State.AutocertCache()

	err := cache.Put(ctx, "a", []byte("aval"))
	c.Assert(err, tc.ErrorIsNil)

	err = cache.Put(ctx, "a", []byte("aval2"))
	c.Assert(err, tc.ErrorIsNil)

	data, err := cache.Get(ctx, "a")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "aval2")
}
