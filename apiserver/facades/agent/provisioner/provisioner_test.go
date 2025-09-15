// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/facades/agent/provisioner/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	environtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
)

type provisionerSuite struct {
	testing.JujuConnSuite

	machines []*state.Machine

	authorizer  apiservertesting.FakeAuthorizer
	resources   *common.Resources
	provisioner *provisioner.ProvisionerAPIV11
}

func TestProvisionerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &provisionerSuite{})
}

func (s *provisionerSuite) SetUpTest(c *tc.C) {
	s.setUpTest(c, false)
}

func (s *provisionerSuite) setUpTest(c *tc.C, withController bool) {
	if s.JujuConnSuite.ConfigAttrs == nil {
		s.JujuConnSuite.ConfigAttrs = make(map[string]interface{})
	}
	s.JujuConnSuite.ConfigAttrs["image-stream"] = "daily"
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines (if any) and create 3 machines
	// for the tests, plus an optional controller machine.
	s.machines = nil
	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	if withController {
		s.machines = append(s.machines, testing.AddControllerMachine(c, s.State))
	}
	for i := 0; i < 5; i++ {
		machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Check(err, tc.ErrorIsNil)
		s.machines = append(s.machines, machine)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as the controller.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
	}

	// Create the resource registry separately to track invocations to
	// Register, and to register the root for tools URLs.
	s.resources = common.NewResources()

	// Create a provisioner API for the machine.
	provisionerAPI, err := provisioner.NewProvisionerAPIV11(facadetest.Context{
		Auth_:      s.authorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.provisioner = provisionerAPI
}

type withoutControllerSuite struct {
	provisionerSuite
	*commontesting.ModelWatcherTest
}

func TestWithoutControllerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &withoutControllerSuite{})
}

func (s *withoutControllerSuite) SetUpTest(c *tc.C) {
	s.setUpTest(c, false)
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(s.provisioner, s.State, s.resources)
}

func (s *withoutControllerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = true
	// Works with a controller, which is not a machine agent.
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	// But fails with neither a machine agent or a controller.
	anAuthorizer.Controller = false
	aProvisioner, err = provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.NotNil)
	c.Assert(aProvisioner, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *withoutControllerSuite) TestSetPasswords(c *tc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[0].Tag().String(), Password: "xxx0-1234567890123457890"},
			{Tag: s.machines[1].Tag().String(), Password: "xxx1-1234567890123457890"},
			{Tag: s.machines[2].Tag().String(), Password: "xxx2-1234567890123457890"},
			{Tag: s.machines[3].Tag().String(), Password: "xxx3-1234567890123457890"},
			{Tag: s.machines[4].Tag().String(), Password: "xxx4-1234567890123457890"},
			{Tag: "machine-42", Password: "foo"},
			{Tag: "unit-foo-0", Password: "zzz"},
			{Tag: "application-bar", Password: "abc"},
		},
	}
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes to both machines succeeded.
	for i, machine := range s.machines {
		c.Logf("trying %q password", machine.Tag())
		err = machine.Refresh()
		c.Assert(err, tc.ErrorIsNil)
		changed := machine.PasswordValid(fmt.Sprintf("xxx%d-1234567890123457890", i))
		c.Assert(changed, tc.IsTrue)
	}
}

func (s *withoutControllerSuite) TestShortSetPasswords(c *tc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: s.machines[1].Tag().String(), Password: "xxx1"},
		},
	}
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches,
		"password is only 4 bytes long, and is not a valid Agent password")
}

func (s *withoutControllerSuite) TestLifeAsMachineAgent(c *tc.C) {
	// NOTE: This and the next call serve to test the two
	// different authorization schemes:
	// 1. Machine agents can access their own machine and
	// any container that has their own machine as parent;
	// 2. Controllers can access any machine without
	// a parent.
	// There's no need to repeat this test for each method,
	// because the authorization logic is common.

	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	// Make the machine dead before trying to add containers.
	err = s.machines[0].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// Create some containers to work on.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	var containers []*state.Machine
	for i := 0; i < 3; i++ {
		container, err := s.State.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXD)
		c.Check(err, tc.ErrorIsNil)
		containers = append(containers, container)
	}
	// Make one container dead.
	err = containers[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: containers[0].Tag().String()},
		{Tag: containers[1].Tag().String()},
		{Tag: containers[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := aProvisioner.Life(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestLifeAsController(c *tc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machines[0].Life(), tc.Equals, state.Alive)
	c.Assert(s.machines[1].Life(), tc.Equals, state.Dead)
	c.Assert(s.machines[2].Life(), tc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Life(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.machines[1].Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	result, err = s.provisioner.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag().String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

func (s *withoutControllerSuite) TestRemove(c *tc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Remove(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `cannot remove entity "machine-0": still alive`}},
			{nil},
			{&params.Error{Message: `cannot remove entity "machine-2": still alive`}},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Alive)
	err = s.machines[2].Refresh()
	c.Assert(err, tc.ErrorIsNil)
	s.assertLife(c, 2, state.Alive)
}

func (s *withoutControllerSuite) TestSetStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Error.String(), Info: "not really",
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Stopped.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Started.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Started.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Stopped.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Stopped.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertStatus(c, 0, status.Error, "not really", map[string]interface{}{"foo": "bar"})
	s.assertStatus(c, 1, status.Stopped, "foobar", map[string]interface{}{})
	s.assertStatus(c, 2, status.Started, "again", map[string]interface{}{})
}

func (s *withoutControllerSuite) TestSetInstanceStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Provisioning.String(), Info: "not really",
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Running.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.ProvisioningError.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Provisioning.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.ProvisioningError.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetInstanceStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertInstanceStatus(c, 0, status.Provisioning, "not really", map[string]interface{}{"foo": "bar"})
	s.assertInstanceStatus(c, 1, status.Running, "foobar", map[string]interface{}{})
	s.assertInstanceStatus(c, 2, status.ProvisioningError, "again", map[string]interface{}{})
	// ProvisioningError also has a special case which is to set the machine to Error
	s.assertStatus(c, 2, status.Error, "again", map[string]interface{}{})
}

func (s *withoutControllerSuite) TestSetModificationStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Applied,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Since:   &now,
	}
	err = s.machines[2].SetModificationStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: s.machines[0].Tag().String(), Status: status.Pending.String(), Info: "not really",
				Data: map[string]interface{}{"foo": "bar"}},
			{Tag: s.machines[1].Tag().String(), Status: status.Applied.String(), Info: "foobar"},
			{Tag: s.machines[2].Tag().String(), Status: status.Error.String(), Info: "again"},
			{Tag: "machine-42", Status: status.Pending.String(), Info: "blah"},
			{Tag: "unit-foo-0", Status: status.Error.String(), Info: "foobar"},
			{Tag: "application-bar", Status: status.Error.String(), Info: "foobar"},
		}}
	result, err := s.provisioner.SetModificationStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertModificationStatus(c, 0, status.Pending, "not really", map[string]interface{}{"foo": "bar"})
	s.assertModificationStatus(c, 1, status.Applied, "foobar", map[string]interface{}{})
	s.assertModificationStatus(c, 2, status.Error, "again", map[string]interface{}{})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrors(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]interface{}{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	// Machine 4 is provisioned but error not reset yet.
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "transient error",
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[4].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[4].SetProvisioned("i-am", "", "fake_nonce", &hwChars)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.provisioner.MachinesWithTransientErrors()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Id: "1", Life: "alive", Status: "provisioning error", Info: "transient error",
				Data: map[string]interface{}{"transient": true, "foo": "bar"}},
		},
	})
}

func (s *withoutControllerSuite) TestMachinesWithTransientErrorsPermission(c *tc.C) {
	// Machines where there's permission issues are omitted.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = names.NewMachineTag("1")
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	},
	)
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "blah",
		Since:   &now,
	}
	err = s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "transient error",
		Data:    map[string]interface{}{"transient": true, "foo": "bar"},
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Data:    map[string]interface{}{"transient": false},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "error",
		Since:   &now,
	}
	err = s.machines[3].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.MachinesWithTransientErrors()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Id: "1", Life: "alive", Status: "provisioning error",
			Info: "transient error",
			Data: map[string]interface{}{"transient": true, "foo": "bar"},
		},
		},
	})
}

func (s *withoutControllerSuite) TestEnsureDead(c *tc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	s.assertLife(c, 0, state.Alive)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.EnsureDead(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes.
	s.assertLife(c, 0, state.Dead)
	s.assertLife(c, 1, state.Dead)
	s.assertLife(c, 2, state.Dead)
}

func (s *withoutControllerSuite) assertLife(c *tc.C, index int, expectLife state.Life) {
	err := s.machines[index].Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.machines[index].Life(), tc.Equals, expectLife)
}

func (s *withoutControllerSuite) assertStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

	statusInfo, err := s.machines[index].Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertInstanceStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

	statusInfo, err := s.machines[index].InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) assertModificationStatus(c *tc.C, index int, expectStatus status.Status, expectInfo string,
	expectData map[string]interface{}) {

	statusInfo, err := s.machines[index].ModificationStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo.Status, tc.Equals, expectStatus)
	c.Assert(statusInfo.Message, tc.Equals, expectInfo)
	c.Assert(statusInfo.Data, tc.DeepEquals, expectData)
}

func (s *withoutControllerSuite) TestWatchContainers(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String(), ContainerType: string(instance.LXD)},
		{MachineTag: s.machines[1].Tag().String(), ContainerType: string(instance.KVM)},
		{MachineTag: "machine-42", ContainerType: ""},
		{MachineTag: "unit-foo-0", ContainerType: ""},
		{MachineTag: "application-bar", ContainerType: ""},
	}}
	result, err := s.provisioner.WatchContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestWatchAllContainers(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.WatchContainers{Params: []params.WatchContainer{
		{MachineTag: s.machines[0].Tag().String()},
		{MachineTag: s.machines[1].Tag().String()},
		{MachineTag: "machine-42"},
		{MachineTag: "unit-foo-0"},
		{MachineTag: "application-bar"},
	}}
	result, err := s.provisioner.WatchAllContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{}},
			{StringsWatcherId: "2", Changes: []string{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 2)
	m0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, m0Watcher)
	m1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, m1Watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc0 := statetesting.NewStringsWatcherC(c, m0Watcher.(state.StringsWatcher))
	wc0.AssertNoChange()
	wc1 := statetesting.NewStringsWatcherC(c, m1Watcher.(state.StringsWatcher))
	wc1.AssertNoChange()
}

func (s *withoutControllerSuite) TestModelConfigNonManager(c *tc.C) {
	// Now test it with a non-controller and make sure
	// the secret attributes are masked.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertModelConfig(c, aProvisioner)
}

func (s *withoutControllerSuite) TestStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Stopped,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Status(args)
	c.Assert(err, tc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, tc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: status.Started.String(), Info: "blah", Data: map[string]interface{}{}},
			{Status: status.Stopped.String(), Info: "foo", Data: map[string]interface{}{}},
			{Status: status.Error.String(), Info: "not really", Data: map[string]interface{}{"foo": "bar"}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestInstanceStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Provisioning,
		Message: "blah",
		Since:   &now,
	}
	err := s.machines[0].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Running,
		Message: "foo",
		Since:   &now,
	}
	err = s.machines[1].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "not really",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}
	err = s.machines[2].SetInstanceStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceStatus(args)
	c.Assert(err, tc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, tc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: status.Provisioning.String(), Info: "blah", Data: map[string]interface{}{}},
			{Status: status.Running.String(), Info: "foo", Data: map[string]interface{}{}},
			{Status: status.ProvisioningError.String(), Info: "not really", Data: map[string]interface{}{"foo": "bar"}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestAvailabilityZone(c *tc.C) {
	availabilityZone := "ru-north-siberia"
	emptyAz := ""
	hcWithAZ := instance.HardwareCharacteristics{AvailabilityZone: &availabilityZone}
	hcWithEmptyAZ := instance.HardwareCharacteristics{AvailabilityZone: &emptyAz}
	hcWithNilAz := instance.HardwareCharacteristics{AvailabilityZone: nil}

	// add machines with different availability zones: string, empty string, nil
	azMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithAZ,
	})

	emptyAzMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithEmptyAZ,
	})

	nilAzMachine, _ := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Characteristics: &hcWithNilAz,
	})
	args := params.Entities{Entities: []params.Entity{
		{Tag: azMachine.Tag().String()},
		{Tag: emptyAzMachine.Tag().String()},
		{Tag: nilAzMachine.Tag().String()},
	}}
	result, err := s.provisioner.AvailabilityZone(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: availabilityZone},
			{Result: emptyAz},
			{Result: emptyAz},
		},
	})
}

func (s *withoutControllerSuite) TestKeepInstance(c *tc.C) {
	// Add a machine with keep-instance = true.
	foobarMachine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "1234"})
	err := foobarMachine.SetKeepInstance(true)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: foobarMachine.Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.KeepInstance(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false},
			{Result: true},
			{Result: false},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroup(c *tc.C) {
	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := s.AddTestingApplication(c, name, s.AddTestingCharm(c, name))
		for _, m := range machines {
			unit, err := app.AddUnit(state.AddUnitParams{})
			c.Assert(err, tc.ErrorIsNil)
			err = unit.AssignToMachine(m)
			c.Assert(err, tc.ErrorIsNil)
			units = append(units, unit)
		}
		return units
	}
	setProvisioned := func(id string) {
		m, err := s.State.Machine(id)
		c.Assert(err, tc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id("machine-"+id+"-inst"), "", "nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	mysqlUnit := addUnits("mysql", s.machines[0], s.machines[3])[0]
	wordpressUnits := addUnits("wordpress", s.machines[0], s.machines[1], s.machines[2])

	// Unassign wordpress/1 from machine-1.
	// The unit should not show up in the results.
	err := wordpressUnits[1].UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)

	// Provision machines 1, 2 and 3. Machine-0 remains
	// unprovisioned, and machine-1 has no units, and so
	// neither will show up in the results.
	setProvisioned("1")
	setProvisioned("2")
	setProvisioned("3")

	// Add a few controllers, provision two of them.
	_, err = s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	setProvisioned("5")
	setProvisioned("7")

	// Create a logging service, subordinate to mysql.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(mysqlUnit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.machines[3].Tag().String()},
		{Tag: "machine-5"},
	}}
	result, err := s.provisioner.DistributionGroup(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Result: []instance.Id{"machine-2-inst", "machine-3-inst"}},
			{Result: []instance.Id{}},
			{Result: []instance.Id{"machine-2-inst"}},
			{Result: []instance.Id{"machine-3-inst"}},
			{Result: []instance.Id{"machine-5-inst", "machine-7-inst"}},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupControllerAuth(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroup(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			// controller may access any top-level machines.
			{Result: []instance.Id{}},
			{Error: apiservertesting.NotFoundError("machine 42")},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			// non-machines always unauthorized
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupMachineAgentAuth(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroup(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: []instance.Id{}},
			{Error: apiservertesting.ErrUnauthorized},
			// only a machine agent for the container or its
			// parent may access it.
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 1/lxd/99")},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineId(c *tc.C) {
	addUnits := func(name string, machines ...*state.Machine) (units []*state.Unit) {
		app := s.AddTestingApplication(c, name, s.AddTestingCharm(c, name))
		for _, m := range machines {
			unit, err := app.AddUnit(state.AddUnitParams{})
			c.Assert(err, tc.ErrorIsNil)
			err = unit.AssignToMachine(m)
			c.Assert(err, tc.ErrorIsNil)
			units = append(units, unit)
		}
		return units
	}
	setProvisioned := func(id string) {
		m, err := s.State.Machine(id)
		c.Assert(err, tc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id("machine-"+id+"-inst"), "", "nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
	}

	_ = addUnits("mysql", s.machines[0], s.machines[3])[0]
	wordpressUnits := addUnits("wordpress", s.machines[0], s.machines[1], s.machines[2])

	// Unassign wordpress/1 from machine-1.
	// The unit should not show up in the results.
	err := wordpressUnits[1].UnassignFromMachine()
	c.Assert(err, tc.ErrorIsNil)

	// Provision machines 1, 2 and 3. Machine-0 remains
	// unprovisioned.
	setProvisioned("1")
	setProvisioned("2")
	setProvisioned("3")

	// Add a few controllers, provision two of them.
	_, err = s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	setProvisioned("5")
	setProvisioned("7")

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.machines[3].Tag().String()},
		{Tag: "machine-5"},
	}}
	result, err := s.provisioner.DistributionGroupByMachineId(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"2", "3"}},
			{Result: []string{}},
			{Result: []string{"0"}},
			{Result: []string{"0"}},
			{Result: []string{"6", "7"}},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdControllerAuth(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.DistributionGroupByMachineId(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			// controller may access any top-level machines.
			{Result: []string{}, Error: nil},
			{Result: nil, Error: apiservertesting.NotFoundError("machine 42")},
			// only a machine agent for the container or its
			// parent may access it.
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			// non-machines always unauthorized
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestDistributionGroupByMachineIdMachineAgentAuth(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	provisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Check(err, tc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "machine-0-lxd-99"},
		{Tag: "machine-1-lxd-99"},
		{Tag: "machine-1-lxd-99-lxd-100"},
	}}
	result, err := provisioner.DistributionGroupByMachineId(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: []string{}, Error: nil},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			// only a machine agent for the container or its
			// parent may access it.
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
			{Result: nil, Error: apiservertesting.NotFoundError("machine 1/lxd/99")},
			{Result: nil, Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestConstraints(c *tc.C) {
	// Add a machine with some constraints.
	cons := constraints.MustParse("cores=123", "mem=8G")
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	}
	consMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)

	machine0Constraints, err := s.machines[0].Constraints()
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: consMachine.Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.Constraints(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ConstraintsResults{
		Results: []params.ConstraintsResult{
			{Constraints: machine0Constraints},
			{Constraints: template.Constraints},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSetInstanceInfo(c *tc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("static-pool", "static", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.Model.UpdateModelConfig(map[string]interface{}{
		"storage-default-block-source": "static-pool",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Provision machine 0 first.
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[0].SetInstanceInfo("i-am", "", "fake_nonce", &hwChars, nil, nil, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	volumesMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Size: 1000},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	args := params.InstancesInfo{Machines: []params.InstanceInfo{{
		Tag:        s.machines[0].Tag().String(),
		InstanceId: "i-was",
		Nonce:      "fake_nonce",
	}, {
		Tag:             s.machines[1].Tag().String(),
		InstanceId:      "i-will",
		Nonce:           "fake_nonce",
		Characteristics: &hwChars,
	}, {
		Tag:             s.machines[2].Tag().String(),
		InstanceId:      "i-am-too",
		Nonce:           "fake",
		Characteristics: nil,
	}, {
		Tag:        volumesMachine.Tag().String(),
		InstanceId: "i-am-also",
		Nonce:      "fake",
		Volumes: []params.Volume{{
			VolumeTag: "volume-0",
			Info: params.VolumeInfo{
				VolumeId: "vol-0",
				Size:     1234,
			},
		}},
		VolumeAttachments: map[string]params.VolumeAttachmentInfo{
			"volume-0": {
				DeviceName: "sda",
			},
		},
	},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.SetInstanceInfo(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{
				Message: `cannot record provisioning info for "i-was": cannot set instance data for machine "0": already set`,
			}},
			{nil},
			{nil},
			{nil},
			{apiservertesting.NotFoundError("machine 42")},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify machine 1 and 2 were provisioned.
	c.Assert(s.machines[1].Refresh(), tc.IsNil)
	c.Assert(s.machines[2].Refresh(), tc.IsNil)

	instanceId, err := s.machines[1].InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, instance.Id("i-will"))
	instanceId, err = s.machines[2].InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, instance.Id("i-am-too"))
	c.Check(s.machines[1].CheckProvisioned("fake_nonce"), tc.IsTrue)
	c.Check(s.machines[2].CheckProvisioned("fake"), tc.IsTrue)
	gotHardware, err := s.machines[1].HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotHardware, tc.DeepEquals, &hwChars)

	// Verify the machine with requested volumes was provisioned, and the
	// volume information recorded in state.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(volumesMachine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 1)
	volumeAttachmentInfo, err := volumeAttachments[0].Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachmentInfo, tc.Equals, state.VolumeAttachmentInfo{DeviceName: "sda"})
	volume, err := sb.Volume(volumeAttachments[0].Volume())
	c.Assert(err, tc.ErrorIsNil)
	volumeInfo, err := volume.Info()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeInfo, tc.Equals, state.VolumeInfo{VolumeId: "vol-0", Pool: "static-pool", Size: 1234})

	// Verify the machine without requested volumes still has no volume
	// attachments recorded in state.
	volumeAttachments, err = sb.MachineVolumeAttachments(s.machines[1].MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 0)
}

func (s *withoutControllerSuite) TestInstanceId(c *tc.C) {
	// Provision 2 machines first.
	err := s.machines[0].SetProvisioned("i-am", "", "fake_nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	hwChars := instance.MustParseHardware("arch=arm64", "mem=4G")
	err = s.machines[1].SetProvisioned("i-am-not", "", "fake_nonce", &hwChars)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: "machine-42"},
		{Tag: "unit-foo-0"},
		{Tag: "application-bar"},
	}}
	result, err := s.provisioner.InstanceId(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-am"},
			{Result: "i-am-not"},
			{Error: apiservertesting.NotProvisionedError("2")},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestWatchModelMachines(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	got, err := s.provisioner.WatchModelMachines()
	c.Assert(err, tc.ErrorIsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0", "1", "2", "3", "4"},
	}
	c.Assert(got.StringsWatcherId, tc.Equals, want.StringsWatcherId)
	c.Assert(got.Changes, tc.SameContents, want.Changes)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Make sure WatchModelMachines fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.WatchModelMachines()
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{})
}

func (s *provisionerSuite) getManagerConfig(c *tc.C, typ instance.ContainerType) map[string]string {
	args := params.ContainerManagerConfigParams{Type: typ}
	results, err := s.provisioner.ContainerManagerConfig(args)
	c.Assert(err, tc.ErrorIsNil)
	return results.ManagerConfig
}

func (s *withoutControllerSuite) TestContainerManagerConfigDefaults(c *tc.C) {
	cfg := s.getManagerConfig(c, instance.KVM)
	c.Assert(cfg, tc.DeepEquals, map[string]string{
		container.ConfigModelUUID:        coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey:   "released",
		config.ContainerNetworkingMethod: config.ConfigDefaults()[config.ContainerNetworkingMethod].(string),
	})
}

func (s *withoutControllerSuite) TestContainerManagerConfigDefaultMetadataDisabled(c *tc.C) {
	attrs := map[string]interface{}{
		"container-image-metadata-defaults-disabled": true,
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, tc.ErrorIsNil)
	cfg := s.getManagerConfig(c, instance.KVM)
	c.Assert(cfg, tc.DeepEquals, map[string]string{
		container.ConfigModelUUID:                        coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey:                   "released",
		config.ContainerImageMetadataDefaultsDisabledKey: "true",
		config.ContainerNetworkingMethod:                 config.ConfigDefaults()[config.ContainerNetworkingMethod].(string),
	})
}

func (s *withoutControllerSuite) TestWatchMachineErrorRetry(c *tc.C) {
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	s.PatchValue(&provisioner.ErrorRetryWaitDelay, 2*coretesting.ShortWait)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	_, err := s.provisioner.WatchMachineErrorRetry()
	c.Assert(err, tc.ErrorIsNil)

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()

	// We should now get a time triggered change.
	wc.AssertOneChange()

	// Make sure WatchMachineErrorRetry fails with a machine agent login.
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("1")
	anAuthorizer.Controller = false
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := aProvisioner.WatchMachineErrorRetry()
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{})
}

func (s *withoutControllerSuite) TestMarkMachinesForRemoval(c *tc.C) {
	err := s.machines[0].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines[2].EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	res, err := s.provisioner.MarkMachinesForRemoval(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-2"},         // ok
			{Tag: "machine-100"},       // not found
			{Tag: "machine-0"},         // ok
			{Tag: "machine-1"},         // not dead
			{Tag: "machine-0-lxd-5"},   // unauthorised
			{Tag: "application-thing"}, // only machines allowed
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	results := res.Results
	c.Assert(results, tc.HasLen, 6)
	c.Check(results[0].Error, tc.IsNil)
	c.Check(*results[1].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.NotFoundf("machine 100")))
	c.Check(*results[1].Error, tc.Satisfies, params.IsCodeNotFound)
	c.Check(results[2].Error, tc.IsNil)
	c.Check(*results[3].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.New("cannot remove machine 1: machine is not dead")))
	c.Check(*results[4].Error, tc.DeepEquals, *apiservertesting.ErrUnauthorized)
	c.Check(*results[5].Error, tc.DeepEquals,
		*apiservererrors.ServerError(errors.New(`"application-thing" is not a valid machine tag`)))

	removals, err := s.State.AllMachineRemovals()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removals, tc.SameContents, []string{"0", "2"})
}

func (s *withoutControllerSuite) TestContainerConfig(c *tc.C) {
	attrs := map[string]interface{}{
		"juju-http-proxy":              "http://proxy.example.com:9000",
		"apt-https-proxy":              "https://proxy.example.com:9000",
		"allow-lxd-loop-mounts":        true,
		"apt-mirror":                   "http://example.mirror.com",
		"snap-https-proxy":             "https://snap-proxy.example.com:9000",
		"snap-store-assertions":        "BLOB",
		"snap-store-proxy":             "b4dc0ffee",
		"cloudinit-userdata":           validCloudInitUserData,
		"container-inherit-properties": "ca-certs,apt-primary",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, tc.ErrorIsNil)
	expectedAPTProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		Https:   "https://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedSnapProxy := proxy.Settings{
		Https: "https://snap-proxy.example.com:9000",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.UpdateBehavior, tc.Not(tc.IsNil))
	c.Check(results.ProviderType, tc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, tc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, tc.IsTrue)
	c.Check(results.LegacyProxy.HasProxySet(), tc.IsFalse)
	c.Check(results.JujuProxy, tc.DeepEquals, expectedProxy)
	c.Check(results.AptProxy, tc.DeepEquals, expectedAPTProxy)
	c.Check(results.AptMirror, tc.DeepEquals, "http://example.mirror.com")
	c.Check(results.SnapProxy, tc.DeepEquals, expectedSnapProxy)
	c.Check(results.SnapStoreAssertions, tc.Equals, "BLOB")
	c.Check(results.SnapStoreProxyID, tc.Equals, "b4dc0ffee")
	c.Check(results.CloudInitUserData, tc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
	c.Check(results.ContainerInheritProperties, tc.DeepEquals, "ca-certs,apt-primary")
}

func (s *withoutControllerSuite) TestContainerConfigLegacy(c *tc.C) {
	attrs := map[string]interface{}{
		"http-proxy":                   "http://proxy.example.com:9000",
		"apt-https-proxy":              "https://proxy.example.com:9000",
		"allow-lxd-loop-mounts":        true,
		"apt-mirror":                   "http://example.mirror.com",
		"cloudinit-userdata":           validCloudInitUserData,
		"container-inherit-properties": "ca-certs,apt-primary",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, tc.ErrorIsNil)
	expectedAPTProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		Https:   "https://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	expectedProxy := proxy.Settings{
		Http:    "http://proxy.example.com:9000",
		NoProxy: "127.0.0.1,localhost,::1",
	}

	results, err := s.provisioner.ContainerConfig()
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.UpdateBehavior, tc.Not(tc.IsNil))
	c.Check(results.ProviderType, tc.Equals, "dummy")
	c.Check(results.AuthorizedKeys, tc.Equals, s.Environ.Config().AuthorizedKeys())
	c.Check(results.SSLHostnameVerification, tc.IsTrue)
	c.Check(results.LegacyProxy, tc.DeepEquals, expectedProxy)
	c.Check(results.JujuProxy.HasProxySet(), tc.IsFalse)
	c.Check(results.AptProxy, tc.DeepEquals, expectedAPTProxy)
	c.Check(results.AptMirror, tc.DeepEquals, "http://example.mirror.com")
	c.Check(results.CloudInitUserData, tc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
	c.Check(results.ContainerInheritProperties, tc.DeepEquals, "ca-certs,apt-primary")
}

func (s *withoutControllerSuite) TestSetSupportedContainers(c *tc.C) {
	args := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD, instance.KVM},
	}}}
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
	}
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{instance.LXD})
	m1, err := s.State.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *withoutControllerSuite) TestSetSupportedContainersPermissions(c *tc.C) {
	// Login as a machine agent for machine 0.
	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(facadetest.Context{
		Auth_:      anAuthorizer,
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	args := params.MachineContainersParams{
		Params: []params.MachineContainers{{
			MachineTag:     "machine-0",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		}, {
			MachineTag:     "machine-1",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		}, {
			MachineTag:     "machine-42",
			ContainerTypes: []instance.ContainerType{instance.LXD},
		},
		},
	}
	// Only machine 0 can have it's containers updated.
	results, err := aProvisioner.SetSupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *withoutControllerSuite) TestSupportedContainers(c *tc.C) {
	setArgs := params.MachineContainersParams{Params: []params.MachineContainers{{
		MachineTag:     "machine-0",
		ContainerTypes: []instance.ContainerType{instance.LXD},
	}, {
		MachineTag:     "machine-1",
		ContainerTypes: []instance.ContainerType{instance.LXD, instance.KVM},
	}}}
	_, err := s.provisioner.SetSupportedContainers(setArgs)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
	}
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, results.Results[0].ContainerTypes)
	m1, err := s.State.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok = m1.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, results.Results[1].ContainerTypes)
}

func (s *withoutControllerSuite) TestSupportedContainersWithoutBeingSet(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "machine-1",
	}}}
	results, err := s.provisioner.SupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.IsNil)
		c.Assert(result.ContainerTypes, tc.HasLen, 0)
	}
}

func (s *withoutControllerSuite) TestSupportedContainersWithInvalidTag(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: "user-0",
	}}}
	results, err := s.provisioner.SupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	for _, result := range results.Results {
		c.Assert(result.Error, tc.ErrorMatches, "permission denied")
	}
}

func (s *withoutControllerSuite) TestSupportsNoContainers(c *tc.C) {
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{
				MachineTag: "machine-0",
			},
		},
	}
	results, err := s.provisioner.SetSupportedContainers(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	m0, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	containers, ok := m0.SupportedContainers()
	c.Assert(ok, tc.IsTrue)
	c.Assert(containers, tc.DeepEquals, []instance.ContainerType{})
}

func TestWithControllerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &withControllerSuite{})
}

type withControllerSuite struct {
	provisionerSuite
}

func (s *withControllerSuite) SetUpTest(c *tc.C) {
	s.provisionerSuite.setUpTest(c, true)
}

func (s *withControllerSuite) TestAPIAddresses(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.provisioner.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestCACert(c *tc.C) {
	result, err := s.provisioner.CACert()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}

type withImageMetadataSuite struct {
	provisionerSuite
}

func TestWithImageMetadataSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &withImageMetadataSuite{})
}

func (s *withImageMetadataSuite) SetUpTest(c *tc.C) {
	s.ConfigAttrs = map[string]interface{}{
		config.ContainerImageStreamKey:      "daily",
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org/",
	}
	s.setUpTest(c, false)
}

func (s *withImageMetadataSuite) TestContainerManagerConfigImageMetadata(c *tc.C) {
	cfg := s.getManagerConfig(c, instance.LXD)
	c.Assert(cfg, tc.DeepEquals, map[string]string{
		container.ConfigModelUUID:           coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey:      "daily",
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org/",
		config.LXDSnapChannel:               "5.0/stable",
		config.ContainerNetworkingMethod:    config.ConfigDefaults()[config.ContainerNetworkingMethod].(string),
	})
}

// TODO(jam): 2017-02-15 We seem to be lacking most of direct unit tests around ProcessOneContainer
// Some of the use cases we need to be testing are:
// 1) Provider can allocate addresses, should result in a container with
//    addresses requested from the provider, and 'static' configuration on those
//    devices.
// 2) Provider cannot allocate addresses, currently this should make us use
//    'lxdbr0' and DHCP allocated addresses.
// 3) Provider could allocate DHCP based addresses on the host device, which would let us
//    use a bridge on the device and DHCP. (Currently not supported, but desirable for
//    vSphere and Manual and probably LXD providers.)
// Addition (manadart 2018-10-09): To begin accommodating the deficiencies noted
// above, the new suite below uses mocks for tests ill-suited to the dummy
// provider. We could reasonably re-write the tests above over time to use the
// new suite.

type provisionerMockSuite struct {
	coretesting.BaseSuite

	environ      *environtesting.MockNetworkingEnviron
	policy       *mocks.MockBridgePolicy
	host         *mocks.MockMachine
	container    *mocks.MockMachine
	device       *mocks.MockLinkLayerDevice
	parentDevice *mocks.MockLinkLayerDevice

	unit        *mocks.MockUnit
	application *mocks.MockApplication
	charm       *mocks.MockCharm
}

func TestProvisionerMockSuite(t *tctesting.T) {
	tc.Run(t, &provisionerMockSuite{})
}

// Even when the provider supports container addresses, manually provisioned
// machines should fall back to DHCP.
func (s *provisionerMockSuite) TestManuallyProvisionedHostsUseDHCPForContainers(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectManuallyProvisionedHostsUseDHCPForContainers()

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := provisioner.NewPrepareOrGetContext(res, false)

	// ProviderCallContext is not required by this logical path and can be nil
	err := ctx.ProcessOneContainer(s.environ, nil, s.policy, 0, s.host, s.container)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results[0].Config, tc.HasLen, 1)

	cfg := res.Results[0].Config[0]
	c.Check(cfg.ConfigType, tc.Equals, "dhcp")
	c.Check(cfg.ProviderSubnetId, tc.Equals, "")
	c.Check(cfg.VLANTag, tc.Equals, 0)
}

func (s *provisionerMockSuite) expectManuallyProvisionedHostsUseDHCPForContainers() {
	s.expectNetworkingEnviron()

	cExp := s.container.EXPECT()
	cExp.InstanceId().Return(instance.UnknownId, errors.NotProvisionedf("idk-lol"))

	s.policy.EXPECT().PopulateContainerLinkLayerDevices(s.host, s.container, false).Return(
		network.InterfaceInfos{
			{
				InterfaceName: "eth0",
				ConfigType:    network.ConfigDHCP,
			},
		}, nil)

	cExp.Id().Return("lxd/0").AnyTimes()

	hExp := s.host.EXPECT()
	// Crucial behavioural trait. Set false to test failure, whereupon the
	// PopulateContainerLinkLayerDevices expectation will not be satisfied.
	hExp.IsManual().Return(true, nil)
	hExp.InstanceId().Return(instance.Id("manual:10.0.0.66"), nil)
}

// expectNetworkingEnviron stubs an environ that supports container networking.
func (s *provisionerMockSuite) expectNetworkingEnviron() {
	eExp := s.environ.EXPECT()
	eExp.Config().Return(&config.Config{}).AnyTimes()
	eExp.SupportsContainerAddresses(gomock.Any()).Return(true, nil).AnyTimes()
}

func (s *provisionerMockSuite) TestContainerAlreadyProvisionedError(c *tc.C) {
	defer s.setup(c).Finish()

	exp := s.container.EXPECT()
	exp.InstanceId().Return(instance.Id("juju-8ebd6c-0"), nil)
	exp.Id().Return("0/lxd/0")

	res := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{}},
	}
	ctx := provisioner.NewPrepareOrGetContext(res, true)

	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(s.environ, nil, nil, 0, s.host, s.container)
	c.Assert(err, tc.ErrorMatches, `container "0/lxd/0" already provisioned as "juju-8ebd6c-0"`)
}

func (s *provisionerMockSuite) TestGetContainerProfileInfo(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.application.EXPECT().Name().Return("application")
	s.charm.EXPECT().Revision().Return(3)
	s.charm.EXPECT().LXDProfile().Return(
		&charm.LXDProfile{
			Config: map[string]string{
				"security.nesting":    "true",
				"security.privileged": "true",
			},
		})

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := provisioner.NewContainerProfileContext(res, "testme", coretesting.ModelTag)

	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(s.environ, nil, nil, 0, s.host, s.container)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, tc.HasLen, 1)
	profile := res.Results[0].LXDProfiles[0]
	c.Check(profile.Name, tc.Equals, "juju-testme-deadbe-application-3")
	c.Check(profile.Profile.Config, tc.DeepEquals,
		map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
	)
}

func (s *provisionerMockSuite) TestGetContainerProfileInfoNoProfile(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()
	s.expectCharmLXDProfiles(ctrl)

	s.charm.EXPECT().LXDProfile().Return(nil)
	s.unit.EXPECT().Name().Return("application/0")

	res := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{{}},
	}
	ctx := provisioner.NewContainerProfileContext(res, "testme", coretesting.ModelTag)

	// ProviderCallContext and BridgePolicy are not
	// required by this logical path and can be nil.
	err := ctx.ProcessOneContainer(s.environ, nil, nil, 0, s.host, s.container)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[0].LXDProfiles, tc.HasLen, 0)
}

func (s *provisionerMockSuite) expectCharmLXDProfiles(ctrl *gomock.Controller) {
	s.unit = mocks.NewMockUnit(ctrl)
	s.application = mocks.NewMockApplication(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)

	s.container.EXPECT().Units().Return([]provisioner.Unit{s.unit}, nil)
	s.unit.EXPECT().Application().Return(s.application, nil)
	s.application.EXPECT().Charm().Return(s.charm, false, nil)
}

func (s *provisionerMockSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.environ = environtesting.NewMockNetworkingEnviron(ctrl)
	s.policy = mocks.NewMockBridgePolicy(ctrl)
	s.host = mocks.NewMockMachine(ctrl)
	s.container = mocks.NewMockMachine(ctrl)
	s.device = mocks.NewMockLinkLayerDevice(ctrl)
	s.parentDevice = mocks.NewMockLinkLayerDevice(ctrl)

	return ctrl
}
