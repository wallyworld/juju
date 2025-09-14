// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/lxd"
)

type instanceSuite struct {
	lxd.BaseSuite
}

func TestInstanceSuite(t *tctesting.T) {
	tc.Run(t, &instanceSuite{})
}

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	inst := lxd.NewInstance(s.Container, s.Env)

	c.Check(lxd.ExposeInstContainer(inst), tc.Equals, s.Container)
	c.Check(lxd.ExposeInstEnv(inst), tc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *tc.C) {
	id := s.Instance.Id()

	c.Check(id, tc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *tc.C) {
	instanceStatus := s.Instance.Status(context.NewEmptyCloudCallContext())

	c.Check(instanceStatus.Message, tc.Equals, "Running")
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	addresses, err := s.Instance.Addresses(context.NewEmptyCloudCallContext())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(addresses, tc.DeepEquals, s.Addresses)
}
