// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"strings"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"
)

func newFingerprint(c *tc.C, data string) charmresource.Fingerprint {
	reader := strings.NewReader(data)
	fp, err := charmresource.GenerateFingerprint(reader)
	c.Assert(err, tc.ErrorIsNil)
	return fp
}

func newFullCharmResource(c *tc.C, name string) charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeFile,
			Path:        name + ".tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: newFingerprint(c, name),
	}
}
