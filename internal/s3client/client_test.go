// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"io"
	"strings"
	tctesting "testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

type s3ClientSuite struct {
	s3Client *MockS3Client
}

func TestS3ClientSuite(t *tctesting.T) {
	tc.Run(t, &s3ClientSuite{})
}

func (s *s3ClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.s3Client = NewMockS3Client(ctrl)

	return ctrl
}

func (s *s3ClientSuite) TestGetObject(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.s3Client.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("object"),
	}, gomock.Any()).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader("blob")),
	}, nil)

	cli := objectsClient{
		client: s.s3Client,
		logger: loggo.GetLogger("juju.testing.s3client"),
	}
	resp, err := cli.GetObject(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIsNil)

	blob, err := io.ReadAll(resp)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(blob), tc.Equals, "blob")
}
