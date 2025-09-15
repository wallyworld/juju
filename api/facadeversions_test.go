// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/facades"
	coretesting "github.com/juju/juju/internal/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

func TestFacadeVersionSuite(t *tctesting.T) {
	tc.Run(t, &facadeVersionSuite{})
}

func (s *facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *tc.C) {
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name, versions := range api.SupportedFacadeVersions() {
		clientFacadeNames.Add(name)
		// All versions should now be non-zero.
		c.Check(set.NewInts(versions...).Contains(0), tc.IsFalse)
	}
	allServerFacades := apiserver.AllFacades().List()
	serverFacadeNames := set.NewStrings()
	serverFacadeBestVersions := make(facades.FacadeVersions, len(allServerFacades))
	for _, facade := range allServerFacades {
		serverFacadeNames.Add(facade.Name)
		serverFacadeBestVersions[facade.Name] = facade.Versions
	}
	// First check that both sides know about all the same versions
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), tc.HasLen, 0)
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), tc.HasLen, 0)
	// Next check that the best versions match
	c.Check(api.SupportedFacadeVersions(), tc.DeepEquals, serverFacadeBestVersions)
}

func checkBestVersion(c *tc.C, desiredVersion, versions []int, expectedVersion int) {
	resultVersion := facades.BestVersion(desiredVersion, versions)
	c.Check(resultVersion, tc.Equals, expectedVersion)
}

func (*facadeVersionSuite) TestBestVersionDesiredAvailable(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{0, 1, 2}, 0)
	checkBestVersion(c, []int{0, 1}, []int{0, 1, 2}, 1)
	checkBestVersion(c, []int{0, 1, 2}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredNewer(c *tc.C) {
	checkBestVersion(c, []int{1, 2, 3}, []int{0}, 0)
	checkBestVersion(c, []int{1, 2, 3}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredGap(c *tc.C) {
	checkBestVersion(c, []int{1}, []int{0, 2}, 0)
}

func (*facadeVersionSuite) TestBestVersionNoVersions(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{}, 0)
	checkBestVersion(c, []int{1}, []int{}, 0)
	checkBestVersion(c, []int{0}, []int(nil), 0)
	checkBestVersion(c, []int{1}, []int(nil), 0)
}

func (*facadeVersionSuite) TestBestVersionNotSorted(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{0, 3, 1, 2}, 0)
	checkBestVersion(c, []int{3}, []int{0, 3, 1, 2}, 3)
	checkBestVersion(c, []int{1}, []int{0, 3, 1, 2}, 1)
	checkBestVersion(c, []int{2}, []int{0, 3, 1, 2}, 2)
}

func (s *facadeVersionSuite) TestBestFacadeVersionExactMatch(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	st := api.NewTestingState(c, api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerServer(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	st := api.NewTestingState(c, api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1, 2},
		}})
	c.Check(st.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerClient(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1, 2}})
	st := api.NewTestingState(c, api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionServerUnknown(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"TestingAPI": {1, 2}})
	st := api.NewTestingState(c, api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("TestingAPI"), tc.Equals, 0)
}
