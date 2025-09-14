// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/stateconverter"
	"github.com/juju/juju/internal/worker/stateconverter/mocks"
)

func TestManifoldConfigSuite(t *tctesting.T) {
	tc.Run(t, &manifoldConfigSuite{})
}

type manifoldConfigSuite struct {
	machiner *mocks.MockMachiner
	agent    *mocks.MockAgent
	config   *mocks.MockConfig
	context  *mocks.MockContext
}

func (s *manifoldConfigSuite) TestValidateAgentNameFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("empty AgentName").Error())
}

func (s *manifoldConfigSuite) TestValidateAPICallerFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName: "machine-2",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("empty APICallerName").Error())
}

func (s *manifoldConfigSuite) TestValidateLoggerFail(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
	}
	err := cfg.Validate()
	c.Assert(err.Error(), tc.Equals, errors.NotValidf("nil Logger").Error())
}

func (s *manifoldConfigSuite) TestValidateSuccess(c *tc.C) {
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "machine-2",
		APICallerName: "machiner",
		Logger:        &fakeLogger{},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *manifoldConfigSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan any)
	cfg := stateconverter.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "machiner",
		Logger:        &fakeLogger{},
		NewMachinerAPI: func(_ base.APICaller) stateconverter.Machiner {
			return s.machiner
		},
	}
	gomock.InOrder(
		s.context.EXPECT().Get(cfg.AgentName, gomock.Any()).SetArg(1, s.agent).Return(nil),
		s.agent.EXPECT().CurrentConfig().Return(s.config),
		s.config.EXPECT().Tag().Return(names.NewMachineTag("3")),
		s.machiner.EXPECT().Machine(names.NewMachineTag("3")).DoAndReturn(func(_ names.MachineTag) (stateconverter.Machine, error) {
			close(done)
			return nil, errors.New("nope")
		}),
	)
	manifold := stateconverter.Manifold(cfg)
	w, err := manifold.Start(s.context)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for calls")
	}
	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, `nope`)
}

func (s *manifoldConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agent = mocks.NewMockAgent(ctrl)
	s.config = mocks.NewMockConfig(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.machiner = mocks.NewMockMachiner(ctrl)
	return ctrl
}

type fakeLogger struct{}

func (l *fakeLogger) Debugf(format string, args ...interface{}) {}

func (l *fakeLogger) Criticalf(format string, args ...interface{}) {}

func (l *fakeLogger) Tracef(format string, args ...interface{}) {}
