// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestCaasagentSuite(t *tctesting.T) {
	tc.Run(t, &caasagentSuite{})
}

type caasagentSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
}

func (s *caasagentSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
}

func (s *caasagentSuite) TestPermission(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("someapp"),
	}
	_, err := caasagent.NewStateFacadeV2(facadetest.Context{Auth_: s.authorizer})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
