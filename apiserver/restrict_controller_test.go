// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
)

type restrictControllerSuite struct {
	testing.BaseSuite
	root rpc.Root
}

func TestRestrictControllerSuite(t *tctesting.T) {
	tc.Run(t, &restrictControllerSuite{})
}

func (s *restrictControllerSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingControllerOnlyRoot()
}

func (s *restrictControllerSuite) TestAllowed(c *tc.C) {
	s.assertMethod(c, "AllModelWatcher", 4, "Next")
	s.assertMethod(c, "AllModelWatcher", 4, "Stop")
	s.assertMethod(c, "ModelManager", modelManagerFacadeVersion, "CreateModel")
	s.assertMethod(c, "ModelManager", modelManagerFacadeVersion, "ListModels")
	s.assertMethod(c, "Pinger", pingerFacadeVersion, "Ping")
	s.assertMethod(c, "Bundle", 6, "GetChanges")
	s.assertMethod(c, "HighAvailability", highAvailabilityFacadeVersion, "EnableHA")
	s.assertMethod(c, "ApplicationOffers", 4, "ApplicationOffers")
}

func (s *restrictControllerSuite) TestNotAllowed(c *tc.C) {
	caller, err := s.root.FindMethod("Client", clientFacadeVersion, "FullStatus")
	c.Assert(err, tc.ErrorMatches, `facade "Client" not supported for controller API connection`)
	c.Assert(errors.IsNotSupported(err), tc.IsTrue)
	c.Assert(caller, tc.IsNil)
}

func (s *restrictControllerSuite) assertMethod(c *tc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, tc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}
