// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas/specs"
	k8sspces "github.com/juju/juju/internal/provider/kubernetes/specs"
	"github.com/juju/juju/internal/provider/kubernetes/specs/mocks"
	"github.com/juju/juju/internal/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

func TestTypesSuite(t *tctesting.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestParsePodSpec(c *tc.C) {

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	converter := mocks.NewMockPodSpecConverter(ctrl)
	getParser := func(specVersion specs.Version) (k8sspces.ParserType, error) {
		return func(string) (k8sspces.PodSpecConverter, error) {
			return converter, nil
		}, nil
	}

	minSpecs := &specs.PodSpec{}
	minSpecs.Version = specs.CurrentVersion
	minSpecs.Containers = []specs.ContainerSpec{
		{
			Name:  "gitlab-helper",
			Image: "gitlab-helper/latest",
			Ports: []specs.ContainerPort{
				{ContainerPort: 8080, Protocol: "TCP"},
			},
		},
	}

	gomock.InOrder(
		converter.EXPECT().Validate().Return(nil),
		converter.EXPECT().ToLatest().Return(minSpecs),
	)

	out, err := k8sspces.ParsePodSpecForTest("", getParser)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(minSpecs, tc.DeepEquals, out)
}

func (s *typesSuite) TestK8sContainersValidate(c *tc.C) {
	cs := &k8sspces.K8sContainers{}
	c.Assert(cs.Validate(), tc.ErrorMatches, `require at least one container spec`)

	c1 := k8sspces.K8sContainer{}
	c1.Name = "c1"
	c1.Image = "gitlab"
	cs = &k8sspces.K8sContainers{
		Containers: []k8sspces.K8sContainer{c1},
	}
	c.Assert(cs.Validate(), tc.ErrorIsNil)
}
