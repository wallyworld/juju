// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	tctesting "testing"

	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/gce/internal/google"
	"github.com/juju/juju/internal/testing"
)

type connConfigSuite struct {
	testing.BaseSuite
}

func TestConnConfigSuite(t *tctesting.T) {
	tc.Run(t, &connConfigSuite{})
}

func (*connConfigSuite) TestValidateValid(c *tc.C) {
	cfg := google.ConnectionConfig{
		Region:     "spam",
		HTTPClient: jujuhttp.NewClient(),
	}
	err := cfg.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (*connConfigSuite) TestValidateMissingRegion(c *tc.C) {
	cfg := google.ConnectionConfig{}
	err := cfg.Validate()

	c.Assert(err, tc.FitsTypeOf, &google.InvalidConfigValueError{})
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_REGION")
}
