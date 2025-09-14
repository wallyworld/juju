// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
)

type verifySuite struct{}

func TestVerifySuite(t *tctesting.T) {
	tc.Run(t, &verifySuite{})
}

func (s *verifySuite) TestNextOpNotInstallNorUpgrade(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.RunAction},
	}
	remote := remotestate.Snapshot{}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileNotRequired(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: false,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpInstallProfileRequiredEmptyName(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Install},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpMisMatchCharmRevisions(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-74",
		CharmURL:             "ch:wordpress-75",
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrDoNotProceed)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNextOpMatchingCharmRevisions(c *tc.C) {
	local := resolver.LocalState{
		State: operation.State{Kind: operation.Upgrade},
	}
	remote := remotestate.Snapshot{
		CharmProfileRequired: true,
		LXDProfileName:       "juju-wordpress-75",
		CharmURL:             "ch:wordpress-75",
	}
	res := newVerifyCharmProfileResolver()

	op, err := res.NextOp(local, remote, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.IsNil)
}

func (s *verifySuite) TestNewResolverCAAS(c *tc.C) {
	r := verifycharmprofile.NewResolver(&fakelogger{}, model.CAAS)
	op, err := r.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, nil)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
	c.Assert(op, tc.ErrorIsNil)
}

func newVerifyCharmProfileResolver() resolver.Resolver {
	return verifycharmprofile.NewResolver(&fakelogger{}, model.IAAS)
}

type fakelogger struct{}

func (*fakelogger) Debugf(string, ...interface{}) {}

func (*fakelogger) Tracef(string, ...interface{}) {}
