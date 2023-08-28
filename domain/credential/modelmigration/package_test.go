// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/credential/modelmigration Coordinator,ImportService,ExportService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
