// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/retrystrategy"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	jujufactory "github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

func TestRetryStrategySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &retryStrategySuite{})
}

type retryStrategySuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	unit *state.Unit

	strategy retrystrategy.RetryStrategy
}

var tagsTests = []struct {
	tag         string
	expectedErr string
}{
	{"user-admin", "permission denied"},
	{"unit-wut-4", "permission denied"},
	{"definitelynotatag", `"definitelynotatag" is not a valid tag`},
	{"machine-5", "permission denied"},
}

func (s *retryStrategySuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.UnitTag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	strategy, err := retrystrategy.NewRetryStrategyAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.strategy = strategy
}

func (s *retryStrategySuite) TestRetryStrategyUnauthenticated(c *tc.C) {
	svc, err := s.unit.Application()
	c.Assert(err, tc.ErrorIsNil)
	otherUnit := s.Factory.MakeUnit(c, &jujufactory.UnitParams{Application: svc})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.RetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].Result, tc.IsNil)
}

func (s *retryStrategySuite) TestRetryStrategyBadTag(c *tc.C) {
	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}
	res, err := s.strategy.RetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, tc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].Result, tc.IsNil)
	}
}

func (s *retryStrategySuite) TestRetryStrategyUnit(c *tc.C) {
	s.assertRetryStrategy(c, s.unit.Tag().String())
}

func (s *retryStrategySuite) TestRetryStrategyApplication(c *tc.C) {
	app := s.Factory.MakeApplication(c, &jujufactory.ApplicationParams{Name: "app"})
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: app.Tag(),
	}

	strategy, err := retrystrategy.NewRetryStrategyAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.strategy = strategy

	s.assertRetryStrategy(c, app.Tag().String())
}

func (s *retryStrategySuite) assertRetryStrategy(c *tc.C, tag string) {
	expected := &params.RetryStrategy{
		ShouldRetry:     true,
		MinRetryTime:    retrystrategy.MinRetryTime,
		MaxRetryTime:    retrystrategy.MaxRetryTime,
		JitterRetryTime: retrystrategy.JitterRetryTime,
		RetryTimeFactor: retrystrategy.RetryTimeFactor,
	}
	args := params.Entities{Entities: []params.Entity{{Tag: tag}}}
	r, err := s.strategy.RetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 1)
	c.Assert(r.Results[0].Error, tc.IsNil)
	c.Assert(r.Results[0].Result, tc.DeepEquals, expected)

	s.setRetryStrategy(c, false)
	expected.ShouldRetry = false

	r, err = s.strategy.RetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 1)
	c.Assert(r.Results[0].Error, tc.IsNil)
	c.Assert(r.Results[0].Result, tc.DeepEquals, expected)
}

func (s *retryStrategySuite) setRetryStrategy(c *tc.C, automaticallyRetryHooks bool) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"automatically-retry-hooks": automaticallyRetryHooks}, nil)
	c.Assert(err, tc.ErrorIsNil)
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelConfig.AutomaticallyRetryHooks(), tc.Equals, automaticallyRetryHooks)
}

func (s *retryStrategySuite) TestWatchRetryStrategyUnauthenticated(c *tc.C) {
	svc, err := s.unit.Application()
	c.Assert(err, tc.ErrorIsNil)
	otherUnit := s.Factory.MakeUnit(c, &jujufactory.UnitParams{Application: svc})
	args := params.Entities{Entities: []params.Entity{{otherUnit.Tag().String()}}}

	res, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Assert(res.Results[0].NotifyWatcherId, tc.Equals, "")
}

func (s *retryStrategySuite) TestWatchRetryStrategyBadTag(c *tc.C) {
	args := params.Entities{Entities: make([]params.Entity, len(tagsTests))}
	for i, t := range tagsTests {
		args.Entities[i] = params.Entity{Tag: t.tag}
	}
	res, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, len(tagsTests))
	for i, r := range res.Results {
		c.Logf("result %d", i)
		c.Assert(r.Error, tc.ErrorMatches, tagsTests[i].expectedErr)
		c.Assert(res.Results[i].NotifyWatcherId, tc.Equals, "")
	}
}

func (s *retryStrategySuite) TestWatchRetryStrategy(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.unit.UnitTag().String()},
		{Tag: "unit-foo-42"},
	}}
	r, err := s.strategy.WatchRetryStrategy(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	s.setRetryStrategy(c, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}
