// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasenvironupgrader"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (*WorkerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	_, err := caasenvironupgrader.NewWorker(caasenvironupgrader.Config{})
	c.Assert(err, tc.ErrorMatches, "nil Facade not valid")
}

func (*WorkerSuite) TestNewWorker(c *tc.C) {
	mockFacade := mockFacade{}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := caasenvironupgrader.NewWorker(caasenvironupgrader.Config{
		Facade:       &mockFacade,
		GateUnlocker: &mockGateUnlocker,
		ModelTag:     coretesting.ModelTag,
	})
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testhelpers.StubCall{
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Available, "", nilData}},
	})
	mockGateUnlocker.CheckCallNames(c, "Unlock")
}

type mockFacade struct {
	testhelpers.Stub
}

var nilData map[string]interface{}

func (f *mockFacade) SetModelStatus(tag names.ModelTag, status status.Status, info string, data map[string]interface{}) error {
	f.MethodCall(f, "SetModelStatus", tag, status, info, data)
	return f.NextErr()
}

type mockGateUnlocker struct {
	testhelpers.Stub
}

func (g *mockGateUnlocker) Unlock() {
	g.MethodCall(g, "Unlock")
	g.PopNoErr()
}
