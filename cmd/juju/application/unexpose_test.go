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
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testcharms"
)

type UnexposeSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *UnexposeSuite) SetUpTest(c *tc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpTest(c)

	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, tc.NotNil)
	s.AddCleanup(func(*tc.C) { s.CmdBlockHelper.Close() })
}

func TestUnexposeSuite(t *tctesting.T) {
	tc.Run(t, &UnexposeSuite{})
}

func runUnexpose(c *tc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewUnexposeCommand(), args...)
	return err
}

func (s *UnexposeSuite) assertExposed(c *tc.C, application string, expected bool) {
	svc, err := s.State.Application(application)
	c.Assert(err, tc.ErrorIsNil)
	actual := svc.IsExposed()
	c.Assert(actual, tc.Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *tc.C) {
	ch := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "jammy")

	c.Assert(err, tc.ErrorIsNil)
	curl := "local:multi-series-1"
	s.AssertApplication(c, "some-application-name", curl, 1, 0)

	err = runExpose(c, "some-application-name")
	c.Assert(err, tc.ErrorIsNil)
	s.assertExposed(c, "some-application-name", true)

	err = runUnexpose(c, "some-application-name")
	c.Assert(err, tc.ErrorIsNil)
	s.assertExposed(c, "some-application-name", false)

	err = runUnexpose(c, "nonexistent-application")
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: `application "nonexistent-application" not found`,
		Code:    "not found",
	})
}

func (s *UnexposeSuite) TestBlockUnexpose(c *tc.C) {
	ch := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "multi-series")
	err := runDeploy(c, ch, "some-application-name", "--series", "jammy")

	c.Assert(err, tc.ErrorIsNil)
	curl := "local:multi-series-1"
	s.AssertApplication(c, "some-application-name", curl, 1, 0)

	// Block operation
	s.BlockAllChanges(c, "TestBlockUnexpose")
	err = runExpose(c, "some-application-name")
	s.AssertBlocked(c, err, ".*TestBlockUnexpose.*")
}
