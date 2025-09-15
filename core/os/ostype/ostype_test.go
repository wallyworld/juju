// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ostype

import (
	tctesting "testing"

	"github.com/juju/tc"
)

type osTypeSuite struct{}

func TestOsTypeSuite(t *tctesting.T) {
	tc.Run(t, &osTypeSuite{})
}

func (s *osTypeSuite) TestEquivalentTo(c *tc.C) {
	c.Check(Ubuntu.EquivalentTo(CentOS), tc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(GenericLinux), tc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(Ubuntu), tc.IsTrue)
	c.Check(CentOS.EquivalentTo(CentOS), tc.IsTrue)
}

func (s *osTypeSuite) TestIsLinux(c *tc.C) {
	c.Check(Ubuntu.IsLinux(), tc.IsTrue)
	c.Check(CentOS.IsLinux(), tc.IsTrue)
	c.Check(GenericLinux.IsLinux(), tc.IsTrue)

	c.Check(Windows.IsLinux(), tc.IsFalse)
	c.Check(Unknown.IsLinux(), tc.IsFalse)

	c.Check(OSX.EquivalentTo(Ubuntu), tc.IsFalse)
	c.Check(OSX.EquivalentTo(Windows), tc.IsFalse)
	c.Check(GenericLinux.EquivalentTo(OSX), tc.IsFalse)
}
