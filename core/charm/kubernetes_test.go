// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type kubernetesSuite struct {
	testhelpers.CleanupSuite
}

func TestKubernetesSuite(t *tctesting.T) {
	tc.Run(t, &kubernetesSuite{})
}

func (s *kubernetesSuite) TestMetadataV1NoKubernetes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{Series: []string{"bionic"}}).MinTimes(2)
	cm.EXPECT().Manifest().Return(nil).AnyTimes()

	c.Assert(IsKubernetes(cm), tc.IsFalse)
}

func (s *kubernetesSuite) TestMetadataV1Kubernetes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{Series: []string{"kubernetes"}}).MinTimes(2)
	cm.EXPECT().Manifest().Return(nil).AnyTimes()

	c.Assert(IsKubernetes(cm), tc.IsTrue)
}

func (s *kubernetesSuite) TestMetadataV2NoKubernetes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()
	cm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{
		{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk:  "stable",
				Track: "20.04",
			},
		},
	}}).AnyTimes()

	c.Assert(IsKubernetes(cm), tc.IsFalse)
}

func (s *kubernetesSuite) TestMetadataV2Kubernetes(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{
		Containers: map[string]charm.Container{
			"redis": {Resource: "redis-container-resource"},
		},
		Resources: map[string]charmresource.Meta{
			"redis-container-resource": {
				Name: "redis-container",
				Type: charmresource.TypeContainerImage,
			},
		},
	}).AnyTimes()
	cm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{
		{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk:  "stable",
				Track: "20.04",
			},
		},
	}}).AnyTimes()

	c.Assert(IsKubernetes(cm), tc.IsTrue)
}
