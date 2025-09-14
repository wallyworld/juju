// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/charmrevisionupdater"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type versionUpdaterSuite struct {
	coretesting.BaseSuite
}

func TestVersionUpdaterSuite(t *tctesting.T) {
	tc.Run(t, &versionUpdaterSuite{})
}

func (s *versionUpdaterSuite) TestUpdateRevisions(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CharmRevisionUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "UpdateLatestRevisions")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{
			Error: &params.Error{Message: "boom"},
		}
		return nil
	})

	client := charmrevisionupdater.NewClient(apiCaller)
	err := client.UpdateLatestRevisions()
	c.Assert(err, tc.ErrorMatches, "boom")
}
