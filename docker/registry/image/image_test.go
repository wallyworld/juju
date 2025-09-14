// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image_test

import (
	"encoding/json"
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/version/v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/docker/registry/image"
	"github.com/juju/juju/internal/testhelpers"
)

type imageSuite struct {
	testhelpers.IsolationSuite
}

func TestImageSuite(t *tctesting.T) {
	tc.Run(t, &imageSuite{})
}

func (s *imageSuite) TestImageInfo(c *tc.C) {
	imageInfo := image.NewImageInfo(version.MustParse("2.9.13"))
	dataJSON, err := json.Marshal(imageInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(dataJSON), tc.DeepEquals, `"2.9.13"`)

	dataYAML, err := yaml.Marshal(imageInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(dataYAML), tc.DeepEquals, `2.9.13
`)

	imageInfo = image.NewImageInfo(version.Zero)
	c.Assert(imageInfo.AgentVersion(), tc.DeepEquals, version.Zero)
	err = json.Unmarshal(dataJSON, &imageInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), tc.DeepEquals, `2.9.13`)

	imageInfo = image.NewImageInfo(version.Zero)
	c.Assert(imageInfo.AgentVersion(), tc.DeepEquals, version.Zero)
	err = yaml.Unmarshal(dataYAML, &imageInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), tc.DeepEquals, `2.9.13`)
}
