// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/undertaker"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *tctesting.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestNilFacade(c *tc.C) {
	config := validConfig()
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilCredentialAPI(c *tc.C) {
	config := validConfig()
	config.CredentialAPI = nil
	checkInvalid(c, config, "nil CredentialAPI not valid")
}

func (*ValidateSuite) TestNilLogger(c *tc.C) {
	config := validConfig()
	config.Logger = nil
	checkInvalid(c, config, "nil Logger not valid")
}

func (*ValidateSuite) TestNilNewCloudDestroyerFunc(c *tc.C) {
	config := validConfig()
	config.NewCloudDestroyerFunc = nil
	checkInvalid(c, config, "nil NewCloudDestroyerFunc not valid")
}

func (*ValidateSuite) TestNilClock(c *tc.C) {
	config := validConfig()
	config.Clock = nil
	checkInvalid(c, config, "nil Clock not valid")
}

func validConfig() undertaker.Config {
	return undertaker.Config{
		Facade:                &fakeFacade{},
		CredentialAPI:         &fakeCredentialAPI{},
		Logger:                &fakeLogger{},
		Clock:                 testclock.NewClock(time.Time{}),
		NewCloudDestroyerFunc: func(ctx context.Context, op environs.OpenParams) (environs.CloudDestroyer, error) { return nil, nil },
	}
}

func checkInvalid(c *tc.C, config undertaker.Config, message string) {
	check := func(err error) {
		c.Check(err, tc.Satisfies, errors.IsNotValid)
		c.Check(err, tc.ErrorMatches, message)
	}
	err := config.Validate()
	check(err)

	worker, err := undertaker.NewUndertaker(config)
	c.Check(worker, tc.IsNil)
	check(err)
}
