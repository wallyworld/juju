// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	. "github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/internal/testhelpers"
)

// cacheSuite is gocheck boilerplate.
type cacheSuite struct {
	testhelpers.IsolationSuite
}

// _ is gocheck boilerplate.
func TestCacheSuite(t *tctesting.T) {
	tc.Run(t, &cacheSuite{})
}

func (cacheSuite) TestSyncOnerErrors(c *tc.C) {
	o := fakeParams{FakeData: nil, Err: errors.New("oner failed")}
	u := fakeFetcher{}
	got := Sync(o, u, "", nil)
	c.Assert(got, tc.ErrorMatches, "oner failed")
}

func (cacheSuite) TestSyncOnerExists(c *tc.C) {
	o := fakeParams{
		FakeData: nil,
		Err:      errors.AlreadyExistsf("exists")}
	u := fakeFetcher{}
	got := Sync(o, u, "", nil)
	c.Assert(got, tc.ErrorIsNil)
}

func (cacheSuite) TestSyncUpdaterErrors(c *tc.C) {
	o := fakeParams{FakeData: &imagedownloads.Metadata{}, Err: nil}
	u := fakeFetcher{Err: errors.New("updater failed")}
	got := Sync(o, u, "", nil)
	c.Assert(got, tc.ErrorMatches, "updater failed")
}

func (cacheSuite) TestSyncSucceeds(c *tc.C) {
	o := fakeParams{FakeData: &imagedownloads.Metadata{}}
	u := fakeFetcher{}
	got := Sync(o, u, "", nil)
	c.Assert(got, tc.ErrorIsNil)
}

type fakeParams struct {
	FakeData *imagedownloads.Metadata
	Err      error
}

func (f fakeParams) One() (*imagedownloads.Metadata, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.FakeData, nil
}

type fakeFetcher struct {
	// Used to return an error
	Err error
}

func (f fakeFetcher) Fetch() error {
	if f.Err != nil {
		return f.Err
	}
	return nil
}

func (f fakeFetcher) Close() {
	return
}
