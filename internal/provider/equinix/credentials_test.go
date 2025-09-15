// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"os"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type credentialsSuite struct {
	testhelpers.IsolationSuite
}

func TestCredentialsSuite(t *tctesting.T) {
	tc.Run(t, &credentialsSuite{})
}

func (e credentialsSuite) TestDetectCredentials(c *tc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_AUTH_TOKEN", "tokenright")
	os.Setenv("METAL_PROJECT_ID", "project-id")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err, tc.ErrorIsNil)
}

func (e credentialsSuite) TestDetectCredentials_NoMetalToken(c *tc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_PROJECT_ID", "project-id")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err.Error(), tc.Contains, "equinix metal auth token not found")
}

func (e credentialsSuite) TestDetectCredentials_NoProject(c *tc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_AUTH_TOKEN", "metal")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err.Error(), tc.Contains, "equinix metal project ID not found")
}
