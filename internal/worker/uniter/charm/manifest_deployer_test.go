// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"path/filepath"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers/filetesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/charm/mocks"
)

type ManifestDeployerSuite struct {
	testing.BaseSuite
	bundles    *bundleReader
	targetPath string
	deployer   charm.Deployer
}

func TestManifestDeployerSuite(t *tctesting.T) {
	tc.Run(t, &ManifestDeployerSuite{})
}

// because we generally use real charm bundles for testing, and charm bundling
// sets every file mode to 0755 or 0644, all our input data uses those modes as
// well.

func (s *ManifestDeployerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.bundles = &bundleReader{}
	s.targetPath = filepath.Join(c.MkDir(), "target")
	deployerPath := filepath.Join(c.MkDir(), "deployer")
	s.deployer = charm.NewManifestDeployer(s.targetPath, deployerPath, s.bundles, loggo.GetLogger("test"))
}

func (s *ManifestDeployerSuite) addMockCharm(revision int, bundle charm.Bundle) charm.BundleInfo {
	return s.bundles.AddBundle(charmURL(revision), bundle)
}

func (s *ManifestDeployerSuite) addCharm(c *tc.C, revision int, content ...filetesting.Entry) charm.BundleInfo {
	return s.bundles.AddCustomBundle(c, charmURL(revision), func(path string) {
		filetesting.Entries(content).Create(c, path)
	})
}

func (s *ManifestDeployerSuite) deployCharm(c *tc.C, revision int, content ...filetesting.Entry) charm.BundleInfo {
	info := s.addCharm(c, revision, content...)
	err := s.deployer.Stage(info, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.deployer.Deploy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertCharm(c, revision, content...)
	return info
}

func (s *ManifestDeployerSuite) assertCharm(c *tc.C, revision int, content ...filetesting.Entry) {
	url, err := charm.ReadCharmURL(filepath.Join(s.targetPath, ".juju-charm"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.Equals, charmURL(revision).String())
	filetesting.Entries(content).Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestAbortStageWhenClosed(c *tc.C) {
	info := s.addMockCharm(1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(info, abort)
	}()
	close(abort)
	err := <-errors
	c.Assert(err, tc.ErrorMatches, "charm read aborted")
}

func (s *ManifestDeployerSuite) TestDontAbortStageWhenNotClosed(c *tc.C) {
	info := s.addMockCharm(1, mockBundle{})
	abort := make(chan struct{})
	errors := make(chan error)
	stopWaiting := s.bundles.EnableWaitForAbort()
	go func() {
		errors <- s.deployer.Stage(info, abort)
	}()
	close(stopWaiting)
	err := <-errors
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifestDeployerSuite) TestDeployWithoutStage(c *tc.C) {
	err := s.deployer.Deploy()
	c.Assert(err, tc.ErrorMatches, "charm deployment failed: no charm set")
}

func (s *ManifestDeployerSuite) TestInstall(c *tc.C) {
	s.deployCharm(c, 1,
		filetesting.File{"some-file", "hello", 0644},
		filetesting.Dir{"some-dir", 0755},
		filetesting.Symlink{"some-dir/some-link", "../some-file"},
	)
}

func (s *ManifestDeployerSuite) TestUpgradeOverwrite(c *tc.C) {
	s.deployCharm(c, 1,
		filetesting.File{"some-file", "hello", 0644},
		filetesting.Dir{"some-dir", 0755},
		filetesting.File{"some-dir/another-file", "to be removed", 0755},
		filetesting.Dir{"another-dir", 0755},
		filetesting.Symlink{"another-dir/some-link", "../some-file"},
	)
	// Replace each of file, dir, and symlink with a different entry; in
	// the case of dir, checking that contained files are also removed.
	s.deployCharm(c, 2,
		filetesting.Symlink{"some-file", "no-longer-a-file"},
		filetesting.File{"some-dir", "no-longer-a-dir", 0644},
		filetesting.Dir{"another-dir", 0755},
		filetesting.Dir{"another-dir/some-link", 0755},
	)
}

func (s *ManifestDeployerSuite) TestUpgradePreserveUserFiles(c *tc.C) {
	originalCharmContent := filetesting.Entries{
		filetesting.File{"charm-file", "to-be-removed", 0644},
		filetesting.Dir{"charm-dir", 0755},
	}
	s.deployCharm(c, 1, originalCharmContent...)

	// Add user files we expect to keep to the target dir.
	preserveUserContent := filetesting.Entries{
		filetesting.File{"user-file", "to-be-preserved", 0644},
		filetesting.Dir{"user-dir", 0755},
		filetesting.File{"user-dir/user-file", "also-preserved", 0644},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be removed.
	removeUserContent := filetesting.Entries{
		filetesting.File{"charm-dir/user-file", "whoops-removed", 0755},
	}.Create(c, s.targetPath)

	// Add some user files we expect to be replaced.
	filetesting.Entries{
		filetesting.File{"replace-file", "original", 0644},
		filetesting.Dir{"replace-dir", 0755},
		filetesting.Symlink{"replace-symlink", "replace-file"},
	}.Create(c, s.targetPath)

	// Deploy an upgrade; all new content overwrites the old...
	s.deployCharm(c, 2,
		filetesting.File{"replace-file", "updated", 0644},
		filetesting.Dir{"replace-dir", 0755},
		filetesting.Symlink{"replace-symlink", "replace-dir"},
	)

	// ...and other files are preserved or removed according to
	// source and location.
	preserveUserContent.Check(c, s.targetPath)
	removeUserContent.AsRemoveds().Check(c, s.targetPath)
	originalCharmContent.AsRemoveds().Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestUpgradeConflictResolveRetrySameCharm(c *tc.C) {
	// Create base install.
	s.deployCharm(c, 1,
		filetesting.File{"shared-file", "old", 0755},
		filetesting.File{"old-file", "old", 0644},
	)

	// Create mock upgrade charm that can (claim to) fail to expand...
	failDeploy := true
	upgradeContent := filetesting.Entries{
		filetesting.File{"shared-file", "new", 0755},
		filetesting.File{"new-file", "new", 0644},
	}
	mockCharm := mockBundle{
		paths: set.NewStrings(upgradeContent.Paths()...),
		expand: func(targetPath string) error {
			upgradeContent.Create(c, targetPath)
			if failDeploy {
				return fmt.Errorf("oh noes")
			}
			return nil
		},
	}
	info := s.addMockCharm(2, mockCharm)
	err := s.deployer.Stage(info, nil)
	c.Assert(err, tc.ErrorIsNil)

	// ...and see it fail to expand. We're not too bothered about the actual
	// content of the target dir at this stage, but we do want to check it's
	// still marked as based on the original charm...
	err = s.deployer.Deploy()
	c.Assert(err, tc.Equals, charm.ErrConflict)
	s.assertCharm(c, 1)

	// ...and we want to verify that if we "fix the errors" and redeploy the
	// same charm...
	failDeploy = false
	err = s.deployer.Deploy()
	c.Assert(err, tc.ErrorIsNil)

	// ...we end up with the right stuff in play.
	s.assertCharm(c, 2, upgradeContent...)
	filetesting.Removed{"old-file"}.Check(c, s.targetPath)
}

func (s *ManifestDeployerSuite) TestUpgradeConflictRevertRetryDifferentCharm(c *tc.C) {
	// Create base install and add a user file.
	s.deployCharm(c, 1,
		filetesting.File{"shared-file", "old", 0755},
		filetesting.File{"old-file", "old", 0644},
	)
	userFile := filetesting.File{"user-file", "user", 0644}.Create(c, s.targetPath)

	// Create a charm upgrade that never works (but still writes a bunch of files),
	// and deploy it.
	badUpgradeContent := filetesting.Entries{
		filetesting.File{"shared-file", "bad", 0644},
		filetesting.File{"bad-file", "bad", 0644},
	}
	badCharm := mockBundle{
		paths: set.NewStrings(badUpgradeContent.Paths()...),
		expand: func(targetPath string) error {
			badUpgradeContent.Create(c, targetPath)
			return fmt.Errorf("oh noes")
		},
	}
	badInfo := s.addMockCharm(2, badCharm)
	err := s.deployer.Stage(badInfo, nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.deployer.Deploy()
	c.Assert(err, tc.Equals, charm.ErrConflict)

	// Create a charm upgrade that creates a bunch of different files, without
	// error, and deploy it; check user files are preserved, and nothing from
	// charm 1 or 2 is.
	s.deployCharm(c, 3,
		filetesting.File{"shared-file", "new", 0755},
		filetesting.File{"new-file", "new", 0644},
	)
	userFile.Check(c, s.targetPath)
	filetesting.Removed{"old-file"}.Check(c, s.targetPath)
	filetesting.Removed{"bad-file"}.Check(c, s.targetPath)
}

func TestRetryingBundleReaderSuite(t *tctesting.T) {
	tc.Run(t, &RetryingBundleReaderSuite{})
}

type RetryingBundleReaderSuite struct {
	bundleReader *mocks.MockBundleReader
	bundleInfo   *mocks.MockBundleInfo
	bundle       *mocks.MockBundle
	clock        *testclock.Clock
	rbr          charm.RetryingBundleReader
}

func (s *RetryingBundleReaderSuite) TestReadBundleMaxAttemptsExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.bundleInfo.EXPECT().URL().Return("ch:focal/dummy-1").AnyTimes()
	s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any()).Return(nil, errors.NotYetAvailablef("still in the oven")).AnyTimes()

	go func() {
		// We retry 10 times in total so we need to advance the clock 9
		// times to exceed the max retry attempts (the first attempt
		// does not use the clock).
		for i := 0; i < 9; i++ {
			c.Assert(s.clock.WaitAdvance(10*time.Second, time.Second, 1), tc.ErrorIsNil)
		}
	}()

	_, err := s.rbr.Read(s.bundleInfo, nil)
	c.Assert(errors.Is(err, errors.NotFound), tc.IsTrue)
}

func (s *RetryingBundleReaderSuite) TestReadBundleEventuallySucceeds(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.bundleInfo.EXPECT().URL().Return("ch:focal/dummy-1").AnyTimes()
	gomock.InOrder(
		s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any()).Return(nil, errors.NotYetAvailablef("still in the oven")),
		s.bundleReader.EXPECT().Read(gomock.Any(), gomock.Any()).Return(s.bundle, nil),
	)

	go func() {
		// The first attempt should fail; advance the clock to trigger
		// another attempt which should succeed.
		c.Assert(s.clock.WaitAdvance(10*time.Second, time.Second, 1), tc.ErrorIsNil)
	}()

	got, err := s.rbr.Read(s.bundleInfo, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, s.bundle)
}

func (s *RetryingBundleReaderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.bundleReader = mocks.NewMockBundleReader(ctrl)
	s.bundleInfo = mocks.NewMockBundleInfo(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	s.clock = testclock.NewClock(time.Now())
	s.rbr = charm.RetryingBundleReader{
		BundleReader: s.bundleReader,
		Clock:        s.clock,
		Logger:       loggo.GetLogger("test"),
	}

	return ctrl
}
