// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	SubStorageSuite
	mockAPI *mockListAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockListAPI{}
	s.PatchValue(storage.GetStorageListAPI, func(c *storage.ListCommand) (storage.StorageListAPI, error) {
		return s.mockAPI, nil
	})

}

func runList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.ListCommand{}), args...)
}

func (s *ListSuite) TestList(c *gc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
[Storage]   
ID          OWNER        SIZE      LOCATION  
db-dir/1000 postgresql/0 1.0GiB    /srv/data 
shared-fs/0 transcode/0  (unknown) /srv      

`[1:],
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
db-dir/1000:
  storage: db-dir
  owner: postgresql/0
  location: /srv/data
  available-size: 1
  total-size: 1024
  tags:
  - tests
  - well
  - maybe
shared-fs/0:
  storage: shared-fs
  owner: transcode/0
  location: /srv
`[1:],
	)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockListAPI struct {
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) List() ([]params.StorageInstance, error) {
	tcLocation := "/srv"
	pgLocation := "/srv/data"
	pgAvailableSize := uint64(1)
	pgTotalSize := uint64(1024)

	results := []params.StorageInstance{{
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "unit-transcode-0",
		Location:   &tcLocation,
	}, {
		StorageTag:    "storage-db-dir-1000",
		OwnerTag:      "unit-postgresql-0",
		Location:      &pgLocation,
		AvailableSize: &pgAvailableSize,
		TotalSize:     &pgTotalSize,
		Tags:          []string{"tests", "well", "maybe"},
	}}

	return results, nil
}
