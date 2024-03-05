// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"strings"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type objectStoreSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&objectStoreSuite{})

func (s *objectStoreSuite) TestObjectStore(c *gc.C) {
	tests := []struct {
		value string
		err   string
	}{{
		value: "file",
	}, {
		value: "s3",
	}, {
		value: "inferi",
		err:   "object store type \"inferi\" not valid",
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		backend, err := ParseObjectStoreType(test.value)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(backend, gc.Equals, BackendType(test.value))
	}
}

func (s *objectStoreSuite) TestBucketName(c *gc.C) {
	tests := []struct {
		value string
		err   string
	}{{
		value: "",
		err:   `bucket name "" not valid`,
	}, {
		value: "f",
		err:   `bucket name "f": too short`,
	}, {
		value: strings.Repeat("f", 64),
		err:   `bucket name "f{64}": too long`,
	}, {
		value: "Abcd",
		err:   `bucket name "Abcd": invalid characters`,
	}, {
		value: "ab.cd",
		err:   `bucket name "ab.cd": invalid characters`,
	}, {
		value: "10.0.0.1",
		err:   `bucket name "10.0.0.1": invalid characters`,
	}, {
		value: "-foo",
		err:   `bucket name "-foo": invalid characters`,
	}, {
		value: "foo-",
		err:   `bucket name "foo-": invalid characters`,
	}, {
		value: "xn--foo",
		err:   `bucket name "xn--foo": invalid prefix`,
	}, {
		value: "sthree-foo",
		err:   `bucket name "sthree-foo": invalid prefix`,
	}, {
		value: "sthree-configurator",
		err:   `bucket name "sthree-configurator": invalid prefix`,
	}, {
		value: "foo-s3alias",
		err:   `bucket name "foo-s3alias": invalid suffix`,
	}, {
		value: "my-bucket",
	}, {
		value: "m-f",
	}, {
		value: "juju-123",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		s, err := ParseObjectStoreBucketName(test.value)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(s, gc.Equals, test.value)
	}
}
