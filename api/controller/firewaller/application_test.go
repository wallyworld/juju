// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type applicationSuite struct {
	firewallerSuite

	apiApplication *firewaller.Application
}

func TestApplicationSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &applicationSuite{})
}

func (s *applicationSuite) SetUpTest(c *tc.C) {
	s.firewallerSuite.SetUpTest(c)

	apiUnit, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, tc.ErrorIsNil)
	s.apiApplication, err = apiUnit.Application()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TearDownTest(c *tc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *applicationSuite) TestName(c *tc.C) {
	c.Assert(s.apiApplication.Name(), tc.Equals, s.application.Name())
}

func (s *applicationSuite) TestTag(c *tc.C) {
	c.Assert(s.apiApplication.Tag(), tc.Equals, names.NewApplicationTag(s.application.Name()))
}

func (s *applicationSuite) TestWatch(c *tc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w, err := s.apiApplication.Watch()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something and check it's detected.
	err = s.application.MergeExposeSettings(nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Destroy the application and check it's detected.
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *applicationSuite) TestExposeInfo(c *tc.C) {
	err := s.application.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"10.0.0.0/16", "192.168.0.0/24"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	isExposed, exposedEndpoints, err := s.apiApplication.ExposeInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isExposed, tc.IsTrue)
	c.Assert(exposedEndpoints, tc.DeepEquals, map[string]params.ExposedEndpoint{
		"": {
			ExposeToSpaces: []string{network.AlphaSpaceId},
			ExposeToCIDRs:  []string{"10.0.0.0/16", "192.168.0.0/24"},
		},
	})

	err = s.application.ClearExposed()
	c.Assert(err, tc.ErrorIsNil)

	isExposed, exposedEndpoints, err = s.apiApplication.ExposeInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isExposed, tc.IsFalse)
	c.Assert(exposedEndpoints, tc.HasLen, 0)
}
