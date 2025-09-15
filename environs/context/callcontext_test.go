// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type CallContextSuite struct {
	testhelpers.IsolationSuite
}

func TestCallContextSuite(t *tctesting.T) {
	tc.Run(t, &CallContextSuite{})
}

func (s *CallContextSuite) TestCallContext(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	invalidator := NewMockModelCredentialInvalidator(ctrl)
	invalidator.EXPECT().InvalidateModelCredential("call").Return(nil)

	ctx := CallContext(invalidator)
	c.Assert(ctx, tc.NotNil)

	err := ctx.InvalidateCredential("call")
	c.Assert(err, tc.ErrorIsNil)
}
