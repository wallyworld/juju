// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v6"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestRegisterExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator, testing.WrapCheckLog(c))
}

func (s *exportSuite) TestNilModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(nil, nil)

	model := description.NewModel(description.ModelArgs{})

	op := s.newExportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *exportSuite) TestEmptyModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := &config.Config{}

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(config, nil)

	model := description.NewModel(description.ModelArgs{})

	op := s.newExportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *exportSuite) TestModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config, err := config.New(config.NoDefaults, map[string]any{
		"name": "foo",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(config, nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{},
	})

	op := s.newExportOperation()
	err = op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Config(), jc.DeepEquals, config.AllAttrs())
}
