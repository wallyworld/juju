// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	tctesting "testing"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

func TestStagedResourceSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &StagedResourceSuite{})
}

type StagedResourceSuite struct {
	ConnSuite
}

func (s *StagedResourceSuite) assertActivate(c *tc.C, inc state.IncrementCharmModifiedVersionType) {
	ch := s.ConnSuite.AddTestingCharm(c, "starsay")
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "starsay",
		Charm: ch,
	})

	res := s.State.Resources()
	spam := newResourceFromCharm(ch, "store-resource")

	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	var err error
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, tc.ErrorIsNil)

	_, err = res.SetResource("starsay", spam.Username, spam.Resource, bytes.NewBufferString(data), inc)
	c.Assert(err, tc.ErrorIsNil)

	staged := state.StagedResourceForTest(c, s.State, spam)
	err = staged.Activate(inc)
	c.Assert(err, tc.ErrorIsNil)

	_, err = res.GetResource("starsay", "store-resource")
	c.Assert(err, tc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	if inc {
		c.Assert(app.CharmModifiedVersion(), tc.Equals, 2)
	} else {
		c.Assert(app.CharmModifiedVersion(), tc.Equals, 0)
	}
}

func (s *StagedResourceSuite) TestActivateIncrement(c *tc.C) {
	s.assertActivate(c, state.IncrementCharmModifiedVersion)
}

func (s *StagedResourceSuite) TestActivateNoIncrement(c *tc.C) {
	s.assertActivate(c, state.DoNotIncrementCharmModifiedVersion)
}
