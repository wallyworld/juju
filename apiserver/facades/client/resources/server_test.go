// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/client/resources"
)

func TestFacadeSuite(t *tctesting.T) {
	tc.Run(t, &FacadeSuite{})
}

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *tc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(s.backend, func(*charm.URL) (resources.NewCharmRepository, error) { return s.factory, nil })
	c.Check(err, tc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingDataStore(c *tc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(nil, func(*charm.URL) (resources.NewCharmRepository, error) { return s.factory, nil })
	c.Check(err, tc.ErrorMatches, `missing data backend`)
}

func (s *FacadeSuite) TestNewFacadeMissingCSClientFactory(c *tc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(s.backend, nil)
	c.Check(err, tc.ErrorMatches, `missing factory for new repository`)
}
