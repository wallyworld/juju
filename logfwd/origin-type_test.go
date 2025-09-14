// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/logfwd"
)

type OriginTypeSuite struct {
	testhelpers.IsolationSuite
}

func TestOriginTypeSuite(t *tctesting.T) {
	tc.Run(t, &OriginTypeSuite{})
}

func (s *OriginTypeSuite) TestZeroValue(c *tc.C) {
	var ot logfwd.OriginType

	c.Check(ot, tc.Equals, logfwd.OriginTypeUnknown)
}

func (s *OriginTypeSuite) TestParseOriginTypeValid(c *tc.C) {
	tests := map[string]logfwd.OriginType{
		"unknown": logfwd.OriginTypeUnknown,
		"user":    logfwd.OriginTypeUser,
		"machine": logfwd.OriginTypeMachine,
		"unit":    logfwd.OriginTypeUnit,
	}
	for str, expected := range tests {
		c.Logf("trying %q", str)

		ot, err := logfwd.ParseOriginType(str)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(ot, tc.Equals, expected)
	}
}

func (s *OriginTypeSuite) TestParseOriginTypeEmpty(c *tc.C) {
	_, err := logfwd.ParseOriginType("")

	c.Check(err, tc.ErrorMatches, `unrecognized origin type ""`)
}

func (s *OriginTypeSuite) TestParseOriginTypeInvalid(c *tc.C) {
	_, err := logfwd.ParseOriginType("spam")

	c.Check(err, tc.ErrorMatches, `unrecognized origin type "spam"`)
}

func (s *OriginTypeSuite) TestString(c *tc.C) {
	tests := map[logfwd.OriginType]string{
		logfwd.OriginTypeUnknown: "unknown",
		logfwd.OriginTypeUser:    "user",
		logfwd.OriginTypeMachine: "machine",
		logfwd.OriginTypeUnit:    "unit",
	}
	for ot, expected := range tests {
		c.Logf("trying %q", ot)

		str := ot.String()

		c.Check(str, tc.Equals, expected)
	}
}

func (s *OriginTypeSuite) TestValidateValid(c *tc.C) {
	tests := []logfwd.OriginType{
		logfwd.OriginTypeUnknown,
		logfwd.OriginTypeUser,
		logfwd.OriginTypeMachine,
		logfwd.OriginTypeUnit,
	}
	for _, ot := range tests {
		c.Logf("trying %q", ot)

		err := ot.Validate()

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *OriginTypeSuite) TestValidateZero(c *tc.C) {
	var ot logfwd.OriginType

	err := ot.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *OriginTypeSuite) TestValidateInvalid(c *tc.C) {
	ot := logfwd.OriginType(999)

	err := ot.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `unsupported origin type`)
}

func (s *OriginTypeSuite) TestValidateNameValid(c *tc.C) {
	tests := map[logfwd.OriginType]string{
		logfwd.OriginTypeUnknown: "",
		logfwd.OriginTypeUser:    "a-user",
		logfwd.OriginTypeMachine: "99",
		logfwd.OriginTypeUnit:    "svc-a/0",
	}
	for ot, name := range tests {
		c.Logf("trying %q + %q", ot, name)

		err := ot.ValidateName(name)

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *OriginTypeSuite) TestValidateNameInvalid(c *tc.C) {
	tests := []struct {
		ot   logfwd.OriginType
		name string
		err  string
	}{{
		ot:   logfwd.OriginTypeUnknown,
		name: "...",
		err:  `origin name must not be set if type is unknown`,
	}, {
		ot:   logfwd.OriginTypeUser,
		name: "...",
		err:  `bad user name`,
	}, {
		ot:   logfwd.OriginTypeMachine,
		name: "...",
		err:  `bad machine name`,
	}, {
		ot:   logfwd.OriginTypeUnit,
		name: "...",
		err:  `bad unit name`,
	}}
	for _, test := range tests {
		c.Logf("trying %q + %q", test.ot, test.name)

		err := test.ot.ValidateName(test.name)

		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, test.err)
	}
}
