// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
)

func TestPayloadSuite(t *tctesting.T) {
	tc.Run(t, &payloadSuite{})
}

type payloadSuite struct {
	testhelpers.IsolationSuite
}

func (s *payloadSuite) newPayload(name, pType string) payloads.Payload {
	return payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		ID:     "id" + name,
		Status: payloads.StateRunning,
		Labels: []string{"a-tag"},
		Unit:   "a-application/0",
	}
}

func (s *payloadSuite) TestFullID(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	id := payload.FullID()

	c.Check(id, tc.Equals, "spam/idspam")
}

func (s *payloadSuite) TestFullIDMissingID(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.ID = ""
	id := payload.FullID()

	c.Check(id, tc.Equals, "spam")
}

func (s *payloadSuite) TestValidateOkay(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	err := payload.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *payloadSuite) TestValidateMissingName(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Name = ""
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `payload class missing name`)
}

func (s *payloadSuite) TestValidateMissingType(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Type = ""
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `payload class missing type`)
}

func (s *payloadSuite) TestValidateMissingID(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.ID = ""
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `missing ID .*`)
}

func (s *payloadSuite) TestValidateMissingStatus(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Status = ""
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `status .* not supported; expected one of .*`)
}

func (s *payloadSuite) TestValidateUnknownStatus(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Status = "some-unknown-value"
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `status .* not supported; expected one of .*`)
}

func (s *payloadSuite) TestValidateMissingUnit(c *tc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Unit = ""
	err := payload.Validate()

	c.Check(err, tc.ErrorMatches, `missing Unit .*`)
}
