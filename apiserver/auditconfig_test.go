// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/internal/testing"
)

type auditConfigSuite struct {
	apiserverBaseSuite
}

func TestAuditConfigSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &auditConfigSuite{})
}

func (s *auditConfigSuite) TestUsesGetAuditConfig(c *tc.C) {
	var calls int
	s.config.GetAuditConfig = func() auditlog.Config {
		calls++
		return auditlog.Config{
			Enabled:        true,
			ExcludeMethods: set.NewStrings("Midlake.Bandits"),
		}
	}

	srv := s.newServer(c, s.config)

	auditConfig := srv.GetAuditConfig()
	c.Assert(auditConfig, tc.DeepEquals, auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("Midlake.Bandits"),
	})
	c.Assert(calls, tc.Equals, 1)
}

func (s *auditConfigSuite) TestNewServerValidatesConfig(c *tc.C) {
	s.config.GetAuditConfig = nil

	srv, err := apiserver.NewServer(s.config)
	c.Assert(err, tc.ErrorMatches, "missing GetAuditConfig not valid")
	c.Assert(srv, tc.IsNil)
}
