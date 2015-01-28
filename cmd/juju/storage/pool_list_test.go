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

type PoolListSuite struct {
	SubStorageSuite
	mockAPI *mockPoolListAPI
}

var _ = gc.Suite(&PoolListSuite{})

func (s *PoolListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolListAPI{}
	s.PatchValue(storage.GetPoolListAPI, func(c *storage.PoolListCommand) (storage.PoolListAPI, error) {
		return s.mockAPI, nil
	})

}

func runPoolList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.PoolListCommand{}), args...)
}

func (s *PoolListSuite) TestPoolList(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--type", "a", "--type", "b", "--name", "xyz", "--name", "abc"},
		// Default format is yaml
		`- name: testName
  type: a
  characteristics:
    one: true
    three: maybe
    two: well
- name: testName
  type: b
  characteristics:
    one: true
    three: maybe
    two: well
- name: xyz
  type: testType
  characteristics:
    one: true
    three: maybe
    two: well
- name: abc
  type: testType
  characteristics:
    one: true
    three: maybe
    two: well
`,
	)
}

func (s *PoolListSuite) TestPoolListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--type", "a", "--type", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "json"},
		`[`+
			`{"name":"testName","type":"a",`+
			`"characteristics":{"one":true,"three":"maybe","two":"well"}},`+
			`{"name":"testName","type":"b",`+
			`"characteristics":{"one":true,"three":"maybe","two":"well"}},`+
			`{"name":"xyz","type":"testType",`+
			`"characteristics":{"one":true,"three":"maybe","two":"well"}},`+
			`{"name":"abc","type":"testType",`+
			`"characteristics":{"one":true,"three":"maybe","two":"well"}}`+
			"]\n",
	)
}

func (s *PoolListSuite) TestPoolListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--type", "a", "--type", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		"TYPE      NAME      CHARACTERISTICS\n"+
			"a         testName  one=true,two=well,three=maybe\n"+
			"b         testName  one=true,two=well,three=maybe\n"+
			"testType  xyz       one=true,two=well,three=maybe\n"+
			"testType  abc       one=true,two=well,three=maybe\n"+
			"\n",
	)
}

func (s *PoolListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runPoolList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockPoolListAPI struct {
}

func (s mockPoolListAPI) Close() error {
	return nil
}

func (s mockPoolListAPI) PoolList(types []string, names []string) ([]params.PoolInstance, error) {
	results := make([]params.PoolInstance, len(types)+len(names))
	var index int
	addInstance := func(aname, atype string) {
		results[index] = createTestPoolInstance(aname, atype)
		index++
	}
	for _, atype := range types {
		addInstance("testName", atype)
	}
	for _, aname := range names {
		addInstance(aname, "testType")
	}
	return results, nil
}

func createTestPoolInstance(aname, atype string) params.PoolInstance {
	return params.PoolInstance{
		Name:   aname,
		Type:   atype,
		Traits: map[string]interface{}{"one": true, "two": "well", "three": "maybe"},
	}
}
