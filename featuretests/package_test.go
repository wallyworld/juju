// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

func init() {
	// Initialize all suites here.
	tc.Suite(&apiLoggerSuite{})
	tc.Suite(&dblogSuite{})
	tc.Suite(&dumpLogsCommandSuite{})
	tc.Suite(&undertakerSuite{})
	tc.Suite(&debugLogDbSuite1{})
	tc.Suite(&debugLogDbSuite2{})
	tc.Suite(&toolsDownloadSuite{})
	tc.Suite(&toolsWithMacaroonsSuite{})
	tc.Suite(&CredentialManagerSuite{})
	tc.Suite(&ControllerSuite{})
}

func TestPackage(t *testing.T) {
	coretesting.MgoSSLTestPackage(t)
}
