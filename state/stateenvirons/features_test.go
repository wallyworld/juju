// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type featuresSuite struct {
	testhelpers.IsolationSuite
}

func TestFeaturesSuite(t *tctesting.T) {
	tc.Run(t, &featuresSuite{})
}

func (s *featuresSuite) TestSupportedFeaturesWithIncompatibleEnviron(c *tc.C) {
	defer func(getter func(Model) (environs.Environ, error)) {
		iaasEnvironGetter = getter
	}(iaasEnvironGetter)
	iaasEnvironGetter = func(Model) (environs.Environ, error) {
		// Not supporting environs.SupportedFeaturesEnumerator
		return nil, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeIAAS,
	}
	fs, err := SupportedFeatures(m, nil)
	c.Assert(err, tc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
	}

	c.Assert(fs.AsList(), tc.DeepEquals, exp)
}

func (s *featuresSuite) TestSupportedFeaturesWithCompatibleIAASEnviron(c *tc.C) {
	defer func(getter func(Model) (environs.Environ, error)) {
		iaasEnvironGetter = getter
	}(iaasEnvironGetter)
	iaasEnvironGetter = func(Model) (environs.Environ, error) {
		return mockIAASEnvironWithFeatures{}, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeIAAS,
	}
	fs, err := SupportedFeatures(m, nil)
	c.Assert(err, tc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
		// The following feature was reported by the environ.
		{Name: "web-scale"},
	}

	c.Assert(fs.AsList(), tc.DeepEquals, exp)
}

func (s *featuresSuite) TestSupportedFeaturesWithCompatibleCAASEnviron(c *tc.C) {
	defer func(getter func(Model) (caas.Broker, error)) {
		caasBrokerGetter = getter
	}(caasBrokerGetter)
	caasBrokerGetter = func(Model) (caas.Broker, error) {
		return mockCAASEnvironWithFeatures{}, nil
	}

	jujuVersion := version.MustParse("2.9.17")
	m := mockModel{
		jujuVersion: jujuVersion,
		modelType:   state.ModelTypeCAAS,
	}
	fs, err := SupportedFeatures(m, nil)
	c.Assert(err, tc.ErrorIsNil)

	exp := []assumes.Feature{
		{
			Name:        "juju",
			Description: "the version of Juju used by the model",
			Version:     &jujuVersion,
		},
		// The following feature was reported by the environ.
		{Name: "k8s-api"},
	}

	c.Assert(fs.AsList(), tc.DeepEquals, exp)
}

type mockModel struct {
	Model

	jujuVersion version.Number
	modelType   state.ModelType
}

func (m mockModel) Config() (*config.Config, error) {
	return config.New(config.NoDefaults,
		coretesting.FakeConfig().Merge(coretesting.Attrs{
			config.AgentVersionKey: m.jujuVersion.String(),
		}),
	)
}

func (m mockModel) Type() state.ModelType {
	return m.modelType
}

type mockIAASEnvironWithFeatures struct {
	environs.Environ
}

func (mockIAASEnvironWithFeatures) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "web-scale"})
	return fs, nil
}

type mockCAASEnvironWithFeatures struct {
	caas.Broker
}

func (mockCAASEnvironWithFeatures) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "k8s-api"})
	return fs, nil
}
