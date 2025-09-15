// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons_test

import (
	"context"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
)

type environSuite struct {
	statetesting.StateSuite
}

func TestEnvironSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &environSuite{})
}

func (s *environSuite) TestGetNewEnvironFunc(c *tc.C) {
	var calls int
	var callArgs environs.OpenParams
	newEnviron := func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
		calls++
		callArgs = args
		return nil, nil
	}
	_, err := stateenvirons.GetNewEnvironFunc(newEnviron)(s.Model)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(calls, tc.Equals, 1)

	cfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(callArgs.Config, tc.DeepEquals, cfg)
}

func (s *environSuite) TestCloudSpec(c *tc.C) {
	owner := s.Factory.MakeUser(c, nil).UserTag()
	emptyCredential := cloud.NewEmptyCredential()
	tag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/empty-credential")
	err := s.State.UpdateCloudCredential(tag, emptyCredential)
	c.Assert(err, tc.ErrorIsNil)

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "foo",
		CloudName:       "dummy",
		CloudCredential: tag,
		Owner:           owner,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	emptyCredential.Label = "empty-credential"
	cloudSpec, err := stateenvirons.EnvironConfigGetter{Model: m}.CloudSpec()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudSpec, tc.DeepEquals, environscloudspec.CloudSpec{
		Type:              "dummy",
		Name:              "dummy",
		Region:            "dummy-region",
		Endpoint:          "dummy-endpoint",
		IdentityEndpoint:  "dummy-identity-endpoint",
		StorageEndpoint:   "dummy-storage-endpoint",
		Credential:        &emptyCredential,
		IsControllerCloud: true,
	})
}

func (s *environSuite) TestCloudSpecForModel(c *tc.C) {
	owner := s.Factory.MakeUser(c, nil).UserTag()
	emptyCredential := cloud.NewEmptyCredential()
	tag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/empty-credential")
	err := s.State.UpdateCloudCredential(tag, emptyCredential)
	c.Assert(err, tc.ErrorIsNil)

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "foo",
		CloudName:       "dummy",
		CloudCredential: tag,
		Owner:           owner,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	emptyCredential.Label = "empty-credential"
	cloudSpec, err := stateenvirons.CloudSpecForModel(m)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudSpec, tc.DeepEquals, environscloudspec.CloudSpec{
		Type:              "dummy",
		Name:              "dummy",
		Region:            "dummy-region",
		Endpoint:          "dummy-endpoint",
		IdentityEndpoint:  "dummy-identity-endpoint",
		StorageEndpoint:   "dummy-storage-endpoint",
		Credential:        &emptyCredential,
		IsControllerCloud: true,
	})
}

func (s *environSuite) TestGetNewCAASBrokerFunc(c *tc.C) {
	var calls int
	var callArgs environs.OpenParams
	newBroker := func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
		calls++
		callArgs = args
		return nil, nil
	}
	_, err := stateenvirons.GetNewCAASBrokerFunc(newBroker)(s.Model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.Equals, 1)

	cfg, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(callArgs.Config, tc.DeepEquals, cfg)
}

type fakeBroker struct {
	caas.Broker
}

func (*fakeBroker) APIVersion() (string, error) {
	return "6.66", nil
}

func (s *environSuite) TestCloudAPIVersion(c *tc.C) {
	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		Name: "foo",
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	cred := cloud.NewNamedCredential("dummy-credential", "userpass", nil, false)
	newBrokerFunc := func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
		c.Assert(args.Cloud, tc.DeepEquals, environscloudspec.CloudSpec{
			Name:       "caascloud",
			Type:       "kubernetes",
			Credential: &cred,
		})
		return &fakeBroker{}, nil
	}

	envConfigGetter := stateenvirons.EnvironConfigGetter{Model: m, NewContainerBroker: newBrokerFunc}
	cloudSpec, err := envConfigGetter.CloudSpec()
	c.Assert(err, tc.ErrorIsNil)
	apiVersion, err := envConfigGetter.CloudAPIVersion(cloudSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiVersion, tc.Equals, "6.66")
}
