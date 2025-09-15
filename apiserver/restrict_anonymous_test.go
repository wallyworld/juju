// Copyright 2017 Canonical Ltd.
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

type restrictAnonymousSuite struct {
	testing.BaseSuite
	root rpc.Root
}

func TestRestrictAnonymousSuite(t *tctesting.T) {
	tc.Run(t, &restrictAnonymousSuite{})
}

func (s *restrictAnonymousSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.root = apiserver.TestingAnonymousRoot()
}

func (s *restrictAnonymousSuite) TestAllowed(c *tc.C) {
	s.assertMethod(c, "CrossModelRelations", 2, "RegisterRemoteRelations")
}

func (s *restrictAnonymousSuite) TestNotAllowed(c *tc.C) {
	caller, err := s.root.FindMethod("Client", clientFacadeVersion, "FullStatus")
	c.Assert(err, tc.ErrorMatches, `facade "Client" not supported for anonymous API connections`)
	c.Assert(errors.IsNotSupported(err), tc.IsTrue)
	c.Assert(caller, tc.IsNil)
}

func (s *restrictAnonymousSuite) assertMethod(c *tc.C, facadeName string, version int, method string) {
	caller, err := s.root.FindMethod(facadeName, version, method)
	c.Check(err, tc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}
