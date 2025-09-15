// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type setApplicationBaseSuite struct {
	testhelpers.IsolationSuite
	mockApplicationAPI *mockSetApplicationBaseAPI
}

func TestSetApplicationBaseSuite(t *tctesting.T) {
	tc.Run(t, &setApplicationBaseSuite{})
}

func (s *setApplicationBaseSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockApplicationAPI = &mockSetApplicationBaseAPI{Stub: &testhelpers.Stub{}}
}

func (s *setApplicationBaseSuite) runSetApplicationBase(c *tc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	return cmdtesting.RunCommand(c, application.NewSetApplicationBaseCommandForTest(s.mockApplicationAPI, store), args...)
}

func (s *setApplicationBaseSuite) TestSetSeriesApplicationGoodPath(c *tc.C) {
	_, err := s.runSetApplicationBase(c, "ghost", "ubuntu@20.04")
	c.Assert(err, tc.ErrorIsNil)
	s.mockApplicationAPI.CheckCall(c, 0, "UpdateApplicationBase", "ghost", corebase.MustParseBaseFromString("ubuntu@20.04"), false)
}

func (s *setApplicationBaseSuite) TestNoArguments(c *tc.C) {
	_, err := s.runSetApplicationBase(c)
	c.Assert(err, tc.ErrorMatches, "application name and base required")
}

func (s *setApplicationBaseSuite) TestArgumentsSeriesOnly(c *tc.C) {
	_, err := s.runSetApplicationBase(c, "ghost")
	c.Assert(err, tc.ErrorMatches, "no base specified")
}

func (s *setApplicationBaseSuite) TestArgumentsApplicationOnly(c *tc.C) {
	_, err := s.runSetApplicationBase(c, "ubuntu@20.04")
	c.Assert(err, tc.ErrorMatches, "no application name")
}

func (s *setApplicationBaseSuite) TestTooManyArguments(c *tc.C) {
	_, err := s.runSetApplicationBase(c, "ghost", "ubuntu@20.04", "something else")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["something else"\]`, tc.Commentf("details: %s", errors.Details(err)))
}

type mockSetApplicationBaseAPI struct {
	*testhelpers.Stub
}

func (a *mockSetApplicationBaseAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockSetApplicationBaseAPI) UpdateApplicationBase(appName string, series corebase.Base, force bool) error {
	a.MethodCall(a, "UpdateApplicationBase", appName, series, force)
	return a.NextErr()
}
