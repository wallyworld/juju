// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/testing"
)

type OperatorSuite struct {
	testing.BaseSuite
}

func TestOperatorSuite(t *tctesting.T) {
	tc.Run(t, &OperatorSuite{})
}

func (s *OperatorSuite) TestOperatorInfo(c *tc.C) {
	info := caas.OperatorInfo{
		CACert:     "ca cert",
		Cert:       "cert",
		PrivateKey: "private key",
	}
	marshaled, err := info.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	unmarshaledInfo, err := caas.UnmarshalOperatorInfo(marshaled)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*unmarshaledInfo, tc.DeepEquals, info)
}

func (s *OperatorSuite) TestOperatorClientInfo(c *tc.C) {
	info := caas.OperatorClientInfo{
		ServiceAddress: "1.2.3.4",
		Token:          "token",
	}
	marshaled, err := info.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	unmarshaledInfo, err := caas.UnmarshalOperatorClientInfo(marshaled)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*unmarshaledInfo, tc.DeepEquals, info)
}
