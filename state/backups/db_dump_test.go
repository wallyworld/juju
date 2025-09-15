// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state/backups"
)

type dumpSuite struct {
	testing.BaseSuite

	targets    set.Strings
	dbInfo     *backups.DBInfo
	dumpDir    string
	ranCommand bool
}

func TestDumpSuite(t *tctesting.T) {
	tc.Run(t, &dumpSuite{})
} // Register the suite.

func (s *dumpSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	targets := set.NewStrings("juju", "admin")
	s.dbInfo = &backups.DBInfo{
		Address: "a", Username: "b", Password: "c",
		Targets:      targets,
		ApproxSizeMB: 100}
	s.targets = targets
	s.dumpDir = c.MkDir()
}

func (s *dumpSuite) patch(c *tc.C) {
	s.PatchValue(backups.GetMongodumpPath, func() (string, error) {
		return "bogusmongodump", nil
	})

	s.PatchValue(backups.RunCommand, func(cmd string, args ...string) error {
		s.ranCommand = true
		return nil
	})
}

func (s *dumpSuite) prepDB(c *tc.C, name string) string {
	dirName := filepath.Join(s.dumpDir, name)
	err := os.Mkdir(dirName, 0777)
	c.Assert(err, tc.ErrorIsNil)
	return dirName
}

func (s *dumpSuite) prep(c *tc.C, targetDBs ...string) backups.DBDumper {
	dumper, err := backups.NewDBDumper(s.dbInfo)
	c.Assert(err, tc.ErrorIsNil)

	// Prep each of the target databases.
	for _, dbName := range targetDBs {
		s.prepDB(c, dbName)
	}

	return dumper
}

func (s *dumpSuite) checkDBs(c *tc.C, dbNames ...string) {
	for _, dbName := range dbNames {
		_, err := os.Stat(filepath.Join(s.dumpDir, dbName))
		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *dumpSuite) checkStripped(c *tc.C, dbName string) {
	dirName := filepath.Join(s.dumpDir, dbName)
	_, err := os.Stat(dirName)
	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *dumpSuite) TestDumpRanCommand(c *tc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.ranCommand, tc.IsTrue)
}

func (s *dumpSuite) TestDumpStripped(c *tc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")
	s.prepDB(c, "backups") // ignored

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, tc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
	s.checkStripped(c, "backups")
}

func (s *dumpSuite) TestDumpStrippedAdmin(c *tc.C) {
	s.dbInfo.Targets = set.NewStrings("juju")
	s.patch(c)
	dumper := s.prep(c, "juju")
	s.prepDB(c, "backups") // ignored
	s.prepDB(c, "admin")   // ignored

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, tc.ErrorIsNil)

	s.checkDBs(c, "juju")
	s.checkStripped(c, "backups")
	s.checkStripped(c, "admin")
}

func (s *dumpSuite) TestDumpStrippedMultiple(c *tc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")
	s.prepDB(c, "backups")  // ignored
	s.prepDB(c, "presence") // ignored

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, tc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
	// Only "backups" is actually ignored when dumping.  Restore takes
	// care of removing the other ignored databases (like presence).
	s.checkDBs(c, "presence")
	s.checkStripped(c, "backups")
}

func (s *dumpSuite) TestDumpNothingIgnored(c *tc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, tc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
}
