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
OWNER        ID          SIZE      LOCATION  
postgresql/0 db-dir/1000 1.0GiB    /srv/data 
transcode/0  shared-fs/0 (unknown) /srv      

`[1:],
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
postgresql/0:
  db-dir/1000:
    storage: db-dir
    location: /srv/data
    available-size: 1
    total-size: 1024
    tags:
    - tests
    - well
    - maybe
transcode/0:
  shared-fs/0:
    storage: shared-fs
    location: /srv
`[1:],
	)
}

func (s *ListSuite) TestListOwnerStorageIdSort(c *gc.C) {
	s.mockAPI.lexicalChaos = true
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
[Storage]    
OWNER        ID          SIZE      LOCATION  
postgresql/0 db-dir/1000 1.0GiB    /srv/data 
transcode    db-dir/1000 (unknown) /srv      
transcode/0  db-dir/1000 (unknown) /srv      
transcode/0  shared-fs/0 (unknown) /srv      
transcode/0  shared-fs/5 (unknown) /srv      

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
	lexicalChaos bool
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

	if s.lexicalChaos {
		last := params.StorageInstance{
			StorageTag: "storage-shared-fs-5",
			OwnerTag:   "unit-transcode-0",
			Location:   &tcLocation,
		}
		second := params.StorageInstance{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "unit-transcode-0",
			Location:   &tcLocation,
		}
		first := params.StorageInstance{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "service-transcode",
			Location:   &tcLocation,
		}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results, nil
}
