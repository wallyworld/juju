// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucaas "github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caas"
	"github.com/juju/juju/worker/workertest"
)

type TrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) TestValidateObserver(c *gc.C) {
	config := caas.Config{}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil Observer not valid")
	})
}

func (s *TrackerSuite) TestValidateNewBrokerFunc(c *gc.C) {
	config := caas.Config{
		Observer: &runContext{},
	}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil NewBrokerFunc not valid")
	})
}

func (s *TrackerSuite) testValidate(c *gc.C, config caas.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	tracker, err := caas.NewTracker(config)
	c.Check(tracker, gc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestCloudSpecFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no yuo"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caas.NewTracker(caas.Config{
			Observer:      context,
			NewBrokerFunc: newMockBroker,
		})
		c.Check(err, gc.ErrorMatches, "cannot get cloud information: no yuo")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}

func (s *TrackerSuite) TestSuccess(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := caas.NewTracker(caas.Config{
			Observer:      context,
			NewBrokerFunc: newMockBroker,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotBroker := tracker.Broker()
		c.Assert(gotBroker, gc.NotNil)
	})
}

func (s *TrackerSuite) TestCloudSpec(c *gc.C) {
	cloudSpec := environs.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{cloud: cloudSpec}
	fix.Run(c, func(context *runContext) {
		tracker, err := caas.NewTracker(caas.Config{
			Observer: context,
			NewBrokerFunc: func(spec environs.CloudSpec) (jujucaas.Broker, error) {
				c.Assert(spec, jc.DeepEquals, cloudSpec)
				return nil, errors.NotValidf("cloud spec")
			},
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}
