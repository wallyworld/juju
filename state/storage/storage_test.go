// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"io"
	"strings"
	tctesting "testing"

	"github.com/juju/blobstore/v3"
	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state/storage"
)

const testUUID = "9f484882-2f18-4fd2-967d-db9663db7bea"

type StorageSuite struct {
	mgotesting.MgoSuite
	testing.BaseSuite
	managedStorage blobstore.ManagedStorage
	storage        storage.Storage
}

func TestStorageSuite(t *tctesting.T) {
	tc.Run(t, &StorageSuite{})
}

func (s *StorageSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *StorageSuite) TearDownSuite(c *tc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *StorageSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	rs := blobstore.NewGridFS("blobstore", "blobstore", s.Session)
	db := s.Session.DB("juju")
	s.managedStorage = blobstore.NewManagedStorage(db, rs)
	s.storage = storage.NewStorage(testUUID, s.Session)
}

func (s *StorageSuite) TearDownTest(c *tc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *StorageSuite) TestStorageGet(c *tc.C) {
	err := s.managedStorage.PutForBucket(testUUID, "abc", strings.NewReader("abc"), 3)
	c.Assert(err, tc.ErrorIsNil)

	r, length, err := s.storage.Get("abc")
	c.Assert(err, tc.ErrorIsNil)
	defer r.Close()
	c.Assert(length, tc.Equals, int64(3))

	data, err := io.ReadAll(r)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "abc")
}

func (s *StorageSuite) TestStoragePut(c *tc.C) {
	err := s.storage.Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, tc.ErrorIsNil)

	r, length, err := s.managedStorage.GetForBucket(testUUID, "path")
	c.Assert(err, tc.ErrorIsNil)
	defer r.Close()

	c.Assert(length, tc.Equals, int64(3))
	data, err := io.ReadAll(r)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "abc")
}

func (s *StorageSuite) TestStorageRemove(c *tc.C) {
	err := s.storage.Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, tc.ErrorIsNil)

	err = s.storage.Remove("path")
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.storage.Get("path")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = s.storage.Remove("path")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}
