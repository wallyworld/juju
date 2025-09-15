// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"io"
	"os"
	"reflect"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type instanceBrokerSuite struct {
	testhelpers.IsolationSuite
}

func TestInstanceBrokerSuite(t *tctesting.T) {
	tc.Run(t, &instanceBrokerSuite{})
}

func mockOpen(name string) (*os.File, error) {
	return os.Open(".")
}

func mockReadDirNamesInterfaces(f *os.File, n int) (names []string, err error) {
	return nil, io.EOF
}

func mockReadDirNamesNetplan(f *os.File, n int) (names []string, err error) {
	return nil, nil
}

func (s *instanceBrokerSuite) TestDefaultBridgerNetplan(c *tc.C) {
	s.PatchValue(&openFunc, mockOpen)
	s.PatchValue(&readDirFunc, mockReadDirNamesNetplan)

	bridger, err := defaultBridger()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reflect.TypeOf(bridger).Elem().Name(), tc.Equals, "netplanBridger")
}

func (s *instanceBrokerSuite) TestDefaultBridgerInterfaces(c *tc.C) {
	s.PatchValue(&openFunc, mockOpen)
	s.PatchValue(&readDirFunc, mockReadDirNamesInterfaces)

	bridger, err := defaultBridger()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reflect.TypeOf(bridger).Elem().Name(), tc.Equals, "etcNetworkInterfacesBridger")
}
