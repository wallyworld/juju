// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package backups

import (
	"fmt"
	"os"
	"syscall"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

func TestPathsSuite(t *tctesting.T) {
	tc.Run(t, &pathsSuite{})
}

type pathsSuite struct {
	testing.BaseSuite
}

func (s *pathsSuite) SetUpTest(c *tc.C) {
	s.PatchValue(&getMongodPath, func() (string, error) {
		return "path/to/mongod", nil
	})
}

func (s *pathsSuite) TestPathDefaultMongoExists(c *tc.C) {
	calledWithPaths := []string{}
	osStat := func(aPath string) (os.FileInfo, error) {
		calledWithPaths = append(calledWithPaths, aPath)
		return nil, nil
	}
	mongoPath, err := getMongoToolPath("tool", osStat, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mongoPath, tc.Equals, "path/to/juju-db.tool")
	c.Assert(calledWithPaths, tc.DeepEquals, []string{"path/to/juju-db.tool"})
}

func (s *pathsSuite) TestPathNoDefaultMongo(c *tc.C) {
	calledWithPaths := []string{}
	osStat := func(aPath string) (os.FileInfo, error) {
		calledWithPaths = append(calledWithPaths, aPath)
		return nil, fmt.Errorf("sorry no mongo")
	}

	calledWithLookup := []string{}
	execLookPath := func(aLookup string) (string, error) {
		calledWithLookup = append(calledWithLookup, aLookup)
		return "/a/fake/mongo/path", nil
	}

	mongoPath, err := getMongoToolPath("tool", osStat, execLookPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mongoPath, tc.Equals, "/a/fake/mongo/path")
	c.Assert(calledWithPaths, tc.DeepEquals, []string{
		"path/to/juju-db.tool",
	})
	c.Assert(calledWithLookup, tc.DeepEquals, []string{"tool"})
}

func (s *pathsSuite) TestPathSnapMongo(c *tc.C) {
	statPaths := []string{}
	mockStat := func(path string) (os.FileInfo, error) {
		statPaths = append(statPaths, path)
		switch path {
		case "path/to/juju-db.mongodump":
			return nil, nil // nil FileInfo is okay; value isn't used
		default:
			return nil, &os.PathError{Op: "mockStat", Path: path, Err: syscall.ENOENT}
		}
	}

	path, err := getMongoToolPath("mongodump", mockStat, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "path/to/juju-db.mongodump")
	c.Assert(statPaths, tc.DeepEquals, []string{
		"path/to/juju-db.mongodump",
	})
}
