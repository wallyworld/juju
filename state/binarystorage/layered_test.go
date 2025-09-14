// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage_test

import (
	"io"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state/binarystorage"
)

type layeredStorageSuite struct {
	coretesting.BaseSuite
	stores []*mockStorage
	store  binarystorage.Storage
}

func TestLayeredStorageSuite(t *tctesting.T) {
	tc.Run(t, &layeredStorageSuite{})
}

func (s *layeredStorageSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stores = []*mockStorage{{
		metadata: []binarystorage.Metadata{{
			Version: "1.0", Size: 1, SHA256: "foo",
		}, {
			Version: "2.0", Size: 2, SHA256: "bar",
		}},
	}, {
		metadata: []binarystorage.Metadata{{
			Version: "3.0", Size: 3, SHA256: "baz",
		}, {
			Version: "1.0", Size: 3, SHA256: "meh",
		}},
	}}

	stores := make([]binarystorage.Storage, len(s.stores))
	for i, store := range s.stores {
		stores[i] = store
	}
	var err error
	s.store, err = binarystorage.NewLayeredStorage(stores...)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *layeredStorageSuite) TestNewLayeredStorageError(c *tc.C) {
	_, err := binarystorage.NewLayeredStorage(s.stores[0])
	c.Assert(err, tc.ErrorMatches, "expected multiple stores")
}

func (s *layeredStorageSuite) TestAdd(c *tc.C) {
	r := new(readCloser)
	m := binarystorage.Metadata{Version: "4.0", Size: 4, SHA256: "qux"}
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	err := s.store.Add(r, m)
	c.Assert(err, tc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{"Add", []interface{}{r, m}}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestAllMetadata(c *tc.C) {
	all, err := s.store.AllMetadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all, tc.DeepEquals, []binarystorage.Metadata{
		{Version: "1.0", Size: 1, SHA256: "foo"},
		{Version: "2.0", Size: 2, SHA256: "bar"},
		{Version: "3.0", Size: 3, SHA256: "baz"},
	})
	s.stores[0].CheckCallNames(c, "AllMetadata")
	s.stores[1].CheckCallNames(c, "AllMetadata")
}

func (s *layeredStorageSuite) TestAllMetadataError(c *tc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, err := s.store.AllMetadata()
	c.Assert(err, tc.Equals, expectedErr)
	s.stores[0].CheckCallNames(c, "AllMetadata")
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestMetadata(c *tc.C) {
	s.stores[0].SetErrors(errors.NotFoundf("metadata"))
	m, err := s.store.Metadata("3.0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, s.stores[1].metadata[0])
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Metadata", []interface{}{"3.0"},
	}})
	s.stores[1].CheckCalls(c, []testhelpers.StubCall{{
		"Metadata", []interface{}{"3.0"},
	}})
}

func (s *layeredStorageSuite) TestMetadataEarlyExit(c *tc.C) {
	m, err := s.store.Metadata("1.0")
	c.Assert(err, tc.ErrorIsNil)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Metadata", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
	c.Assert(m, tc.DeepEquals, s.stores[0].metadata[0])
}

func (s *layeredStorageSuite) TestMetadataFatalError(c *tc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, err := s.store.Metadata("1.0")
	c.Assert(err, tc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Metadata", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestOpen(c *tc.C) {
	s.stores[0].SetErrors(errors.NotFoundf("metadata"))
	m, rc, err := s.store.Open("3.0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, s.stores[1].metadata[0])
	c.Assert(rc, tc.Equals, &s.stores[1].rc)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Open", []interface{}{"3.0"},
	}})
	s.stores[1].CheckCalls(c, []testhelpers.StubCall{{
		"Open", []interface{}{"3.0"},
	}})
}

func (s *layeredStorageSuite) TestOpenEarlyExit(c *tc.C) {
	m, rc, err := s.store.Open("1.0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, s.stores[0].metadata[0])
	c.Assert(rc, tc.Equals, &s.stores[0].rc)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Open", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestOpenFatalError(c *tc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, _, err := s.store.Open("1.0")
	c.Assert(err, tc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testhelpers.StubCall{{
		"Open", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

type mockStorage struct {
	testhelpers.Stub
	rc       readCloser
	metadata []binarystorage.Metadata
}

func (s *mockStorage) Add(r io.Reader, m binarystorage.Metadata) error {
	s.MethodCall(s, "Add", r, m)
	return s.NextErr()
}

func (s *mockStorage) AllMetadata() ([]binarystorage.Metadata, error) {
	s.MethodCall(s, "AllMetadata")
	return s.metadata, s.NextErr()
}

func (s *mockStorage) Metadata(version string) (binarystorage.Metadata, error) {
	s.MethodCall(s, "Metadata", version)
	return s.metadata[0], s.NextErr()
}

func (s *mockStorage) Open(version string) (binarystorage.Metadata, io.ReadCloser, error) {
	s.MethodCall(s, "Open", version)
	return s.metadata[0], &s.rc, s.NextErr()
}

type readCloser struct{ io.ReadCloser }
