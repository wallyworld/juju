// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/database/testing"
)

type migrationSuite struct {
	testing.DBSuite
}

func TestMigrationSuite(t *tctesting.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestMigrationSuccess(c *tc.C) {
	delta := []string{
		"CREATE TABLE band(name TEXT PRIMARY KEY);",
		"INSERT INTO band VALUES ('Blood Incantation');",
	}

	db := s.DB()
	m := NewDBMigration(db, stubLogger{}, delta)
	c.Assert(m.Apply(), tc.ErrorIsNil)

	rows, err := db.Query("SELECT * from band;")
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { _ = rows.Close() })

	var band string
	c.Assert(rows.Next(), tc.IsTrue)
	c.Assert(rows.Scan(&band), tc.ErrorIsNil)
	c.Check(band, tc.Equals, "Blood Incantation")
}
