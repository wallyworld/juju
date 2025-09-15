// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io"
	"time"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
)

func newCharmResource(c *tc.C, stub *testhelpers.Stub, name, content string, resType charmresource.Type) (resources.Resource, io.ReadCloser) {
	opened := resourcetesting.NewResource(c, stub, name, "a-application", content)
	res := opened.Resource
	res.Type = resType
	if content != "" {
		return res, opened.ReadCloser
	}
	res.Username = ""
	res.Timestamp = time.Time{}
	return res, nil
}

func newResource(c *tc.C, stub *testhelpers.Stub, name, content string) (resources.Resource, io.ReadCloser) {
	return newCharmResource(c, stub, name, content, charmresource.TypeFile)
}

func newDockerResource(c *tc.C, stub *testhelpers.Stub, name, content string) (resources.Resource, io.ReadCloser) {
	return newCharmResource(c, stub, name, content, charmresource.TypeContainerImage)
}
