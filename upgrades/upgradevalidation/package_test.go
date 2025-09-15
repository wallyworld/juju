// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/upgrades/upgradevalidation StatePool,State,Model
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/lxd_mock.go github.com/juju/juju/internal/provider/lxd ServerFactory,Server

var (
	CheckForDeprecatedUbuntuSeriesForModel      = checkForDeprecatedUbuntuSeriesForModel
	CheckNoWinMachinesForModel                  = checkNoWinMachinesForModel
	GetCheckUpgradeSeriesLockForModel           = getCheckUpgradeSeriesLockForModel
	GetCheckTargetVersionForModel               = getCheckTargetVersionForModel
	CheckModelMigrationModeForControllerUpgrade = checkModelMigrationModeForControllerUpgrade
	CheckMongoStatusForControllerUpgrade        = checkMongoStatusForControllerUpgrade
	CheckMongoVersionForControllerModel         = checkMongoVersionForControllerModel
	GetCheckForLXDVersion                       = getCheckForLXDVersion
	CheckForCharmStoreCharms                    = checkForCharmStoreCharms
)
