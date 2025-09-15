// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/storage"
)

type ConstraintsSuite struct {
	testing.BaseSuite
}

func TestConstraintsSuite(t *tctesting.T) {
	tc.Run(t, &ConstraintsSuite{})
}

func (s *ConstraintsSuite) TestParseConstraintsStoragePool(c *tc.C) {
	s.testParse(c, "pool,1M", storage.Constraints{
		Pool:  "pool",
		Count: 1,
		Size:  1,
	})
	s.testParse(c, "pool,", storage.Constraints{
		Pool:  "pool",
		Count: 1,
	})
	s.testParse(c, "1M", storage.Constraints{
		Size:  1,
		Count: 1,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountSize(c *tc.C) {
	s.testParse(c, "p,1G", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1024,
	})
	s.testParse(c, "p,1,0.5T", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1024 * 512,
	})
	s.testParse(c, "p,0.125P,3", storage.Constraints{
		Pool:  "p",
		Count: 3,
		Size:  1024 * 1024 * 128,
	})
	s.testParse(c, "3,p,0.125P", storage.Constraints{
		Pool:  "p",
		Count: 3,
		Size:  1024 * 1024 * 128,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsOptions(c *tc.C) {
	s.testParse(c, "p,1M,", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountRange(c *tc.C) {
	s.testParseError(c, "p,0,100M", `cannot parse count: count must be greater than zero, got "0"`)
	s.testParseError(c, "p,00,100M", `cannot parse count: count must be greater than zero, got "00"`)
	s.testParseError(c, "p,-1,100M", `cannot parse count: count must be greater than zero, got "-1"`)
	s.testParseError(c, "", `storage constraints require at least one field to be specified`)
	s.testParseError(c, ",", `storage constraints require at least one field to be specified`)
}

func (s *ConstraintsSuite) TestParseConstraintsSizeRange(c *tc.C) {
	s.testParseError(c, "p,-100M", `cannot parse size: expected a non-negative number, got "-100M"`)
}

func (s *ConstraintsSuite) TestParseMultiplePoolNames(c *tc.C) {
	s.testParseError(c, "pool1,anyoldjunk", `pool name is already set to "pool1", new value "anyoldjunk" not valid`)
	s.testParseError(c, "pool1,pool2", `pool name is already set to "pool1", new value "pool2" not valid`)
	s.testParseError(c, "pool1,pool2,pool3", `pool name is already set to "pool1", new value "pool2" not valid`)
}

func (s *ConstraintsSuite) TestParseMultipleCounts(c *tc.C) {
	s.testParseError(c, "pool1,10,20", `storage instance count is already set to 10, new value 20 not valid`)
}

func (s *ConstraintsSuite) TestParseMultipleStorageSize(c *tc.C) {
	s.testParseError(c, "pool1,10M,20M", `storage size is already set to 10, new value 20 not valid`)
}

func (s *ConstraintsSuite) TestParseConstraintsUnknown(c *tc.C) {
	// Regression test for #1855181
	s.testParseError(c, "p,100M database-b", `unrecognized storage constraint "100M database-b" not valid`)
	s.testParseError(c, "p,$1234", `unrecognized storage constraint "\$1234" not valid`)
}

func (*ConstraintsSuite) testParse(c *tc.C, s string, expect storage.Constraints) {
	cons, err := storage.ParseConstraints(s)
	c.Check(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, expect)
}

func (*ConstraintsSuite) testParseError(c *tc.C, s, expectErr string) {
	_, err := storage.ParseConstraints(s)
	c.Check(err, tc.ErrorMatches, expectErr)
}

func (s *ConstraintsSuite) TestValidPoolName(c *tc.C) {
	c.Assert(storage.IsValidPoolName("pool"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-ool"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-00l"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?00l"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-?00l"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("P"), tc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?0?l"), tc.IsTrue)
}

func (s *ConstraintsSuite) TestInvalidPoolName(c *tc.C) {
	c.Assert(storage.IsValidPoolName("7ool"), tc.IsFalse)
	c.Assert(storage.IsValidPoolName("/ool"), tc.IsFalse)
	c.Assert(storage.IsValidPoolName("-00l"), tc.IsFalse)
	c.Assert(storage.IsValidPoolName("*00l"), tc.IsFalse)
}

func (s *ConstraintsSuite) TestParseStorageConstraints(c *tc.C) {
	s.testParseStorageConstraints(c,
		[]string{"data=p,1M,"}, true,
		map[string]storage.Constraints{"data": {
			Pool:  "p",
			Count: 1,
			Size:  1,
		}})
	s.testParseStorageConstraints(c,
		[]string{"data"}, false,
		map[string]storage.Constraints{"data": {
			Count: 1,
		}})
	s.testParseStorageConstraints(c,
		[]string{"data=3", "cache"}, false,
		map[string]storage.Constraints{
			"data": {
				Count: 3,
			},
			"cache": {
				Count: 1,
			},
		})
}

func (s *ConstraintsSuite) TestParseStorageConstraintsErrors(c *tc.C) {
	s.testStorageConstraintsError(c,
		[]string{"data"}, true,
		`.*where "constraints" must be specified.*`)
	s.testStorageConstraintsError(c,
		[]string{"data=p,=1M,"}, false,
		`.*expected "name=constraints" or "name", got .*`)
	s.testStorageConstraintsError(c,
		[]string{"data", "data"}, false,
		`storage "data" specified more than once`)
	s.testStorageConstraintsError(c,
		[]string{"data=-1"}, false,
		`.*cannot parse constraints for storage "data".*`)
	s.testStorageConstraintsError(c,
		[]string{"data="}, false,
		`.*cannot parse constraints for storage "data".*`)
}

func (*ConstraintsSuite) testParseStorageConstraints(c *tc.C,
	s []string,
	mustHave bool,
	expect map[string]storage.Constraints,
) {
	cons, err := storage.ParseConstraintsMap(s, mustHave)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(cons), tc.Equals, len(expect))
	for k, v := range expect {
		c.Check(cons[k], tc.DeepEquals, v)
	}
}

func (*ConstraintsSuite) testStorageConstraintsError(c *tc.C, s []string, mustHave bool, e string) {
	_, err := storage.ParseConstraintsMap(s, mustHave)
	c.Check(err, tc.ErrorMatches, e)
}

func (s *ConstraintsSuite) TestToString(c *tc.C) {
	_, err := storage.ToString(storage.Constraints{})
	c.Assert(err, tc.ErrorMatches, "must provide one of pool or size or count")

	for _, t := range []struct {
		pool     string
		count    uint64
		size     uint64
		expected string
	}{
		{"loop", 0, 0, "loop"},
		{"loop", 1, 0, "loop,1"},
		{"loop", 0, 1024, "loop,1024M"},
		{"loop", 1, 1024, "loop,1,1024M"},
		{"", 0, 1024, "1024M"},
		{"", 1, 0, "1"},
		{"", 1, 1024, "1,1024M"},
	} {
		str, err := storage.ToString(storage.Constraints{
			Pool:  t.pool,
			Size:  t.size,
			Count: t.count,
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(str, tc.Equals, t.expected)

		// Test roundtrip, count defaults to 1.
		if t.count == 0 {
			t.count = 1
		}
		s.testParse(c, str, storage.Constraints{
			Pool:  t.pool,
			Size:  t.size,
			Count: t.count,
		})
	}
}
