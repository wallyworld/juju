// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	tctesting "testing"

	"go.uber.org/mock/gomock"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type charmsS3ClientSuite struct {
	session *MockSession
}

func TestCharmsS3ClientSuite(t *tctesting.T) {
	tc.Run(t, &charmsS3ClientSuite{})
}

func (s *charmsS3ClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.session = NewMockSession(ctrl)

	return ctrl
}

func (s *charmsS3ClientSuite) TestGetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().GetObject(gomock.Any(), "model-"+coretesting.ModelTag.Id(), "charms/somecharm-abcd0123")

	cli := NewCharmsS3Client(s.session)
	cli.GetCharm(c.Context(), coretesting.ModelTag.Id(), "somecharm-abcd0123")
}
