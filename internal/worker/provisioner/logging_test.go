// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"errors"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type logSuite struct {
	testhelpers.LoggingSuite
	jujutesting.JujuOSEnvSuite
	logger loggo.Logger
}

func (l *logSuite) SetUpTest(c *tc.C) {
	l.LoggingSuite.SetUpTest(c)
	l.JujuOSEnvSuite.SetUpTest(c)
	l.logger = loggo.GetLogger("juju.provisioner")
}

func TestLogSuite(t *tctesting.T) {
	tc.Run(t, &logSuite{})
}

func (s *logSuite) TestFlagNotSet(c *tc.C) {
	err := errors.New("test error")
	err2 := loggedErrorStack(s.logger, err)
	c.Assert(err, tc.Equals, err2)
	//c.Assert(c.GetTestLog(), tc.Equals, "")
}

func (s *logSuite) TestFlagSet(c *tc.C) {
	s.SetFeatureFlags(feature.LogErrorStack)
	err := errors.New("test error")
	err2 := loggedErrorStack(s.logger, err)
	c.Assert(err, tc.Equals, err2)
	//expected := "ERROR juju.provisioner error stack:\ntest error"
	//c.Assert(c.GetTestLog(), tc.Contains, expected)
}
