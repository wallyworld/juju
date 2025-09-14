// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"
	"regexp"
	tctesting "testing"

	jujucharm "github.com/juju/charm/v12"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type BundlesDirSuite struct {
	testing.JujuConnSuite

	st     api.Connection
	uniter *uniter.State
}

func TestBundlesDirSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &BundlesDirSuite{})
}

func (s *BundlesDirSuite) SetUpSuite(c *tc.C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *BundlesDirSuite) TearDownSuite(c *tc.C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *BundlesDirSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Add a charm, application and unit to login to the API with.
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)

	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, tc.NotNil)
	s.uniter, err = uniter.NewFromConnection(s.st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.uniter, tc.NotNil)
}

func (s *BundlesDirSuite) TearDownTest(c *tc.C) {
	err := s.st.Close()
	c.Assert(err, tc.ErrorIsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *BundlesDirSuite) AddCharm(c *tc.C) (charm.BundleInfo, *state.Charm) {
	curl := jujucharm.MustParseURL("ch:quantal/dummy-1")
	bun := testcharms.Repo.CharmDir("dummy")
	sch, err := testing.AddCharm(s.State, curl, bun, false)
	c.Assert(err, tc.ErrorIsNil)

	apiCharm, err := s.uniter.Charm(sch.URL())
	c.Assert(err, tc.ErrorIsNil)

	return apiCharm, sch
}

type fakeBundleInfo struct {
	charm.BundleInfo
	curl   string
	sha256 string
}

func (f fakeBundleInfo) URL() string {
	if f.curl == "" {
		return f.BundleInfo.URL()
	}
	return f.curl
}

func (f fakeBundleInfo) ArchiveSha256() (string, error) {
	if f.sha256 == "" {
		return f.BundleInfo.ArchiveSha256()
	}
	return f.sha256, nil
}

func (s *BundlesDirSuite) TestGet(c *tc.C) {
	basedir := c.MkDir()
	bunsDir := filepath.Join(basedir, "random", "bundles")
	downloader := charms.NewCharmDownloader(s.st)
	d := charm.NewBundlesDir(bunsDir, downloader, loggo.GetLogger(""))

	checkDownloadsEmpty := func() {
		files, err := os.ReadDir(filepath.Join(bunsDir, "downloads"))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(files, tc.HasLen, 0)
	}

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsDir)
	c.Assert(err, tc.Satisfies, os.IsNotExist)

	// Add a charm to state that we can try to get.
	apiCharm, sch := s.AddCharm(c)

	// Try to get the charm when the content doesn't match.
	_, err = d.Read(&fakeBundleInfo{apiCharm, "", "..."}, nil)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/dummy-1" from API server: `)+`expected sha256 "...", got ".*"`)
	checkDownloadsEmpty()

	// Try to get a charm whose bundle doesn't exist.
	_, err = d.Read(&fakeBundleInfo{apiCharm, "ch:quantal/spam-1", ""}, nil)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/spam-1" from API server: `)+`.* not found`)
	checkDownloadsEmpty()

	// Get a charm whose bundle exists and whose content matches.
	ch, err := d.Read(apiCharm, nil)
	c.Assert(err, tc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(apiCharm, nil)
	c.Assert(err, tc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Check the abort chan is honoured.
	err = os.RemoveAll(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
	abort := make(chan struct{})
	close(abort)

	ch, err = d.Read(apiCharm, abort)
	c.Check(ch, tc.IsNil)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/dummy-1" from API server: download aborted`))
	checkDownloadsEmpty()
}

func assertCharm(c *tc.C, bun charm.Bundle, sch *state.Charm) {
	actual := bun.(*jujucharm.CharmArchive)
	c.Assert(actual.Revision(), tc.Equals, sch.Revision())
	c.Assert(actual.Meta(), tc.DeepEquals, sch.Meta())
	c.Assert(actual.Config(), tc.DeepEquals, sch.Config())
}

type ClearDownloadsSuite struct {
	testhelpers.IsolationSuite
}

func TestClearDownloadsSuite(t *tctesting.T) {
	tc.Run(t, &ClearDownloadsSuite{})
}

func (s *ClearDownloadsSuite) TestWorks(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "stuff"), []byte("foo"), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "thing"), []byte("bar"), 0755), tc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestEmptyOK(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), tc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestMissingOK(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
}

func checkMissing(c *tc.C, p string) {
	_, err := os.Stat(p)
	if !os.IsNotExist(err) {
		c.Fatalf("checking %s is missing: %v", p, err)
	}
}
