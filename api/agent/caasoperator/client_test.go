// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/agent/caasoperator"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type operatorSuite struct {
	testhelpers.IsolationSuite
}

func TestOperatorSuite(t *tctesting.T) {
	tc.Run(t, &operatorSuite{})
}

func (s *operatorSuite) TestSetStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetStatus")
		c.Check(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "foo",
				Info:   "bar",
				Data: map[string]interface{}{
					"baz": "qux",
				},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetStatus("gitlab", "foo", "bar", map[string]interface{}{
		"baz": "qux",
	})
	c.Assert(err, tc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestSetStatusInvalidApplicationName(c *tc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetStatus("", "foo", "bar", nil)
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestCharm(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Charm")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ApplicationCharmResults{})
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{
				Result: &params.ApplicationCharm{
					URL:                  "ch:foo/bar-1",
					ForceUpgrade:         true,
					SHA256:               "fake-sha256",
					CharmModifiedVersion: 666,
					DeploymentMode:       "workload",
				},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	info, err := client.Charm("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.URL, tc.NotNil)
	c.Assert(info.URL.String(), tc.Equals, "ch:foo/bar-1")
	c.Assert(info.SHA256, tc.Equals, "fake-sha256")
	c.Assert(info.ForceUpgrade, tc.IsTrue)
	c.Assert(info.CharmModifiedVersion, tc.Equals, 666)
	c.Assert(info.DeploymentMode, tc.Equals, caas.ModeWorkload)
}

func (s *operatorSuite) TestCharmError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{Error: &params.Error{Code: params.CodeNotFound}}},
		}
		return nil
	})
	client := caasoperator.NewClient(apiCaller)
	_, err := client.Charm("gitlab")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *operatorSuite) TestCharmInvalidApplicationName(c *tc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Charm("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestModel(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "CurrentModel")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ModelResult{})
		*(result.(*params.ModelResult)) = params.ModelResult{
			Name: "some-model",
			UUID: "deadbeef",
			Type: "iaas",
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	m, err := client.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.DeepEquals, &model.Model{
		Name:      "some-model",
		UUID:      "deadbeef",
		ModelType: model.IAAS,
	})
}

func (s *operatorSuite) TestWatchUnits(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchUnits")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	watcher, err := client.WatchUnits("gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *operatorSuite) TestRemoveUnit(c *tc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Remove")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-gitlab-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.RemoveUnit("gitlab/0")
	c.Assert(err, tc.ErrorMatches, "FAIL")
	c.Assert(called, tc.IsTrue)
}

func (s *operatorSuite) TestRemoveUnitNotFound(c *tc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Remove")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-gitlab-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		called = true
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.RemoveUnit("gitlab/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *operatorSuite) TestRemoveUnitInvalidUnitName(c *tc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.RemoveUnit("")
	c.Assert(err, tc.ErrorMatches, `unit name "" not valid`)
}

func (s *operatorSuite) TestLife(c *tc.C) {
	s.testLife(c, names.NewApplicationTag("gitlab"))
	s.testLife(c, names.NewUnitTag("gitlab/0"))
}

func (s *operatorSuite) testLife(c *tc.C, tag names.Tag) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: tag.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *operatorSuite) TestLifeError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	_, err := client.Life("gitlab/0")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *operatorSuite) TestLifeInvalidEntityName(c *tc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, tc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *operatorSuite) TestSetVersion(c *tc.C) {
	called := false
	vers := version.Binary{}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetTools")
		c.Check(arg, tc.DeepEquals, params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag:   "application-gitlab",
				Tools: &params.Version{Version: vers},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetVersion("gitlab", vers)
	c.Assert(err, tc.ErrorMatches, "FAIL")
	c.Assert(called, tc.IsTrue)
}

func (s *operatorSuite) TestSetVersionInvalidApplicationName(c *tc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetVersion("", version.Binary{})
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestWatchUnitStart(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchContainerStart")
		c.Assert(arg, tc.DeepEquals, params.WatchContainerStartArgs{
			Args: []params.WatchContainerStartArg{{
				Entity: params.Entity{
					Tag: "application-gitlab",
				},
				Container: "container",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	watcher, err := client.WatchContainerStart("gitlab", "container")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
