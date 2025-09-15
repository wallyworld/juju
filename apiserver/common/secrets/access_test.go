// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/internal/testing"
)

func (s *secretsSuite) TestCanManageOwnerUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretsConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	authTag := names.NewUnitTag("mariadb/0")

	uri := coresecrets.NewURI()
	gomock.InOrder(
		secretsConsumer.EXPECT().SecretAccess(uri, authTag).Return(coresecrets.RoleManage, nil),
	)

	t, err := secrets.CanManage(secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(t.Check(), tc.ErrorIsNil)
}

func (s *secretsSuite) TestCanManageLeaderUnitAppSecret(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretsConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	authTag := names.NewUnitTag("mariadb/0")

	uri := coresecrets.NewURI()
	gomock.InOrder(
		secretsConsumer.EXPECT().SecretAccess(uri, authTag).Return(coresecrets.RoleNone, nil),
		secretsConsumer.EXPECT().SecretAccess(uri, names.NewApplicationTag("mariadb")).Return(coresecrets.RoleManage, nil),
		leadershipChecker.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(token),
		token.EXPECT().Check().Return(nil),
	)

	_, err := secrets.CanManage(secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsSuite) TestCanManageAppTagLogin(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretsConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	authTag := names.NewApplicationTag("mariadb")

	uri := coresecrets.NewURI()
	gomock.InOrder(
		secretsConsumer.EXPECT().SecretAccess(uri, authTag).Return(coresecrets.RoleManage, nil),
	)

	t, err := secrets.CanManage(secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(t.Check(), tc.ErrorIsNil)
}

func (s *secretsSuite) TestCanManageUserSecrets(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	secretsConsumer := mocks.NewMockSecretsConsumer(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	authTag := coretesting.ModelTag

	uri := coresecrets.NewURI()
	gomock.InOrder(
		secretsConsumer.EXPECT().SecretAccess(uri, authTag).Return(coresecrets.RoleManage, nil),
	)

	t, err := secrets.CanManage(secretsConsumer, leadershipChecker, authTag, uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(t.Check(), tc.ErrorIsNil)
}
