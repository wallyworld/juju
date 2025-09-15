// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"net/http"
	tctesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type legacyLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

func TestLegacyLoginProviderSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &legacyLoginProviderSuite{})
}

// TestLegacyProviderLogin verifies that the legacy login provider
// works for login and returns the password as the token.
func (s *legacyLoginProviderSuite) TestLegacyProviderLogin(c *tc.C) {
	info := s.APIInfo(c)

	username := names.NewUserTag("admin")
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(username, password, "", nil, nil, nil)
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer apiState.Close()
	c.Check(err, tc.ErrorIsNil)
}

func (s *legacyLoginProviderSuite) TestLegacyProviderWithNilTag(c *tc.C) {
	info := s.APIInfo(c)
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(nil, password, "", nil, nil, nil)
	_, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorMatches, `failed to authenticate request: unauthorized \(unauthorized access\)`)
}

// A separate suite for tests that don't need to connect to a controller.
type legacyLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

func TestLegacyLoginProviderBasicSuite(t *tctesting.T) {
	tc.Run(t, &legacyLoginProviderBasicSuite{})
}

func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeader(c *tc.C) {
	userTag := names.NewUserTag("bob")
	password := "test-password"
	nonce := "test-nonce"
	header := jujuhttp.BasicAuthHeader(userTag.String(), password)
	header.Add(params.MachineNonceHeader, nonce)
	header.Add(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	lp := api.NewLegacyLoginProvider(
		userTag,
		password,
		nonce,
		[]macaroon.Slice{},
		nil,
		nil,
	)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, header)
}

func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeaderWithNilTag(c *tc.C) {
	password := "test-password"
	nonce := "test-nonce"
	header := http.Header{}
	header.Add(params.MachineNonceHeader, nonce)
	header.Add(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	lp := api.NewLegacyLoginProvider(
		nil,
		password,
		nonce,
		[]macaroon.Slice{},
		nil,
		nil,
	)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, header)
}
