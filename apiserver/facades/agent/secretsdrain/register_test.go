// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/agent/secretsdrain"
	"github.com/juju/juju/apiserver/facades/agent/secretsdrain/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type drainSuite struct {
	testhelpers.IsolationSuite
}

func TestDrainSuite(t *tctesting.T) {
	tc.Run(t, &drainSuite{})
}

func (s *drainSuite) TestNewSecretManagerAPIPermissionCheck(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := mocks.NewMockAuthorizer(ctrl)
	context := mocks.NewMockContext(ctrl)
	context.EXPECT().Auth().Return(authorizer).AnyTimes()
	authorizer.EXPECT().AuthUnitAgent().Return(false)
	authorizer.EXPECT().AuthApplicationAgent().Return(false)

	_, err := secretsdrain.NewSecretsDrainAPI(context)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
