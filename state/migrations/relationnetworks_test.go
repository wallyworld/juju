// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	tctesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

type RelationNetworksExportSuite struct{}

func TestRelationNetworksExportSuite(t *tctesting.T) {
	tc.Run(t, &RelationNetworksExportSuite{})
}

func (s *RelationNetworksExportSuite) TestExportRelationNetworks(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.migrationRelationNetworks(ctrl, "uuid-4", "wordpress:db mysql:server", []string{"192.168.1.0/16"})

	entities := []MigrationRelationNetworks{
		entity0,
	}

	source := NewMockRelationNetworksSource(ctrl)
	source.EXPECT().AllRelationNetworks().Return(entities, nil)

	model := NewMockRelationNetworksModel(ctrl)
	model.EXPECT().AddRelationNetwork(description.RelationNetworkArgs{
		ID:          names.NewControllerTag("uuid-4").String(),
		RelationKey: "wordpress:db mysql:server",
		CIDRS:       []string{"192.168.1.0/16"},
	})

	migration := ExportRelationNetworks{}
	err := migration.Execute(source, model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *RelationNetworksExportSuite) TestExportRelationNetworksFailsGettingEntities(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockRelationNetworksSource(ctrl)
	source.EXPECT().AllRelationNetworks().Return(nil, errors.New("fail"))

	model := NewMockRelationNetworksModel(ctrl)

	migration := ExportRelationNetworks{}
	err := migration.Execute(source, model)
	c.Assert(err, tc.ErrorMatches, "fail")
}

func (s *RelationNetworksExportSuite) migrationRelationNetworks(ctrl *gomock.Controller, id, relationKey string, cidrs []string) *MockMigrationRelationNetworks {
	entity := NewMockMigrationRelationNetworks(ctrl)
	entity.EXPECT().Id().Return(names.NewControllerTag(id).String())
	entity.EXPECT().RelationKey().Return(relationKey)
	entity.EXPECT().CIDRS().Return(cidrs)
	return entity
}
