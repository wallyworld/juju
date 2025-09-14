// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/internal/testing"
)

type MachineStartupSuite struct {
	testing.BaseSuite
	manifold    dependency.Manifold
	startCalled bool
}

func TestMachineStartupSuite(t *tctesting.T) {
	tc.Run(t, &MachineStartupSuite{})
}

func (s *MachineStartupSuite) SetUpTest(c *tc.C) {
	s.startCalled = false
	s.manifold = machine.MachineStartupManifold(machine.MachineStartupConfig{
		APICallerName: "api-caller",
		MachineStartup: func(api.Connection, machine.Logger) error {
			s.startCalled = true
			return nil
		},
		Logger: noOpLogger{},
	})
}

func (s *MachineStartupSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{
		"api-caller",
	})
}

func (s *MachineStartupSuite) TestStartSuccess(c *tc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": new(mockAPIConn),
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "resource permanently unavailable")
	c.Check(s.startCalled, tc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}

type noOpLogger struct{}

func (noOpLogger) Warningf(string, ...interface{}) {}

func (noOpLogger) Criticalf(string, ...interface{}) {}

func (noOpLogger) Debugf(string, ...interface{}) {}

func (noOpLogger) Tracef(string, ...interface{}) {}
