// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	tctesting "testing"

	"github.com/juju/mgo/v3/txn"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/spaces"
)

type SpaceRemoveSuite struct {
	space *spaces.MockRemoveSpace
}

func TestSpaceRemoveSuite(t *tctesting.T) {
	tc.Run(t, &SpaceRemoveSuite{})
}

func (s *SpaceRemoveSuite) TestSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	removeSpaceOps := []txn.Op{{
		C:      "1",
		Id:     "2",
		Remove: true,
	}, {
		C:      "1",
		Remove: false,
	}}

	s.space.EXPECT().RemoveSpaceOps().Return(removeSpaceOps, nil)

	op := spaces.NewRemoveSpaceOp(s.space)

	ops, err := op.Build(0)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ops, tc.HasLen, 2)
}

func (s *SpaceRemoveSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.space = spaces.NewMockRemoveSpace(ctrl)

	return ctrl
}
