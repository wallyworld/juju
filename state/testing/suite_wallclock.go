// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	tctesting "testing"

	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

func TestStateWithWallClockSuite(t *tctesting.T) {
	tc.Run(t, &StateWithWallClockSuite{})
}

// StateWithWallClockSuite provides setup and teardown for tests that require a
// state.State. This should be deprecated in favour of StateSuite, and tests
// updated to use the testing clock StateSuite provides.
type StateWithWallClockSuite struct {
	mgotesting.MgoSuite
	coretesting.BaseSuite
	NewPolicy                 state.NewPolicyFunc
	Controller                *state.Controller
	StatePool                 *state.StatePool
	State                     *state.State
	Model                     *state.Model
	Owner                     names.UserTag
	Factory                   *factory.Factory
	InitialConfig             *config.Config
	ControllerInheritedConfig map[string]interface{}
	RegionConfig              cloud.RegionConfig
}

func (s *StateWithWallClockSuite) SetUpSuite(c *tc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *StateWithWallClockSuite) TearDownSuite(c *tc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *StateWithWallClockSuite) SetUpTest(c *tc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.Owner = names.NewLocalUserTag("test-admin")
	s.Controller = Initialize(c, s.Owner, s.InitialConfig, s.ControllerInheritedConfig, s.RegionConfig, s.NewPolicy)
	s.AddCleanup(func(*tc.C) {
		s.Controller.Close()
	})
	s.StatePool = s.Controller.StatePool()
	var err error
	s.State, err = s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.Model = model

	s.Factory = factory.NewFactory(s.State, s.StatePool)
}

func (s *StateWithWallClockSuite) TearDownTest(c *tc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}
