// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/version/v2"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/cloudconfig/podcfg"
)

type UpgraderSuite struct {
}

func TestUpgraderSuite(t *tctesting.T) {
	tc.Run(t, &UpgraderSuite{})
}

func (u *UpgraderSuite) TestUpgradePodTemplateSpec(c *tc.C) {
	tests := []struct {
		ExpectedPodTemplateSpec core.PodTemplateSpec
		PodTemplateSpec         core.PodTemplateSpec
		ImagePath               string
		Version                 version.Number
	}{
		{
			ExpectedPodTemplateSpec: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: fmt.Sprintf("%s/%s:2.6.7", podcfg.JujudOCINamespace, podcfg.JujudOCIName),
						},
					},
				},
			},
			PodTemplateSpec: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Image: fmt.Sprintf("%s/%s:2.6.6", podcfg.JujudOCINamespace, podcfg.JujudOCIName),
						},
					},
				},
			},
			Version: version.MustParse("2.6.7"),
		},
	}

	for _, test := range tests {
		containers, err := upgradePodTemplateSpec(test.PodTemplateSpec.Spec.Containers, test.ImagePath, test.Version)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(test.ExpectedPodTemplateSpec.Spec.Containers[0].Image, tc.Equals, containers[0].Image)
	}
}
