// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/logfwd"
)

type OriginSuite struct {
	testhelpers.IsolationSuite
}

func TestOriginSuite(t *tctesting.T) {
	tc.Run(t, &OriginSuite{})
}

func (s *OriginSuite) TestOriginForMachineAgent(c *tc.C) {
	tag := names.NewMachineTag("99")

	origin := logfwd.OriginForMachineAgent(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)

	c.Check(origin, tc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "machine-99." + validOrigin.ModelUUID,
		Type:           logfwd.OriginTypeMachine,
		Name:           "99",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "jujud-machine-agent",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestOriginForUnitAgent(c *tc.C) {
	tag := names.NewUnitTag("svc-a/0")

	origin := logfwd.OriginForUnitAgent(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)

	c.Check(origin, tc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "unit-svc-a-0." + validOrigin.ModelUUID,
		Type:           logfwd.OriginTypeUnit,
		Name:           "svc-a/0",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "jujud-unit-agent",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestOriginForJuju(c *tc.C) {
	tag := names.NewUserTag("bob")

	origin, err := logfwd.OriginForJuju(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(origin, tc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "",
		Type:           logfwd.OriginTypeUser,
		Name:           "bob",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "juju",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestValidateValid(c *tc.C) {
	origin := validOrigin

	err := origin.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateEmpty(c *tc.C) {
	var origin logfwd.Origin

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *OriginSuite) TestValidateEmptyControllerUUID(c *tc.C) {
	origin := validOrigin
	origin.ControllerUUID = ""

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty ControllerUUID`)
}

func (s *OriginSuite) TestValidateBadControllerUUID(c *tc.C) {
	origin := validOrigin
	origin.ControllerUUID = "..."

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `ControllerUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateEmptyModelUUID(c *tc.C) {
	origin := validOrigin
	origin.ModelUUID = ""

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty ModelUUID`)
}

func (s *OriginSuite) TestValidateBadModelUUID(c *tc.C) {
	origin := validOrigin
	origin.ModelUUID = "..."

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `ModelUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateEmptyHostname(c *tc.C) {
	origin := validOrigin
	origin.Hostname = ""

	err := origin.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateBadOriginType(c *tc.C) {
	origin := validOrigin
	origin.Type = logfwd.OriginType(999)

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `invalid Type: unsupported origin type`)
}

func (s *OriginSuite) TestValidateEmptyName(c *tc.C) {
	origin := validOrigin
	origin.Name = ""

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `empty Name`)
}

func (s *OriginSuite) TestValidateBadName(c *tc.C) {
	origin := validOrigin
	origin.Name = "..."

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `invalid Name "...": bad user name`)
}

func (s *OriginSuite) TestValidateEmptySoftware(c *tc.C) {
	origin := validOrigin
	origin.Software = logfwd.Software{}

	err := origin.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateBadSoftware(c *tc.C) {
	origin := validOrigin
	origin.Software.Version = version.Zero

	err := origin.Validate()

	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `invalid Software: empty Version`)
}

var validOrigin = logfwd.Origin{
	ControllerUUID: "9f484882-2f18-4fd2-967d-db9663db7bea",
	ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
	Hostname:       "spam.x.y.z.com",
	Type:           logfwd.OriginTypeUser,
	Name:           "a-user",
	Software: logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	},
}
