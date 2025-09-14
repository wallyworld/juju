// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"context"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/singular"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var otherUUID = utils.MustNewUUID().String()

type SingularSuite struct {
	testhelpers.IsolationSuite
}

func TestSingularSuite(t *tctesting.T) {
	tc.Run(t, &SingularSuite{})
}

func (s *SingularSuite) TestRequiresController(c *tc.C) {
	auth := mockAuth{nonController: true}
	facade, err := singular.NewFacade(nil, nil, auth)
	c.Check(facade, tc.IsNil)
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *SingularSuite) TestAcceptsController(c *tc.C) {
	backend := &mockBackend{}
	facade, err := singular.NewFacade(backend, backend, mockAuth{})
	c.Check(facade, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)

	backend.stub.CheckCallNames(c)
}

func (s *SingularSuite) TestInvalidClaims(c *tc.C) {
	breakers := []func(claim *params.SingularClaim){
		func(claim *params.SingularClaim) { claim.EntityTag = "machine-123" },
		func(claim *params.SingularClaim) { claim.EntityTag = "model-" + otherUUID },
		func(claim *params.SingularClaim) { claim.ClaimantTag = "" },
		func(claim *params.SingularClaim) { claim.ClaimantTag = "machine-42" },
		func(claim *params.SingularClaim) { claim.Duration = time.Second - time.Millisecond },
		func(claim *params.SingularClaim) { claim.Duration = time.Minute + time.Millisecond },
	}
	count := len(breakers)

	var claims params.SingularClaims
	claims.Claims = make([]params.SingularClaim, count)
	for i, breaker := range breakers {
		claim := params.SingularClaim{
			EntityTag:   coretesting.ModelTag.String(),
			ClaimantTag: "machine-123",
			Duration:    time.Minute,
		}
		breaker(&claim)
		claims.Claims[i] = claim
	}

	backend := &mockBackend{}
	facade, err := singular.NewFacade(backend, backend, mockAuth{})
	c.Assert(err, tc.ErrorIsNil)
	result := facade.Claim(claims)
	c.Assert(result.Results, tc.HasLen, count)

	for i, result := range result.Results {
		c.Logf("checking claim %d", i)
		checkDenied(c, result)
	}
	backend.stub.CheckCallNames(c)
}

func (s *SingularSuite) TestValidClaims(c *tc.C) {
	durations := []time.Duration{
		time.Second,
		10 * time.Second,
		30 * time.Second,
		time.Minute,
	}
	errors := []error{
		nil,
		errors.New("pow!"),
		lease.ErrClaimDenied,
		nil,
	}
	count := len(durations)
	if len(errors) != count {
		c.Fatalf("please fix your test data")
	}

	var claims params.SingularClaims
	claims.Claims = make([]params.SingularClaim, count)
	expectCalls := []testhelpers.StubCall{}
	for i, duration := range durations {
		var tag names.Tag = coretesting.ModelTag
		if i%2 == 1 {
			tag = coretesting.ControllerTag
		}
		claims.Claims[i] = params.SingularClaim{
			EntityTag:   tag.String(),
			ClaimantTag: "machine-123",
			Duration:    duration,
		}
		expectCalls = append(expectCalls, testhelpers.StubCall{
			FuncName: "Claim",
			Args: []interface{}{
				tag.Id(),
				"machine-123",
				durations[i],
			},
		})
	}

	backend := &mockBackend{}
	backend.stub.SetErrors(errors...)
	facade, err := singular.NewFacade(backend, backend, mockAuth{})
	c.Assert(err, tc.ErrorIsNil)
	result := facade.Claim(claims)
	c.Assert(result.Results, tc.HasLen, count)

	for i, err := range result.Results {
		switch errors[i] {
		case nil:
			c.Check(err.Error, tc.IsNil)
		case lease.ErrClaimDenied:
			c.Check(err.Error, tc.Satisfies, params.IsCodeLeaseClaimDenied)
		default:
			c.Check(err.Error.Error(), tc.Equals, errors[i].Error())
		}
	}
	backend.stub.CheckCalls(c, expectCalls)
}

func (s *SingularSuite) TestWait(c *tc.C) {
	waits := params.Entities{
		Entities: []params.Entity{{
			"machine-123", // rejected
		}, {
			"model-" + otherUUID, // rejected
		}, {
			coretesting.ModelTag.String(), // stub-error
		}, {
			coretesting.ModelTag.String(), // success
		}, {
			coretesting.ControllerTag.String(), // success
		}},
	}
	count := len(waits.Entities)

	backend := &mockBackend{}
	backend.stub.SetErrors(errors.New("zap!"), nil)
	facade, err := singular.NewFacade(backend, backend, mockAuth{})
	c.Assert(err, tc.ErrorIsNil)
	result := facade.Wait(context.TODO(), waits)
	c.Assert(result.Results, tc.HasLen, count)

	checkDenied(c, result.Results[0])
	checkDenied(c, result.Results[1])
	c.Check(result.Results[2].Error, tc.ErrorMatches, "zap!")
	c.Check(result.Results[3].Error, tc.IsNil)
	c.Check(result.Results[4].Error, tc.IsNil)

	backend.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "WaitUntilExpired",
		Args:     []interface{}{coretesting.ModelTag.Id()},
	}, {
		FuncName: "WaitUntilExpired",
		Args:     []interface{}{coretesting.ModelTag.Id()},
	}, {
		FuncName: "WaitUntilExpired",
		Args:     []interface{}{coretesting.ControllerTag.Id()},
	}})
}

func (s *SingularSuite) TestWaitCancelled(c *tc.C) {
	waits := params.Entities{
		Entities: []params.Entity{{
			coretesting.ModelTag.String(), // success
		}},
	}
	count := len(waits.Entities)

	backend := &mockBackend{}
	facade, err := singular.NewFacade(backend, backend, mockAuth{})
	c.Assert(err, tc.ErrorIsNil)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	result := facade.Wait(ctx, waits)
	c.Assert(result.Results, tc.HasLen, count)
	c.Check(result.Results[0].Error, tc.ErrorMatches, "waiting for lease cancelled by client")
}

func checkDenied(c *tc.C, result params.ErrorResult) {
	if !c.Check(result.Error, tc.NotNil) {
		return
	}
	c.Check(result.Error, tc.ErrorMatches, "permission denied")
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}
