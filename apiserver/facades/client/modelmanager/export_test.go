// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

func AuthCheck(c *tc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	mm.authCheck(user)
	return mm.isAdmin
}

func MockSupportedFeatures(fs assumes.FeatureSet) {
	supportedFeaturesGetter = func(stateenvirons.Model, environs.NewEnvironFunc) (assumes.FeatureSet, error) {
		return fs, nil
	}
}

func ResetSupportedFeaturesGetter() {
	supportedFeaturesGetter = stateenvirons.SupportedFeatures
}
