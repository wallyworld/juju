// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/usersecrets"
	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretSuite(t *tctesting.T) {
	tc.Run(t, &secretSuite{})
}

type secretSuite struct {
	coretesting.BaseSuite
}

func (s *secretSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *secretSuite) TestWatchRevisionsToPrune(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "UserSecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchRevisionsToPrune")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	_, err := client.WatchRevisionsToPrune()
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *secretSuite) TestDeleteRevisions(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "UserSecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "DeleteRevisions")
		c.Check(arg, tc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{
					URI:       uri.String(),
					Revisions: []int{1, 2, 3},
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	err := client.DeleteRevisions(uri, 1, 2, 3)
	c.Assert(err, tc.ErrorMatches, "boom")
}
