// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"runtime"
	tctesting "testing"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
)

type ExposeSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *ExposeSuite) SetUpTest(c *tc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, tc.NotNil)
	s.AddCleanup(func(*tc.C) { s.CmdBlockHelper.Close() })
}

func TestExposeSuite(t *tctesting.T) {
	tc.Run(t, &ExposeSuite{})
}

func runExpose(c *tc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewExposeCommand(), args...)
	return err
}

func (s *ExposeSuite) assertExposed(c *tc.C, application string) {
	svc, err := s.State.Application(application)
	c.Assert(err, tc.ErrorIsNil)
	exposed := svc.IsExposed()
	c.Assert(exposed, tc.IsTrue)
}

func (s *ExposeSuite) TestExpose(c *tc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "some-application-name"})

	err := runExpose(c, "some-application-name")
	c.Assert(err, tc.ErrorIsNil)
	s.assertExposed(c, "some-application-name")

	err = runExpose(c, "nonexistent-application")
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: `application "nonexistent-application" not found`,
		Code:    "not found",
	})
}

func (s *ExposeSuite) TestBlockExpose(c *tc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "some-application-name"})

	// Block operation
	s.BlockAllChanges(c, "TestBlockExpose")

	err := runExpose(c, "some-application-name")
	s.AssertBlocked(c, err, ".*TestBlockExpose.*")
}
