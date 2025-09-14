// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net"
	"strconv"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/internal/testhelpers"
)

// DBSuite is used to provide a Dqlite-backed sql.DB reference to tests.
type DBSuite struct {
	testhelpers.IsolationSuite

	dqlite    *app.App
	db        *sql.DB
	trackedDB coredatabase.TrackedDB
}

// SetUpSuite creates a new Dqlite application and waits for it to be ready.
func (s *DBSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)

	dbPath := c.MkDir()
	port := FindTCPPort(c)

	url := fmt.Sprintf("%s:%d", "127.0.0.1", port)
	c.Logf("Opening sqlite3 db with: %v", url)

	var err error
	s.dqlite, err = app.New(dbPath, app.WithAddress(url))
	c.Assert(err, tc.ErrorIsNil)

	err = s.dqlite.Ready(context.TODO())
	c.Assert(err, tc.ErrorIsNil)
}

// TearDownSuite terminates the Dqlite node, releasing all resources.
func (s *DBSuite) TearDownSuite(c *tc.C) {
	if s.dqlite != nil {
		err := s.dqlite.Close()
		c.Assert(err, tc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownSuite(c)
}

// SetUpTest opens a new, randomly named database and
// makes it available for use by test the next test.
func (s *DBSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.db, err = s.dqlite.Open(context.TODO(), strconv.Itoa(rand.Intn(10)))
	c.Assert(err, tc.ErrorIsNil)

	s.trackedDB = &trackedDB{
		db: s.db,
	}
}

// TearDownTest closes the database opened in SetUpTest.
// TODO (manadart 2022-09-12): There is currently no avenue for dropping a DB.
func (s *DBSuite) TearDownTest(c *tc.C) {
	if s.db != nil {
		c.Logf("Closing DB")
		err := s.db.Close()
		c.Assert(err, tc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownTest(c)
}

func (s *DBSuite) DB() *sql.DB {
	return s.db
}

func (s *DBSuite) TrackedDB() coredatabase.TrackedDB {
	return s.trackedDB
}

// FindTCPPort finds an unused TCP port and returns it.
// It is prone to racing, so the port should be used as soon as it is acquired
// to minimise the change of another process using it in the interim.
// The chances of this should be negligible during testing.
func FindTCPPort(c *tc.C) int {
	l, err := net.Listen("tcp", ":0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(l.Close(), tc.ErrorIsNil)
	return l.Addr().(*net.TCPAddr).Port
}
