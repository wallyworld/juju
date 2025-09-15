// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type cloudImageMetadataSuite struct{}

func TestCloudImageMetadataSuite(t *tctesting.T) {
	tc.Run(t, &cloudImageMetadataSuite{})
}

func (s *cloudImageMetadataSuite) TestCloudImageMetadataDocFields(c *tc.C) {
	ignored := set.NewStrings("Id")
	migrated := set.NewStrings(
		"Stream",
		"Region",
		"Version",
		"Arch",
		"VirtType",
		"RootStorageType",
		"RootStorageSize",
		"Source",
		"Priority",
		"ImageId",
		"DateCreated",
		"ExpireAt",
	)
	fields := migrated.Union(ignored)
	expected := testing.GetExportedFields(imagesMetadataDoc{})
	unknown := expected.Difference(fields)
	removed := fields.Difference(expected)
	// If this test fails, it means that extra fields have been added to the
	// doc without thinking about the migration implications.
	c.Check(unknown, tc.HasLen, 0)
	c.Assert(removed, tc.HasLen, 0)
}
