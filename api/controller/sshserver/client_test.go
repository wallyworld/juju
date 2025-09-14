// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/sshserver"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/rpc/params"
)

type sshserverSuite struct {
	testhelpers.IsolationSuite
}

func TestSshserverSuite(t *tctesting.T) {
	tc.Run(t, &sshserverSuite{})
}

func newClient(f basetesting.APICallerFunc) (*sshserver.Client, error) {
	return sshserver.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *sshserverSuite) TestControllerConfig(c *tc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHServer")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ControllerConfig")
			c.Assert(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.ControllerConfigResult{})

			*(result.(*params.ControllerConfigResult)) = params.ControllerConfigResult{
				Config: params.ControllerConfig{
					"ssh-server-port":                96,
					"ssh-max-concurrent-connections": 96,
				},
			}
			return nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := client.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		cfg,
		tc.DeepEquals,
		controller.Config{
			"ssh-server-port":                96,
			"ssh-max-concurrent-connections": 96,
		},
	)
}

func (s *sshserverSuite) TestSSHServerHostKey(c *tc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHServer")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "SSHServerHostKey")
			c.Assert(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.StringResult{})

			*(result.(*params.StringResult)) = params.StringResult{
				Result: "key",
			}
			return nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	key, err := client.SSHServerHostKey()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.Equals, "key")
}

func (s *sshserverSuite) TestSSHServerHostKeyError(c *tc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHServer")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "SSHServerHostKey")
			c.Assert(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.StringResult{})

			*(result.(*params.StringResult)) = params.StringResult{
				Result: "",
				Error:  apiservererrors.ServerError(errors.New("blah")),
			}
			return nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = client.SSHServerHostKey()
	c.Assert(err, tc.ErrorMatches, "blah")
}

func (s *sshserverSuite) TestListPublicKeysForModel(c *tc.C) {
	key, err := pkitest.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	signer, err := gossh.NewSignerFromKey(key)
	c.Assert(err, tc.ErrorIsNil)
	pubKey := signer.PublicKey()
	authorizedKey := string(gossh.MarshalAuthorizedKey(pubKey))

	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHServer")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ListAuthorizedKeysForModel")
			c.Assert(arg, tc.FitsTypeOf, params.ListAuthorizedKeysArgs{})
			c.Assert(arg, tc.DeepEquals, params.ListAuthorizedKeysArgs{
				ModelUUID: "abcd",
			})
			c.Assert(result, tc.FitsTypeOf, &params.ListAuthorizedKeysResult{})
			*(result.(*params.ListAuthorizedKeysResult)) = params.ListAuthorizedKeysResult{
				AuthorizedKeys: []string{authorizedKey},
			}
			return nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	publicKeys, err := client.ListPublicKeysForModel(params.ListAuthorizedKeysArgs{
		ModelUUID: "abcd",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(publicKeys, tc.DeepEquals, []gossh.PublicKey{pubKey})
}

func (s *sshserverSuite) TestVirtualHostKey(c *tc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SSHServer")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "VirtualHostKey")
			c.Assert(arg, tc.FitsTypeOf, params.SSHVirtualHostKeyRequestArg{})
			c.Assert(result, tc.FitsTypeOf, &params.SSHHostKeyResult{})

			*(result.(*params.SSHHostKeyResult)) = params.SSHHostKeyResult{
				HostKey: []byte("key"),
			}
			return nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	key, err := client.VirtualHostKey(params.SSHVirtualHostKeyRequestArg{Hostname: "host"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.DeepEquals, []byte("key"))
}
