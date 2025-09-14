// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	tctesting "testing"

	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/ec2"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type ProviderSuite struct {
	testhelpers.IsolationSuite
	spec     environscloudspec.CloudSpec
	provider environs.EnvironProvider
}

func TestProviderSuite(t *tctesting.T) {
	tc.Run(t, &ProviderSuite{})
}

func (s *ProviderSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "foo",
			"secret-key": "bar",
		},
	)
	s.spec = environscloudspec.CloudSpec{
		Type:       "ec2",
		Name:       "aws",
		Region:     "us-east-1",
		Credential: &credential,
	}

	provider, err := environs.Provider("ec2")
	c.Assert(err, tc.ErrorIsNil)
	s.provider = provider
}

func (s *ProviderSuite) TestOpen(c *tc.C) {
	env, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)
}

func (s *ProviderSuite) TestOpenMissingCredential(c *tc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *ProviderSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *ProviderSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *ProviderSuite) TestVerifyCredentialsErrs(c *tc.C) {
	err := ec2.VerifyCredentials(context.NewEmptyCloudCallContext())
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsFalse)
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorIgnoresNil(c *tc.C) {
	err := ec2.MaybeConvertCredentialError(nil, context.NewEmptyCloudCallContext())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorConvertsCredentialRelatedFailures(c *tc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		err := ec2.MaybeConvertCredentialError(
			&smithy.GenericAPIError{Code: code}, context.NewEmptyCloudCallContext())
		c.Assert(err, tc.NotNil)
		c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsTrue)
	}
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorNotInvalidCredential(c *tc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		err := ec2.MaybeConvertCredentialError(
			&smithy.GenericAPIError{Code: code}, context.NewEmptyCloudCallContext())
		c.Assert(err, tc.NotNil)
		c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsFalse)
	}
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorHandlesOtherProviderErrors(c *tc.C) {
	// Any other ec2.Error is returned unwrapped.
	err := ec2.MaybeConvertCredentialError(&smithy.GenericAPIError{Code: "DryRunOperation"}, context.NewEmptyCloudCallContext())
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), tc.IsFalse)
}

func (s *ProviderSuite) TestConvertedCredentialError(c *tc.C) {
	// Trace() will keep error type
	inner := ec2.MaybeConvertCredentialError(
		&smithy.GenericAPIError{Code: "Blocked"}, context.NewEmptyCloudCallContext())
	traced := errors.Trace(inner)
	c.Assert(traced, tc.NotNil)
	c.Assert(errors.Is(traced, common.ErrorCredentialNotValid), tc.IsTrue)

	// Annotate() will keep error type
	annotated := errors.Annotate(inner, "annotation")
	c.Assert(annotated, tc.NotNil)
	c.Assert(errors.Is(annotated, common.ErrorCredentialNotValid), tc.IsTrue)

	// Running a CredentialNotValid through conversion call again is a no-op.
	again := ec2.MaybeConvertCredentialError(inner, context.NewEmptyCloudCallContext())
	c.Assert(again, tc.NotNil)
	c.Assert(errors.Is(again, common.ErrorCredentialNotValid), tc.IsTrue)
	c.Assert(again.Error(), tc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")

	// Running an annotated CredentialNotValid through conversion call again is a no-op too.
	againAnotated := ec2.MaybeConvertCredentialError(annotated, context.NewEmptyCloudCallContext())
	c.Assert(againAnotated, tc.NotNil)
	c.Assert(errors.Is(againAnotated, common.ErrorCredentialNotValid), tc.IsTrue)
	c.Assert(againAnotated.Error(), tc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")
}
