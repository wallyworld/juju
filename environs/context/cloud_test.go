// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	stdcontext "context"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type CloudCallContextSuite struct {
	testhelpers.IsolationSuite
}

func TestCloudCallContextSuite(t *tctesting.T) {
	tc.Run(t, &CloudCallContextSuite{})
}

func (s *CloudCallContextSuite) TestCloudCallContext(c *tc.C) {
	stdctx := stdcontext.TODO()
	ctx := NewCloudCallContext(stdctx)
	c.Assert(ctx, tc.NotNil)
	c.Assert(ctx.Context, tc.Equals, stdctx)

	err := ctx.InvalidateCredential("call")
	c.Assert(errors.Is(err, errors.NotImplemented), tc.Equals, true)
}
