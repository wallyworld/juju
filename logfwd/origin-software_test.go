// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/logfwd"
)

type SoftwareSuite struct {
	testhelpers.IsolationSuite
}

func TestSoftwareSuite(t *tctesting.T) {
	tc.Run(t, &SoftwareSuite{})
}

func (s *SoftwareSuite) TestValidateFull(c *tc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *SoftwareSuite) TestValidateZeroValue(c *tc.C) {
	var sw logfwd.Software

	err := sw.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *SoftwareSuite) TestValidateEmptyPEN(c *tc.C) {
	sw := logfwd.Software{
		Name:    "juju",
		Version: version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `missing PrivateEnterpriseNumber`)
}

func (s *SoftwareSuite) TestValidateNegativePEN(c *tc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: -1,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `missing PrivateEnterpriseNumber`)
}

func (s *SoftwareSuite) TestValidateEmptyName(c *tc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty Name`)
}

func (s *SoftwareSuite) TestValidateEmptyVersion(c *tc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Name:                    "juju",
	}

	err := sw.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty Version`)
}
