// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type formatSuite struct {
	testhelpers.CleanupSuite
}

func TestFormatSuite(t *tctesting.T) {
	tc.Run(t, &formatSuite{})
}

func (s formatSuite) TestFormatV2(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{})
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			}},
		},
	})

	c.Assert(Format(cm), tc.Equals, FormatV2)
}

func (s formatSuite) TestFormatV1EmptyManifest(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{})

	c.Assert(Format(cm), tc.Equals, FormatV1)
}

func (s formatSuite) TestFormatV1Series(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	})
	cm.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"kubernetes"},
	})

	c.Assert(Format(cm), tc.Equals, FormatV1)
}
