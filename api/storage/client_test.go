// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/set"
)

type storageMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageMockSuite{})

func (s *storageMockSuite) TestShow(c *gc.C) {
	var called bool

	one := "shared-fs/0"
	oneTag := names.NewStorageTag(one)
	two := "db-dir/1000"
	twoTag := names.NewStorageTag(two)
	expected := set.NewStrings(oneTag.String(), twoTag.String())

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Show")

			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Entities, gc.HasLen, 2)

			if results, k := result.(*params.StorageShowResults); k {
				instances := make([]params.StorageShowResult, len(args.Entities))
				for i, entity := range args.Entities {
					c.Assert(expected.Contains(entity.Tag), jc.IsTrue)
					instances[i] = params.StorageShowResult{
						Result: params.StorageInstance{StorageTag: entity.Tag},
					}
				}
				results.Results = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	tags := []names.StorageTag{oneTag, twoTag}
	found, err := storageClient.Show(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 2)
	c.Assert(expected.Contains(found[0].StorageTag), jc.IsTrue)
	c.Assert(expected.Contains(found[1].StorageTag), jc.IsTrue)
	c.Assert(called, jc.IsTrue)
}

func (s *storageMockSuite) TestListPools(c *gc.C) {
	var called bool
	want := 3

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Storage")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListPools")

			args, ok := a.(params.StoragePoolFilter)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Names, gc.HasLen, 2)
			c.Assert(args.Types, gc.HasLen, 1)

			if results, k := result.(*params.StoragePoolsResult); k {
				instances := make([]params.StoragePool, want)
				for i := 0; i < want; i++ {
					instances[i] = params.StoragePool{
						Name: fmt.Sprintf("name%v", i),
						Type: fmt.Sprintf("type%v", i),
					}
				}
				results.Pools = instances
			}

			return nil
		})
	storageClient := storage.NewClient(apiCaller)
	names := []string{"a", "b"}
	types := []string{"1"}
	found, err := storageClient.ListPools(types, names)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, want)
	c.Assert(called, jc.IsTrue)
}
