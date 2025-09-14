// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/singular"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type APISuite struct {
	testhelpers.IsolationSuite
}

func TestAPISuite(t *tctesting.T) {
	tc.Run(t, &APISuite{})
}

var machine123 = names.NewMachineTag("123")

func (s *APISuite) TestBadClaimantTag(c *tc.C) {
	apiCaller := apiCaller(c, nil, nil)
	badTag := names.NewMachineTag("")
	api, err := singular.NewAPI(apiCaller, badTag, nil)
	c.Check(api, tc.IsNil)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, "claimant tag not valid")
}

func (s *APISuite) TestBadEntityTag(c *tc.C) {
	apiCaller := apiCaller(c, nil, nil)

	api, err := singular.NewAPI(apiCaller, machine123, nil)
	c.Check(api, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "nil entity supplied")

	api, err = singular.NewAPI(apiCaller, machine123, machine123)
	c.Check(api, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `invalid entity kind "machine" for singular API`)
}

func (s *APISuite) TestNoCalls(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, nil, nil)
	_, err := singular.NewAPI(apiCaller, machine123, coretesting.ControllerTag)
	c.Check(err, tc.ErrorIsNil)
	stub.CheckCallNames(c)
}

func (s *APISuite) TestClaimSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)

	err = api.Claim(time.Minute)
	c.Check(err, tc.ErrorIsNil)
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Minute,
		}},
	})
}

func (s *APISuite) TestClaimDenied(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: apiservererrors.ServerError(lease.ErrClaimDenied),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)

	err = api.Claim(time.Hour)
	c.Check(err, tc.Equals, lease.ErrClaimDenied)
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Hour,
		}},
	})
}

func (s *APISuite) TestClaimError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.New("zap pow splat oof")),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)

	err = api.Claim(time.Second)
	c.Check(err, tc.ErrorMatches, "zap pow splat oof")
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Second,
		}},
	})
}

func (s *APISuite) TestWaitSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)

	err = api.Wait()
	c.Check(err, tc.ErrorIsNil)
	checkCall(c, stub, "Wait", params.Entities{
		Entities: []params.Entity{{
			Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}},
	})
}

func (s *APISuite) TestWaitError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.New("crunch squelch")),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)

	err = api.Wait()
	c.Check(err, tc.ErrorMatches, "crunch squelch")
	checkCall(c, stub, "Wait", params.Entities{
		Entities: []params.Entity{{
			Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}},
	})
}

type setResultFunc func(result *params.ErrorResults) error

func apiCaller(c *tc.C, stub *testhelpers.Stub, setResult setResultFunc) base.APICaller {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			stub.AddCall(objType, version, id, request, args)
			result, ok := response.(*params.ErrorResults)
			c.Assert(ok, tc.IsTrue)
			return setResult(result)
		},
	)
}

func checkCall(c *tc.C, stub *testhelpers.Stub, method string, args interface{}) {
	stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Singular",
		Args:     []interface{}{0, "", method, args},
	}})
}
