// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	tctesting "testing"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type clientCredentialsLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

func TestClientCredentialsLoginProviderSuite(t *tctesting.T) {
	tc.Run(t, &clientCredentialsLoginProviderSuite{})
}

func (s *clientCredentialsLoginProviderSuite) TestClientCredentialsLogin(c *tc.C) {
	info := s.APIInfo(c)

	clientID := "test-client-id"
	clientSecret := "test-client-secret"

	s.PatchValue(api.LoginWithClientCredentialsAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
		data, err := json.Marshal(request)
		if err != nil {
			return errors.Trace(err)
		}

		var lr struct {
			ClientID     string `json:"client-id"`
			ClientSecret string `json:"client-secret"`
		}

		err = json.Unmarshal(data, &lr)
		if err != nil {
			return errors.Trace(err)
		}

		if lr.ClientID != clientID {
			return errors.Unauthorized
		}
		if lr.ClientSecret != clientSecret {
			return errors.Unauthorized
		}

		loginResult, ok := response.(*params.LoginResult)
		if !ok {
			return errors.Errorf("expected %T, received %T for response type", loginResult, response)
		}
		loginResult.ControllerTag = names.NewControllerTag(info.ControllerUUID).String()
		loginResult.ServerVersion = "3.4.0"
		loginResult.UserInfo = &params.AuthUserInfo{
			DisplayName:      "alice@external",
			Identity:         names.NewUserTag("alice@external").String(),
			ControllerAccess: "superuser",
		}
		return nil
	})

	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()
	c.Check(err, tc.ErrorIsNil)
}

// A separate suite for tests that don't need to communicate with a Juju controller.
type clientCredentialsLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

func TestClientCredentialsLoginProviderBasicSuite(t *tctesting.T) {
	tc.Run(t, &clientCredentialsLoginProviderBasicSuite{})
}

func (s *clientCredentialsLoginProviderBasicSuite) TestClientCredentialsAuthHeader(c *tc.C) {
	clientID := "test-client-id"
	clientSecret := "test-client-secret"
	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	expectedHeader := jujuhttp.BasicAuthHeader(clientID, clientSecret)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, expectedHeader)
}
