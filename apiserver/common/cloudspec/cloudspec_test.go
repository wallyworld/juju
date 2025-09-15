// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type CloudSpecSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	result   environscloudspec.CloudSpec
	authFunc common.AuthFunc
	api      cloudspec.CloudSpecAPI
}

func TestCloudSpecSuite(t *tctesting.T) {
	tc.Run(t, &CloudSpecSuite{})
}

func (s *CloudSpecSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	s.authFunc = func(tag names.Tag) bool {
		s.AddCall("Auth", tag)
		return tag == coretesting.ModelTag
	}
	s.api = s.getTestCloudSpec(apiservertesting.NewFakeNotifyWatcher())
	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{"k": "v"},
	)
	s.result = environscloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential:       &credential,
		CACertificates:   []string{coretesting.CACert},
		SkipTLSVerify:    true,
	}
}

func (s *CloudSpecSuite) getTestCloudSpec(credentialContentWatcher state.NotifyWatcher) cloudspec.CloudSpecAPI {
	return cloudspec.NewCloudSpec(
		common.NewResources(),
		func(tag names.ModelTag) (environscloudspec.CloudSpec, error) {
			s.AddCall("CloudSpec", tag)
			return s.result, s.NextErr()
		},
		func(tag names.ModelTag) (state.NotifyWatcher, error) {
			s.AddCall("WatchCloudSpec", tag)
			return apiservertesting.NewFakeNotifyWatcher(), s.NextErr()
		},
		func(tag names.ModelTag) (state.NotifyWatcher, error) {
			s.AddCall("WatchCredentialReference", tag)
			return apiservertesting.NewFakeNotifyWatcher(), s.NextErr()
		},
		func(tag names.ModelTag) (state.NotifyWatcher, error) {
			s.AddCall("WatchCredentialContent", tag)
			return credentialContentWatcher, s.NextErr()
		},
		func() (common.AuthFunc, error) {
			s.AddCall("GetAuthFunc")
			return s.authFunc, s.NextErr()
		})
}

func (s *CloudSpecSuite) TestCloudSpec(c *tc.C) {
	otherModelTag := names.NewModelTag(utils.MustNewUUID().String())
	machineTag := names.NewMachineTag("42")
	result, err := s.api.CloudSpec(params.Entities{Entities: []params.Entity{
		{coretesting.ModelTag.String()},
		{otherModelTag.String()},
		{machineTag.String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			Type:             "type",
			Name:             "name",
			Region:           "region",
			Endpoint:         "endpoint",
			IdentityEndpoint: "identity-endpoint",
			StorageEndpoint:  "storage-endpoint",
			Credential: &params.CloudCredential{
				AuthType:   "auth-type",
				Attributes: map[string]string{"k": "v"},
			},
			CACertificates: []string{coretesting.CACert},
			SkipTLSVerify:  true,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeUnauthorized,
			Message: "permission denied",
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})
	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetAuthFunc", nil},
		{"Auth", []interface{}{coretesting.ModelTag}},
		{"CloudSpec", []interface{}{coretesting.ModelTag}},
		{"Auth", []interface{}{otherModelTag}},
	})
}

func (s *CloudSpecSuite) TestWatchCloudSpecsChanges(c *tc.C) {
	otherModelTag := names.NewModelTag(utils.MustNewUUID().String())
	machineTag := names.NewMachineTag("42")
	result, err := s.api.WatchCloudSpecsChanges(params.Entities{Entities: []params.Entity{
		{coretesting.ModelTag.String()},
		{otherModelTag.String()},
		{machineTag.String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.NotifyWatchResult{{
		NotifyWatcherId: "1",
	}, {
		Error: &params.Error{
			Code:    params.CodeUnauthorized,
			Message: "permission denied",
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})
	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetAuthFunc", nil},
		{"Auth", []interface{}{coretesting.ModelTag}},
		{"WatchCloudSpec", []interface{}{coretesting.ModelTag}},
		{"WatchCredentialReference", []interface{}{coretesting.ModelTag}},
		{"WatchCredentialContent", []interface{}{coretesting.ModelTag}},
		{"Auth", []interface{}{otherModelTag}},
	})
}

func (s *CloudSpecSuite) TestWatchCloudSpecsNoCredentialContentToWatch(c *tc.C) {
	s.api = s.getTestCloudSpec(nil)
	result, err := s.api.WatchCloudSpecsChanges(params.Entities{Entities: []params.Entity{
		{coretesting.ModelTag.String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.NotifyWatchResult{{
		NotifyWatcherId: "1",
	}})
	s.CheckCalls(c, []testhelpers.StubCall{
		{"GetAuthFunc", nil},
		{"Auth", []interface{}{coretesting.ModelTag}},
		{"WatchCloudSpec", []interface{}{coretesting.ModelTag}},
		{"WatchCredentialReference", []interface{}{coretesting.ModelTag}},
		{"WatchCredentialContent", []interface{}{coretesting.ModelTag}},
	})
}

func (s *CloudSpecSuite) TestCloudSpecNilCredential(c *tc.C) {
	s.result.Credential = nil
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			Type:             "type",
			Name:             "name",
			Region:           "region",
			Endpoint:         "endpoint",
			IdentityEndpoint: "identity-endpoint",
			StorageEndpoint:  "storage-endpoint",
			Credential:       nil,
			CACertificates:   []string{coretesting.CACert},
			SkipTLSVerify:    true,
		},
	}})
}

func (s *CloudSpecSuite) TestCloudSpecGetAuthFuncError(c *tc.C) {
	expect := errors.New("bewm")
	s.SetErrors(expect)
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, tc.Equals, expect)
	c.Assert(result, tc.DeepEquals, params.CloudSpecResults{})
}

func (s *CloudSpecSuite) TestCloudSpecCloudSpecError(c *tc.C) {
	s.SetErrors(nil, errors.New("bewm"))
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CloudSpecResults{Results: []params.CloudSpecResult{{
		Error: &params.Error{Message: "bewm"},
	}}})
}
