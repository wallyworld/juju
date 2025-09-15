// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type UnitAccessorSuite struct {
	testhelpers.IsolationSuite
}

func TestUnitAccessorSuite(t *tctesting.T) {
	tc.Run(t, &UnitAccessorSuite{})
}

type appGetter struct {
	exits bool
}

func (a appGetter) ApplicationExists(name string) error {
	if a.exits {
		return nil
	}
	return errors.NotFoundf("application %q", name)
}

func (s *UnitAccessorSuite) TestApplicationAgent(c *tc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, tc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, tc.IsFalse)
}

func (s *UnitAccessorSuite) TestApplicationNotFound(c *tc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{false})
	_, err := getAuthFunc()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *UnitAccessorSuite) TestUnitAgent(c *tc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("gitlab/0"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, tc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewApplicationTag("gitlab"))
	c.Assert(ok, tc.IsTrue)
	ok = authFunc(names.NewUnitTag("gitlab/1"))
	c.Assert(ok, tc.IsFalse)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, tc.IsFalse)
}
