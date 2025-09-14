// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"strings"
	tctesting "testing"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/testhelpers"
)

type SerializationSuite struct {
	testhelpers.IsolationSuite
}

func TestSerializationSuite(t *tctesting.T) {
	tc.Run(t, &SerializationSuite{})
}

func (s *SerializationSuite) TestDeserializeFingerprintOkay(c *tc.C) {
	content := "some data\n..."
	expected, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)

	fp, err := resources.DeserializeFingerprint(expected.Bytes())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(fp, tc.DeepEquals, expected)
}

func (s *SerializationSuite) TestDeserializeFingerprintInvalid(c *tc.C) {
	_, err := resources.DeserializeFingerprint([]byte("<too short>"))

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *SerializationSuite) TestDeserializeFingerprintZeroValue(c *tc.C) {
	fp, err := resources.DeserializeFingerprint(nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(fp, tc.DeepEquals, charmresource.Fingerprint{})
}
