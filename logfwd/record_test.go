// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/logfwd"
)

type RecordSuite struct {
	testhelpers.IsolationSuite
}

func TestRecordSuite(t *tctesting.T) {
	tc.Run(t, &RecordSuite{})
}

func (s *RecordSuite) TestValidateValid(c *tc.C) {
	rec := validRecord

	err := rec.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *RecordSuite) TestValidateZero(c *tc.C) {
	var rec logfwd.Record

	err := rec.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *RecordSuite) TestValidateBadOrigin(c *tc.C) {
	rec := validRecord
	rec.Origin.Name = "..."

	err := rec.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `invalid Origin: invalid Name "...": bad user name`)
}

func (s *RecordSuite) TestValidateEmptyTimestamp(c *tc.C) {
	rec := validRecord
	rec.Timestamp = time.Time{}

	err := rec.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty Timestamp`)
}

func (s *RecordSuite) TestValidateBadLocation(c *tc.C) {
	rec := validRecord
	rec.Location.Filename = ""

	err := rec.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `invalid Location: Line set but Filename empty`)
}

type LocationSuite struct {
	testhelpers.IsolationSuite
}

func TestLocationSuite(t *tctesting.T) {
	tc.Run(t, &LocationSuite{})
}

func (s *LocationSuite) TestParseLocationTooLegitToQuit(c *tc.C) {
	expected := validLocation

	loc, err := logfwd.ParseLocation(expected.Module, expected.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(loc, tc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationIsValid(c *tc.C) {
	expected := validLocation
	loc, err := logfwd.ParseLocation(expected.Module, expected.String())
	c.Assert(err, tc.ErrorIsNil)

	err = loc.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *LocationSuite) TestParseLocationMissingFilename(c *tc.C) {
	expected := validLocation
	expected.Filename = ""

	loc, err := logfwd.ParseLocation(expected.Module, ":42")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(loc, tc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationBogusFilename(c *tc.C) {
	expected := validLocation
	expected.Filename = "..."

	loc, err := logfwd.ParseLocation(expected.Module, "...:42")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(loc, tc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationFilenameOnly(c *tc.C) {
	expected := validLocation
	expected.Line = -1

	loc, err := logfwd.ParseLocation(expected.Module, expected.Filename)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(loc, tc.DeepEquals, expected)
}

func (s *LocationSuite) TestParseLocationMissingLine(c *tc.C) {
	_, err := logfwd.ParseLocation(validLocation.Module, "spam.go:")

	c.Check(err, tc.ErrorMatches, `failed to parse sourceLine: missing line number after ":"`)
}

func (s *LocationSuite) TestParseLocationBogusLine(c *tc.C) {
	_, err := logfwd.ParseLocation(validLocation.Module, "spam.go:xxx")

	c.Check(err, tc.ErrorMatches, `failed to parse sourceLine: line number must be non-negative integer: strconv.(ParseInt|Atoi): parsing "xxx": invalid syntax`)
}

func (s *LocationSuite) TestValidateValid(c *tc.C) {
	loc := validLocation

	err := loc.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *LocationSuite) TestValidateEmpty(c *tc.C) {
	var loc logfwd.SourceLocation

	err := loc.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *LocationSuite) TestValidateBadLine(c *tc.C) {
	loc := validLocation
	loc.Filename = ""

	err := loc.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `Line set but Filename empty`)
}

var validLocation = logfwd.SourceLocation{
	Module:   "spam",
	Filename: "eggs.go",
	Line:     42,
}

var validRecord = logfwd.Record{
	Origin:    validOrigin,
	Timestamp: time.Now(),
	Level:     loggo.ERROR,
	Location:  validLocation,
	Message:   "uh-oh",
}
