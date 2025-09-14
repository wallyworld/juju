// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path"
	"runtime"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type createSuite struct {
	LegacySuite
}

const backupfileRegex = "juju-backup-\\d{8}-\\d{6}.tar.gz"

func TestCreateSuite(t *tctesting.T) {
	tc.Run(t, &createSuite{})
} // Register the suite.

type TestDBDumper struct {
	DumpDir string
}

func (d *TestDBDumper) Dump(dumpDir string) error {
	d.DumpDir = dumpDir
	return nil
}

func (d *TestDBDumper) IsSnap() bool {
	return false
}

func (s *createSuite) TestLegacy(c *tc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Currently does not work on windows, see comments inside backups.create function")
	}
	meta := backupstesting.NewMetadataStarted()
	metadataFile, err := meta.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	backupDir := c.MkDir()
	_, testFiles, expected := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, metadataFile)
	result, err := backups.Create(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	archiveFile, size, checksum, _ := backups.ExposeCreateResult(result)
	c.Assert(archiveFile, tc.NotNil)

	// Check the result.
	file, ok := archiveFile.(*os.File)
	c.Assert(ok, tc.IsTrue)

	s.checkSize(c, file, size)
	s.checkChecksum(c, file, checksum)
	s.checkArchive(c, file, expected)
}

func (s *createSuite) TestMetadataFileMissing(c *tc.C) {
	var backupDir string
	var testFiles []string
	dumper := &TestDBDumper{}

	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, nil)
	_, err := backups.Create(args)

	c.Check(err, tc.ErrorMatches, "missing metadataReader")
}

func (s *createSuite) TestCreate(c *tc.C) {
	meta := backupstesting.NewMetadataStarted()
	metadataFile, err := meta.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	backupDir := c.MkDir()
	_, testFiles, _ := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, metadataFile)
	result, err := backups.Create(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	_, _, _, resultFilename := backups.ExposeCreateResult(result)
	dir, filename := path.Split(resultFilename)
	c.Assert(filename, tc.Matches, backupfileRegex)
	c.Assert(dir, tc.Contains, backupDir)
	_, err = os.Stat(resultFilename)
	c.Assert(err, tc.ErrorIsNil)
}
