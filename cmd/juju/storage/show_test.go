// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ShowSuite struct {
	SubStorageSuite
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{}
	s.PatchValue(storage.GetStorageShowAPI, func(c *storage.ShowCommand) (storage.StorageShowAPI, error) {
		return s.mockAPI, nil
	})

}

func runShow(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.ShowCommand{}), args...)
}

func (s *ShowSuite) TestShow(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0"},
		// Default format is yaml
		`
shared-fs/0:
  storage: shared-fs
  owner: postgresql/0
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
`[1:],
	)
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "--format", "json"},
		`{"shared-fs/0":{"storage":"shared-fs","owner":"postgresql/0","location":"witty","available-size":30,"total-size":100,"tags":["tests","well","maybe"]}}
`,
	)
}

func (s *ShowSuite) TestShowMultipleReturn(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "db-dir/1000"},
		`
db-dir/1000:
  storage: db-dir
  owner: postgresql/0
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
shared-fs/0:
  storage: shared-fs
  owner: postgresql/0
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
`[1:],
	)
}

func (s *ShowSuite) assertValidShow(c *gc.C, args []string, expected string) {
	context, err := runShow(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockShowAPI struct {
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) Show(tags []names.StorageTag) ([]params.StorageInstance, error) {
	results := make([]params.StorageInstance, len(tags))

	location := "witty"
	availableSize := uint64(30)
	totalSize := uint64(100)

	for i, tag := range tags {
		results[i] = params.StorageInstance{
			StorageTag:    tag.String(),
			OwnerTag:      "unit-postgresql-0",
			Location:      &location,
			AvailableSize: &availableSize,
			TotalSize:     &totalSize,
			Tags:          []string{"tests", "well", "maybe"},
		}
	}
	return results, nil
}
