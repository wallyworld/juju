// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type fanConfigurerSuite struct {
	testhelpers.IsolationSuite

	facade *MockFanConfigurerFacade
}

func TestFanConfigurerSuite(t *tctesting.T) {
	tc.Run(t, &fanConfigurerSuite{})
}

func (s *fanConfigurerSuite) TestProcessNewConfigNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().FanConfig().Return(nil, errors.NotImplemented)

	fc := &FanConfigurer{
		config: FanConfigurerConfig{
			Facade: s.facade,
		},
	}

	err := fc.processNewConfig()
	c.Assert(err, tc.IsNil)
}

func (s *fanConfigurerSuite) TestProcessLoopNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().WatchForFanConfigChanges().Return(nil, errors.NotImplemented)

	fc := &FanConfigurer{
		config: FanConfigurerConfig{
			Facade: s.facade,
		},
	}

	err := fc.loop()
	c.Assert(err, tc.IsNil)
}

func (s *fanConfigurerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = NewMockFanConfigurerFacade(ctrl)

	return ctrl
}
