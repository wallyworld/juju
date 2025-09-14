// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

func TestCaasmodelconfigmanagerSuite(t *tctesting.T) {
	tc.Run(t, &caasmodelconfigmanagerSuite{})
}

type caasmodelconfigmanagerSuite struct {
	testhelpers.IsolationSuite
}

func (s *caasmodelconfigmanagerSuite) TestAuth(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := mocks.NewMockContext(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)

	gomock.InOrder(
		ctx.EXPECT().Auth().Return(authorizer),
		authorizer.EXPECT().AuthController().Return(false),
	)

	_, err := caasmodelconfigmanager.NewFacade(ctx)
	c.Assert(err, tc.ErrorMatches, `permission denied`)
}
