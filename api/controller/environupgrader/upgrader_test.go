// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/environupgrader"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var (
	modelTag = names.NewModelTag("e5757df7-c86a-4835-84bc-7174af535d25")
)

func TestEnvironUpgraderSuite(t *tctesting.T) {
	tc.Run(t, &EnvironUpgraderSuite{})
}

type EnvironUpgraderSuite struct {
	coretesting.BaseSuite
}

func (s *EnvironUpgraderSuite) TestModelEnvironVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "EnvironUpgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ModelEnvironVersion")
		c.Check(arg, tc.DeepEquals, &params.Entities{
			Entities: []params.Entity{{Tag: modelTag.String()}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.IntResults{})
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Result: 1,
			}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	version, err := client.ModelEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(version, tc.Equals, 1)
}

func (s *EnvironUpgraderSuite) TestModelEnvironVersionError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Error: &params.Error{Message: "foo"},
			}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	_, err := client.ModelEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *EnvironUpgraderSuite) TestModelEnvironArityMismatch(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{}, {}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	_, err := client.ModelEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *EnvironUpgraderSuite) TestModelTargetEnvironVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "EnvironUpgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ModelTargetEnvironVersion")
		c.Check(arg, tc.DeepEquals, &params.Entities{
			Entities: []params.Entity{{Tag: modelTag.String()}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.IntResults{})
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Result: 1,
			}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	version, err := client.ModelTargetEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(version, tc.Equals, 1)
}

func (s *EnvironUpgraderSuite) TestModelTargetEnvironVersionError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Error: &params.Error{Message: "foo"},
			}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	_, err := client.ModelTargetEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *EnvironUpgraderSuite) TestModelTargetEnvironArityMismatch(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{}, {}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	_, err := client.ModelTargetEnvironVersion(modelTag)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *EnvironUpgraderSuite) TestSetModelEnvironVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "EnvironUpgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetModelEnvironVersion")
		c.Check(arg, tc.DeepEquals, &params.SetModelEnvironVersions{
			Models: []params.SetModelEnvironVersion{{
				ModelTag: modelTag.String(),
				Version:  1,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "foo"}}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	err := client.SetModelEnvironVersion(modelTag, 1)
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *EnvironUpgraderSuite) TestSetModelEnvironVersionArityMismatch(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}, {}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	err := client.SetModelEnvironVersion(modelTag, 1)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *EnvironUpgraderSuite) TestSetModelStatus(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "EnvironUpgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetModelStatus")
		c.Check(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    modelTag.String(),
				Status: "foo",
				Info:   "bar",
				Data: map[string]interface{}{
					"baz": "qux",
				},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "foo"}}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	err := client.SetModelStatus(modelTag, "foo", "bar", map[string]interface{}{
		"baz": "qux",
	})
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *EnvironUpgraderSuite) TestSetModelStatusArityMismatch(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}, {}},
		}
		return nil
	})

	client := environupgrader.NewClient(apiCaller)
	err := client.SetModelStatus(modelTag, "foo", "bar", nil)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}
