// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
)

type AuthKeysSuite struct {
	testhelpers.IsolationSuite
}

func TestAuthKeysSuite(t *tctesting.T) {
	tc.Run(t, &AuthKeysSuite{})
}

func (s *AuthKeysSuite) TestConcatAuthKeys(c *tc.C) {
	for _, test := range []struct{ a, b, result string }{
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a\nb"},
		{"a\n", "b", "a\nb"},
	} {
		c.Check(config.ConcatAuthKeys(test.a, test.b), tc.Equals, test.result)
	}
}
