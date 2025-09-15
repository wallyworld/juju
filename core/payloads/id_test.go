// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
)

func TestIdSuite(t *tctesting.T) {
	tc.Run(t, &idSuite{})
}

type idSuite struct {
	testhelpers.IsolationSuite
}

func (s *idSuite) TestParseIDFull(c *tc.C) {
	name, id := payloads.ParseID("a-payload/my-payload")

	c.Check(name, tc.Equals, "a-payload")
	c.Check(id, tc.Equals, "my-payload")
}

func (s *idSuite) TestParseIDNameOnly(c *tc.C) {
	name, id := payloads.ParseID("a-payload")

	c.Check(name, tc.Equals, "a-payload")
	c.Check(id, tc.Equals, "")
}

func (s *idSuite) TestParseIDExtras(c *tc.C) {
	name, id := payloads.ParseID("somecharm/0/a-payload/my-payload")

	c.Check(name, tc.Equals, "somecharm")
	c.Check(id, tc.Equals, "0/a-payload/my-payload")
}
