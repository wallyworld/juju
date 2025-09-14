// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"golang.org/x/crypto/acme"

	"github.com/juju/juju/internal/worker/httpserver"
)

type tlsStateFixture struct {
	stateFixture
	cert *tls.Certificate
}

func (s *tlsStateFixture) SetUpTest(c *tc.C) {
	s.stateFixture.SetUpTest(c)
	s.cert = &tls.Certificate{
		Leaf: &x509.Certificate{
			DNSNames: []string{
				"testing1.invalid",
				"testing2.invalid",
				"testing3.invalid",
			},
		},
	}
}

type TLSStateSuite struct {
	tlsStateFixture
}

func TestTLSStateSuite(t *tctesting.T) {
	tc.Run(t, &TLSStateSuite{})
}

func (s *TLSStateSuite) TestNewTLSConfig(c *tc.C) {
	tlsConfig, err := httpserver.NewTLSConfig(
		s.State,
		testSNIGetter(s.cert),
		loggo.GetLogger("test"),
	)
	c.Assert(err, tc.ErrorIsNil)

	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: "anything.invalid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cert, tc.Equals, s.cert)
}

type TLSStateAutocertSuite struct {
	tlsStateFixture
	autocertQueried bool
}

func TestTLSStateAutocertSuite(t *tctesting.T) {
	tc.Run(t, &TLSStateAutocertSuite{})
}

func (s *TLSStateAutocertSuite) SetUpSuite(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.autocertQueried = true
		http.Error(w, "burp", http.StatusUnavailableForLegalReasons)
	}))
	s.ControllerConfig = map[string]interface{}{
		"autocert-dns-name": "public.invalid",
		"autocert-url":      server.URL,
	}
	s.tlsStateFixture.SetUpSuite(c)
	s.AddCleanup(func(c *tc.C) { server.Close() })
}

func (s *TLSStateAutocertSuite) SetUpTest(c *tc.C) {
	s.tlsStateFixture.SetUpTest(c)
	s.autocertQueried = false
}

func (s *TLSStateAutocertSuite) TestAutocertExceptions(c *tc.C) {
	tlsConfig, err := httpserver.NewTLSConfig(
		s.State,
		testSNIGetter(s.cert),
		loggo.GetLogger("test"),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "127.0.0.1")
	s.testGetCertificate(c, tlsConfig, "juju-apiserver")
	s.testGetCertificate(c, tlsConfig, "testing1.invalid")
	c.Assert(s.autocertQueried, tc.IsFalse)
}

func (s *TLSStateAutocertSuite) TestAutocert(c *tc.C) {
	tlsConfig, err := httpserver.NewTLSConfig(
		s.State,
		testSNIGetter(s.cert),
		loggo.GetLogger("test"),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "public.invalid")
	c.Assert(s.autocertQueried, tc.IsTrue)
	c.Assert(tlsConfig.NextProtos, tc.DeepEquals, []string{"h2", "http/1.1", acme.ALPNProto})
}

func (s *TLSStateAutocertSuite) TestAutocertHostPolicy(c *tc.C) {
	tlsConfig, err := httpserver.NewTLSConfig(
		s.State,
		testSNIGetter(s.cert),
		loggo.GetLogger("test"),
	)
	c.Assert(err, tc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "always.invalid")
	c.Assert(s.autocertQueried, tc.IsFalse)
}

func (s *TLSStateAutocertSuite) testGetCertificate(c *tc.C, tlsConfig *tls.Config, serverName string) {
	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: serverName,
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("server name %q", serverName))
	// NOTE(axw) we always expect to get back s.cert, because we don't have
	// a functioning autocert test server. We do check that we attempt to
	// query the autocert server, but that's as far as we test here.
	c.Assert(cert, tc.Equals, s.cert, tc.Commentf("server name %q", serverName))
}
