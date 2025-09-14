// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager/mocks"
)

func TestManifoldSuite(t *tctesting.T) {
	tc.Run(t, &manifoldSuite{})
}

type manifoldSuite struct {
	testhelpers.IsolationSuite
	config caasmodelconfigmanager.ManifoldConfig
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *manifoldSuite) validConfig() caasmodelconfigmanager.ManifoldConfig {
	return caasmodelconfigmanager.ManifoldConfig{
		APICallerName: "api-caller",
		BrokerName:    "broker",
		NewWorker: func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewFacade: func(caller base.APICaller) (caasmodelconfigmanager.Facade, error) {
			return nil, nil
		},
		Logger: loggo.GetLogger("test"),
		Clock:  testclock.NewClock(time.Time{}),
	}
}

func (s *manifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *manifoldSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *tc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewFacade(c *tc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *manifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	called := false
	s.config.NewFacade = func(caller base.APICaller) (caasmodelconfigmanager.Facade, error) {
		return mocks.NewMockFacade(ctrl), nil
	}
	s.config.NewWorker = func(config caasmodelconfigmanager.Config) (worker.Worker, error) {
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, tc.NotNil)
		mc.AddExpr(`_.Broker`, tc.NotNil)
		mc.AddExpr(`_.Logger`, tc.NotNil)
		mc.AddExpr(`_.RegistryFunc`, tc.NotNil)
		mc.AddExpr(`_.Clock`, tc.NotNil)
		c.Check(config, mc, caasmodelconfigmanager.Config{
			ModelTag: names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		return nil, nil
	}
	manifold := caasmodelconfigmanager.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
		"broker":     struct{ caas.Broker }{},
	}))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.IsNil)
	c.Assert(called, tc.IsTrue)
}

type mockAPICaller struct {
	base.APICaller
}

func (*mockAPICaller) BestFacadeVersion(facade string) int {
	return 1
}

func (*mockAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"), true
}
