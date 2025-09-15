// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/tc"

	"github.com/juju/juju/api/client/credentialmanager"
	"github.com/juju/juju/juju/testing"
)

// This suite only exists because no user facing calls exercise
// invalidate credential calls enough to expose serialisation bugs.
// If/when we have commands that would expose this,
// we should drop this suite and write a new command-based one.

type CredentialManagerSuite struct {
	testing.JujuConnSuite
	client *credentialmanager.Client
}

func (s *CredentialManagerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	info := s.APIInfo(c)
	userConn := s.OpenAPIAs(c, info.Tag, info.Password)

	s.client = credentialmanager.NewClient(userConn)
}

func (s *CredentialManagerSuite) TearDownTest(c *tc.C) {
	s.client.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *tc.C) {
	tag, set := s.Model.CloudCredentialTag()
	c.Assert(set, tc.IsTrue)
	credential, err := s.State.CloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credential.IsValid(), tc.IsTrue)

	c.Assert(s.client.InvalidateModelCredential("no reason really"), tc.ErrorIsNil)

	credential, err = s.State.CloudCredential(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credential.IsValid(), tc.IsFalse)
}
