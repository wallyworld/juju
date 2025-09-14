// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	"github.com/juju/tc"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades/upgradevalidation"
	"github.com/juju/juju/upgrades/upgradevalidation/mocks"
)

func TestUpgradeValidationSuite(t *tctesting.T) {
	tc.Run(t, &upgradeValidationSuite{})
}

type upgradeValidationSuite struct {
	testhelpers.IsolationSuite
}

func (s *upgradeValidationSuite) TestModelUpgradeBlockers(c *tc.C) {
	blockers1 := upgradevalidation.NewModelUpgradeBlockers(
		"controller",
		*upgradevalidation.NewBlocker("model migration is in process"),
		*upgradevalidation.NewBlocker("unexpected upgrade series lock found"),
	)
	for i := 1; i < 5; i++ {
		blockers := upgradevalidation.NewModelUpgradeBlockers(
			fmt.Sprintf("model-%d", i),
			*upgradevalidation.NewBlocker("unexpected upgrade series lock found"),
			*upgradevalidation.NewBlocker("model migration is in process"),
		)
		blockers1.Join(blockers)
	}
	c.Assert(blockers1.String(), tc.Equals, `
"controller":
- model migration is in process
- unexpected upgrade series lock found
"model-1":
- unexpected upgrade series lock found
- model migration is in process
"model-2":
- unexpected upgrade series lock found
- model migration is in process
"model-3":
- unexpected upgrade series lock found
- model migration is in process
"model-4":
- unexpected upgrade series lock found
- model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck("", statePool, st, model,
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return nil, errors.New("server is unreachable")
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, tc.IsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	model.EXPECT().Owner().Return(names.NewUserTag("admin"))
	model.EXPECT().Name().Return("model-1")

	checker := upgradevalidation.NewModelUpgradeCheck(coretesting.ModelTag.Id(), statePool, st, model,
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("unexpected upgrade series lock found"), nil
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers.String(), tc.Equals, `
"admin/model-1":
- model migration is in process
- unexpected upgrade series lock found`[1:])
}

func (s *upgradeValidationSuite) TestCheckNoWinMachinesForModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	gomock.InOrder(
		st.EXPECT().MachineCountForBase(makeBases("windows", winVersions)).Return(nil, nil),
		st.EXPECT().MachineCountForBase(makeBases("windows", winVersions)).Return(map[string]int{"win10": 1, "win7": 2}, nil),
	)

	blocker, err := upgradevalidation.CheckNoWinMachinesForModel("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.CheckNoWinMachinesForModel("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `the model hosts deprecated windows machine(s): win10(1) win7(2)`)
}

func (s *upgradeValidationSuite) TestCheckForDeprecatedUbuntuSeriesForModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", unsupportedUbuntuVersions)).Return(map[string]int{"xenial": 1, "vivid": 2, "trusty": 3}, nil)

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `the model hosts deprecated ubuntu machine(s): trusty(3) vivid(2) xenial(1)`)
}

func (s *upgradeValidationSuite) TestGetCheckUpgradeSeriesLockForModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	gomock.InOrder(
		st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
		st.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
		st.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
	)

	blocker, err := upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(true)("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `unexpected upgrade series lock found`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForControllerModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.30"),
	})

	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.29"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, tc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, tc.ErrorMatches, `upgrading controller to "4.1.1" is not supported from "2.9.31"`)
	c.Assert(blocker, tc.IsNil)
}

func (s *upgradeValidationSuite) TestCheckModelMigrationModeForControllerUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().MigrationMode().Return(state.MigrationModeNone),
		model.EXPECT().MigrationMode().Return(state.MigrationModeImporting),
		model.EXPECT().MigrationMode().Return(state.MigrationModeExporting),
	)

	blocker, err := upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `model is under "importing" mode, upgrade blocked`)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `model is under "exporting" mode, upgrade blocked`)
}

func (s *upgradeValidationSuite) TestCheckMongoStatusForControllerUpgrade(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	gomock.InOrder(
		st.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					Address: "1.1.1.1",
					State:   replicaset.PrimaryState,
				},
				{
					Id:      2,
					Address: "2.2.2.2",
					State:   replicaset.SecondaryState,
				},
				{
					Id:      3,
					Address: "3.3.3.3",
					State:   replicaset.SecondaryState,
				},
			},
		}, nil),
		st.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					Address: "1.1.1.1",
					State:   replicaset.RecoveringState,
				},
				{
					Id:      2,
					Address: "2.2.2.2",
					State:   replicaset.FatalState,
				},
				{
					Id:      3,
					Address: "3.3.3.3",
					State:   replicaset.Startup2State,
				},
				{
					Id:      4,
					Address: "4.4.4.4",
					State:   replicaset.UnknownState,
				},
				{
					Id:      5,
					Address: "5.5.5.5",
					State:   replicaset.ArbiterState,
				},
				{
					Id:      6,
					Address: "6.6.6.6",
					State:   replicaset.DownState,
				},
				{
					Id:      7,
					Address: "7.7.7.7",
					State:   replicaset.RollbackState,
				},
				{
					Id:      8,
					Address: "8.8.8.8",
					State:   replicaset.ShunnedState,
				},
			},
		}, nil),
	)

	blocker, err := upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `unable to upgrade, database node 1 (1.1.1.1) has state RECOVERING, node 2 (2.2.2.2) has state FATAL, node 3 (3.3.3.3) has state STARTUP2, node 4 (4.4.4.4) has state UNKNOWN, node 5 (5.5.5.5) has state ARBITER, node 6 (6.6.6.6) has state DOWN, node 7 (7.7.7.7) has state ROLLBACK, node 8 (8.8.8.8) has state SHUNNED`)
}

func (s *upgradeValidationSuite) TestCheckMongoVersionForControllerModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	pool := mocks.NewMockStatePool(ctrl)
	gomock.InOrder(
		pool.EXPECT().MongoVersion().Return(`4.4`, nil),
		pool.EXPECT().MongoVersion().Return(`4.3`, nil),
	)

	blocker, err := upgradevalidation.CheckMongoVersionForControllerModel("", pool, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.CheckMongoVersionForControllerModel("", pool, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `mongo version has to be "4.4" at least, but current version is "4.3"`)
}

func (s *upgradeValidationSuite) assertGetCheckForLXDVersion(c *tc.C, cloudType string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: cloudType}}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)("", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionLXD(c *tc.C) {
	s.assertGetCheckForLXDVersion(c, "lxd")
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionLocalhost(c *tc.C) {
	s.assertGetCheckForLXDVersion(c, "localhost")
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionSkippedForNonLXDCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	blocker, err := upgradevalidation.GetCheckForLXDVersion(environscloudspec.CloudSpec{Type: "foo"})("", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionFailed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)("", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.NotNil)
	c.Assert(blocker.Error(), tc.Equals, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}

func (s *upgradeValidationSuite) TestCheckForCharmStoreCharmsNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().AllCharmURLs().Return([]*string{}, errors.NotFoundf("charm urls"))

	blocker, err := upgradevalidation.CheckForCharmStoreCharms("", nil, st, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)
}

func (s *upgradeValidationSuite) TestCheckForCharmStoreCharmsError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().AllCharmURLs().Return([]*string{}, errors.BadRequestf("charm urls"))

	_, err := upgradevalidation.CheckForCharmStoreCharms("", nil, st, nil)
	c.Assert(errors.Is(err, errors.BadRequest), tc.IsTrue)
}
