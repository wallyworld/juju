// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	tctesting "testing"

	compute "cloud.google.com/go/compute/apiv1"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/tc"
)

type authSuite struct {
	Credentials *Credentials
}

func TestAuthSuite(t *tctesting.T) {
	tc.Run(t, &authSuite{})
}

func (s *authSuite) SetUpTest(c *tc.C) {
	s.Credentials = &Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("<some-key>"),
		JSONKey: []byte(`
{
    "private_key_id": "mnopq",
    "private_key": "<some-key>",
    "client_email": "user@mail.com",
    "client_id": "spam",
    "type": "service_account"
}`[1:]),
	}
}

func (s *authSuite) TestNewRESTClient(c *tc.C) {
	cfg, err := newJWTConfig(s.Credentials)
	c.Assert(err, tc.ErrorIsNil)
	ctx := c.Context()
	_, err = newRESTClient(ctx, cfg.TokenSource(ctx), jujuhttp.NewClient(), compute.NewNetworksRESTClient)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *authSuite) TestCreateJWTConfig(c *tc.C) {
	cfg, err := newJWTConfig(s.Credentials)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Scopes, tc.DeepEquals, Scopes)
}

func (s *authSuite) TestCreateJWTConfigWithNoJSONKey(c *tc.C) {
	cfg, err := newJWTConfig(&Credentials{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Scopes, tc.DeepEquals, Scopes)
}
