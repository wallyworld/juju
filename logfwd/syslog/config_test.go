// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/logfwd/syslog"
)

type ConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestConfigSuite(t *tctesting.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) TestRawValidateFull(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateWithoutPort(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateZeroValue(c *tc.C) {
	var cfg syslog.RawConfig
	err := cfg.Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateMissingHost(c *tc.C) {
	cfg := syslog.RawConfig{
		Enabled:    true,
		Host:       "",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `Host "" not valid`)
}

func (s *ConfigSuite) TestRawValidateMissingHostNotEnabled(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateMissingHostname(c *tc.C) {
	cfg := syslog.RawConfig{
		Enabled:    true,
		Host:       ":9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `Host ":9876" not valid`)
}

func (s *ConfigSuite) TestRawValidateMissingCACert(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     "",
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing CA certificate: no certificates found`)
}

func (s *ConfigSuite) TestRawValidateBadCACert(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     invalidCert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing CA certificate: x509: malformed certificate`)
}

func (s *ConfigSuite) TestRawValidateBadCACertFormat(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     "abc",
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing CA certificate: no certificates found`)
}

func (s *ConfigSuite) TestRawValidateMissingCert(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: "",
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in certificate input`)
}

func (s *ConfigSuite) TestRawValidateBadCert(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: invalidCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: x509: malformed certificate`)
}

func (s *ConfigSuite) TestRawValidateBadCertFormat(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: "abc",
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in certificate input`)
}

func (s *ConfigSuite) TestRawValidateMissingKey(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  "",
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in key input`)
}

func (s *ConfigSuite) TestRawValidateBadKey(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  invalidKey,
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to parse private key`)
}

func (s *ConfigSuite) TestRawValidateBadKeyFormat(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  "abc",
	}

	err := cfg.Validate()

	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in key input`)
}

func (s *ConfigSuite) TestRawValidateCertKeyMismatch(c *tc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.CAKey,
	}

	err := cfg.Validate()
	c.Check(err, tc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: private key does not match public key`)
}

var invalidCert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]

var invalidKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END RSA PRIVATE KEY-----
`[1:]
