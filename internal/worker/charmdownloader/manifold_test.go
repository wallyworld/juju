// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config ManifoldConfig
}

func TestManifoldConfigSuite(t *tctesting.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validConfig()
}

func validConfig() ManifoldConfig {
	return ManifoldConfig{
		APICallerName:         "api-caller",
		Logger:                loggo.GetLogger("test"),
		NewCharmDownloaderAPI: func(base.APICaller) CharmDownloaderAPI { return nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}
