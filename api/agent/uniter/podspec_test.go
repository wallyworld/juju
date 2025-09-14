// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type podSpecSuite struct {
	coretesting.BaseSuite
}

func TestPodSpecSuite(t *tctesting.T) {
	tc.Run(t, &podSpecSuite{})
}

func (s *podSpecSuite) TestGetPodSpec(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetPodSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, tc.ErrorMatches, "yoink")
}

func (s *podSpecSuite) TestGetPodSpecInvalidApplicationName(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Fail()
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *podSpecSuite) TestGetPodSpecError(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetPodSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		called = true

		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		return errors.New(msg)
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(called, tc.IsTrue)
}

func (s *podSpecSuite) TestGetPodSpecArity(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetPodSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{}, {}},
		}
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetPodSpec("mysql")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *podSpecSuite) TestGetRawK8sSpec(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetRawK8sSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Error: &params.Error{Message: "yoink"},
			}},
		}
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, tc.ErrorMatches, "yoink")
}

func (s *podSpecSuite) TestGetRawK8sSpecInvalidApplicationName(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Fail()
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *podSpecSuite) TestGetRawK8sSpecError(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	var called bool
	msg := "yoink"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetRawK8sSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		called = true

		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		return errors.New(msg)
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(called, tc.IsTrue)
}

func (s *podSpecSuite) TestGetRawK8sSpecArity(c *tc.C) {
	expected := params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mysql",
		}},
	}

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(version, tc.Equals, 0)
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "GetRawK8sSpec")
		c.Assert(arg, tc.DeepEquals, expected)
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{}, {}},
		}
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := st.GetRawK8sSpec("mysql")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}
