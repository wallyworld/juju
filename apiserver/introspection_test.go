// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	tctesting "testing"

	"github.com/juju/tc"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type introspectionSuite struct {
	apiserverBaseSuite
	bob *state.User
	url string
}

func TestIntrospectionSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &introspectionSuite{})
}

func (s *introspectionSuite) SetUpTest(c *tc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	bob, err := s.State.AddUser("bob", "", "hunter2", "admin")
	c.Assert(err, tc.ErrorIsNil)
	s.bob = bob
	s.url = s.server.URL + "/introspection/navel"
}

func (s *introspectionSuite) TestAccess(c *tc.C) {
	s.testAccess(c, s.Owner.String(), ownerPassword)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, err = model.AddUser(
		state.UserAccessSpec{
			User:      s.bob.UserTag(),
			CreatedBy: s.Owner,
			Access:    permission.ReadAccess,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.testAccess(c, "user-bob", "hunter2")
}

func (s *introspectionSuite) testAccess(c *tc.C, tag, password string) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      tag,
		Password: password,
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	content, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, "gazing")
}

func (s *introspectionSuite) TestAccessDenied(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      "user-bob",
		Password: "hunter2",
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusForbidden)
}
