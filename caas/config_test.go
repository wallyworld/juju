// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	tctesting "testing"

	"github.com/juju/schema"
	"github.com/juju/tc"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/testing"
)

var baseFields = environschema.Fields{
	caas.JujuExternalHostNameKey: {
		Description: "the external hostname of an exposed application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	caas.JujuApplicationPath: {
		Description: "the relative http path used to access an application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}

var baseDefaults = schema.Defaults{
	caas.JujuApplicationPath: "/",
}

type ConfigSuite struct {
	testing.BaseSuite
}

func TestConfigSuite(t *tctesting.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) TestConfigSchemaNoProviderFields(c *tc.C) {
	fields, err := caas.ConfigSchema(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fields, tc.DeepEquals, baseFields)
}

func (s *ConfigSuite) TestConfigSchemaProviderFields(c *tc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	fields, err := caas.ConfigSchema(extraFields)
	c.Assert(err, tc.ErrorIsNil)

	expectedFields := make(environschema.Fields)
	for name, f := range baseFields {
		expectedFields[name] = f
	}
	for name, f := range extraFields {
		expectedFields[name] = f
	}
	c.Assert(fields, tc.DeepEquals, expectedFields)
}

func (s *ConfigSuite) TestConfigSchemaProviderFieldsConflict(c *tc.C) {
	extraFields := environschema.Fields{
		"juju-external-hostname": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	_, err := caas.ConfigSchema(extraFields)
	c.Assert(err, tc.ErrorMatches, `config field "juju-external-hostname" clashes with common config`)
}

func (s *ConfigSuite) TestConfigDefaultsNoProviderDefaults(c *tc.C) {
	defaults := caas.ConfigDefaults(nil)
	c.Assert(defaults, tc.DeepEquals, baseDefaults)
}

func (s *ConfigSuite) TestConfigSchemaProviderDefaults(c *tc.C) {
	extraDefaults := schema.Defaults{
		"extra": "extra default",
	}
	defaults := caas.ConfigDefaults(extraDefaults)

	expectedDefaults := make(schema.Defaults)
	for name, d := range baseDefaults {
		expectedDefaults[name] = d
	}
	for name, d := range defaults {
		expectedDefaults[name] = d
	}
	c.Assert(defaults, tc.DeepEquals, expectedDefaults)
}
