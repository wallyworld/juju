// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type sshTunnelerSuite struct {
	testhelpers.IsolationSuite
}

func TestSshTunnelerSuite(t *tctesting.T) {
	tc.Run(t, &sshTunnelerSuite{})
}

func newClient(f basetesting.APICallerFunc) *Client {
	return NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *sshTunnelerSuite) TestControllerAddresses(c *tc.C) {
	entity := names.NewMachineTag("1")

	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHTunneler")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ControllerAddresses")
			c.Assert(arg, tc.DeepEquals, params.Entity{Tag: entity.String()})
			c.Assert(result, tc.FitsTypeOf, &params.StringsResult{})

			*(result.(*params.StringsResult)) = params.StringsResult{
				Result: []string{"1.2.3.4"},
			}
			return nil
		},
	)

	addresses, err := client.ControllerAddresses(entity)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		addresses,
		tc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("1.2.3.4")},
	)
}

func (s *sshTunnelerSuite) TestControllerAddressesError(c *tc.C) {
	entity := names.NewMachineTag("1")

	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHTunneler")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ControllerAddresses")
			c.Assert(arg, tc.DeepEquals, params.Entity{Tag: entity.String()})
			c.Assert(result, tc.FitsTypeOf, &params.StringsResult{})

			*(result.(*params.StringsResult)) = params.StringsResult{
				Error: &params.Error{Message: "my-error"},
			}
			return nil
		},
	)

	_, err := client.ControllerAddresses(entity)
	c.Assert(err, tc.ErrorMatches, "my-error")
}

func (s *sshTunnelerSuite) TestInsertSSHConnRequest(c *tc.C) {
	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHTunneler")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "InsertSSHConnRequest")
			c.Assert(arg, tc.DeepEquals, params.SSHConnRequestArg{
				Username: "ubuntu",
				Password: "foo",
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResult{})

			*(result.(*params.ErrorResult)) = params.ErrorResult{
				Error: nil,
			}
			return nil
		},
	)

	req := state.SSHConnRequestArg{
		Username: "ubuntu",
		Password: "foo",
	}
	err := client.InsertSSHConnRequest(req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *sshTunnelerSuite) TestMachineHostKeys(c *tc.C) {
	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHTunneler")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "MachineHostKeys")
			c.Assert(arg, tc.DeepEquals, params.SSHMachineHostKeysArg{
				ModelUUID:  "my-model",
				MachineTag: "machine-1",
			})
			c.Assert(result, tc.FitsTypeOf, &params.SSHPublicKeysResult{})

			*(result.(*params.SSHPublicKeysResult)) = params.SSHPublicKeysResult{
				Error:      nil,
				PublicKeys: []string{"key-1", "key-2"},
			}
			return nil
		},
	)

	result, err := client.MachineHostKeys("my-model", names.NewMachineTag("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []string{"key-1", "key-2"})
}
