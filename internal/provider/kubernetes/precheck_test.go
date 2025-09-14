// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type PrecheckSuite struct {
	BaseSuite

	callCtx context.ProviderCallContext
}

func TestPrecheckSuite(t *tctesting.T) {
	tc.Run(t, &PrecheckSuite{})
}

func (s *PrecheckSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *PrecheckSuite) TestSuccess(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("mem=4G"),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *PrecheckSuite) TestWrongSeries(c *tc.C) {
	c.Skip("disable for now because TODO(new-charms): handle systems")

	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
	})
	c.Assert(err, tc.ErrorMatches, `series "quantal" not valid`)
}

func (s *PrecheckSuite) TestUnsupportedConstraints(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("instance-type=foo"),
	})
	c.Assert(err, tc.ErrorMatches, `constraints instance-type not supported`)
}

func (s *PrecheckSuite) TestPlacementNotAllowed(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base:      corebase.MakeDefaultBase("ubuntu", "22.04"),
		Placement: "a",
	})
	c.Assert(err, tc.ErrorMatches, `placement directive "a" not valid`)
}

func (s *PrecheckSuite) TestInvalidConstraints(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("tags=foo"),
	})
	c.Assert(err, tc.ErrorMatches, `invalid node affinity constraints: foo`)
	err = s.broker.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("tags=^=bar"),
	})
	c.Assert(err, tc.ErrorMatches, `invalid node affinity constraints: \^=bar`)
}
