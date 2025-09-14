// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	apiserveragent "github.com/juju/juju/apiserver/facades/agent/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type servingInfoSuite struct {
	testing.JujuConnSuite
}

func TestServingInfoSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &servingInfoSuite{})
}

func (s *servingInfoSuite) TestStateServingInfo(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)

	ssi := controller.StateServingInfo{
		PrivateKey:   "some key",
		Cert:         "Some cert",
		SharedSecret: "really, really secret",
		APIPort:      33,
		StatePort:    44,
	}
	err := s.State.SetStateServingInfo(ssi)
	c.Assert(err, tc.ErrorIsNil)
	apiSt, err := apiagent.NewState(st)
	c.Assert(err, tc.ErrorIsNil)
	info, err := apiSt.StateServingInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, ssi)
}

func (s *servingInfoSuite) TestStateServingInfoPermission(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)
	apiSt, err := apiagent.NewState(st)
	c.Assert(err, tc.ErrorIsNil)
	_, err = apiSt.StateServingInfo()
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

func (s *servingInfoSuite) TestIsMaster(c *tc.C) {
	calledIsMaster := false
	var fakeMongoIsMaster = func(session *mgo.Session, m mongo.WithAddresses) (bool, error) {
		calledIsMaster = true
		return true, nil
	}
	s.PatchValue(&apiserveragent.MongoIsMaster, fakeMongoIsMaster)

	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	expected := true
	apiSt, err := apiagent.NewState(st)
	c.Assert(err, tc.ErrorIsNil)
	result, err := apiSt.IsMaster()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, expected)
	c.Assert(calledIsMaster, tc.IsTrue)
}

func (s *servingInfoSuite) TestIsMasterPermission(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)
	apiSt, err := apiagent.NewState(st)
	c.Assert(err, tc.ErrorIsNil)
	_, err = apiSt.IsMaster()
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

type machineSuite struct {
	testing.JujuConnSuite
	machine *state.Machine
	st      api.Connection
}

func TestMachineSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &machineSuite{})
}

func (s *machineSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)
}

func (s *machineSuite) TestIsControllerShortCircuits(c *tc.C) {
	result, err := apiagent.IsController(nil, names.NewControllerAgentTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.IsTrue)
}

func (s *machineSuite) TestMachineEntity(c *tc.C) {
	tag := names.NewMachineTag("42")
	apiSt, err := apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err := apiSt.Entity(tag)
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, tc.IsNil)

	apiSt, err = apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err = apiSt.Entity(s.machine.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Tag(), tc.Equals, s.machine.Tag().String())
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(m.Jobs(), tc.DeepEquals, []model.MachineJob{model.JobHostUnits})

	err = s.machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, tc.ErrorIsNil)

	apiSt, err = apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	m, err = apiSt.Entity(s.machine.Tag())
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, tc.IsNil)
}

func (s *machineSuite) TestEntitySetPassword(c *tc.C) {
	apiSt, err := apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := apiSt.Entity(s.machine.Tag())
	c.Assert(err, tc.ErrorIsNil)

	err = entity.SetPassword("foo")
	c.Assert(err, tc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	err = entity.SetPassword("foo-12345678901234567890")
	c.Assert(err, tc.ErrorIsNil)
	err = entity.ClearReboot()
	c.Assert(err, tc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machine.PasswordValid("bar"), tc.IsFalse)
	c.Assert(s.machine.PasswordValid("foo-12345678901234567890"), tc.IsTrue)

	// Check that we cannot log in to mongo with the correct password.
	// This is because there's no mongo password set for s.machine,
	// which has JobHostUnits
	info := s.MongoInfo()
	// TODO(dfc) this entity.Tag should return a Tag
	tag, err := names.ParseTag(entity.Tag())
	c.Assert(err, tc.ErrorIsNil)
	info.Tag = tag
	info.Password = "foo-12345678901234567890"
	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	c.Assert(err, tc.Satisfies, errors.IsUnauthorized)
	c.Assert(session, tc.IsNil)
}

func (s *machineSuite) TestClearReboot(c *tc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, tc.ErrorIsNil)
	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsTrue)

	apiSt, err := apiagent.NewState(s.st)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := apiSt.Entity(s.machine.Tag())
	c.Assert(err, tc.ErrorIsNil)

	err = entity.ClearReboot()
	c.Assert(err, tc.ErrorIsNil)

	rFlag, err = s.machine.GetRebootFlag()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rFlag, tc.IsFalse)
}
