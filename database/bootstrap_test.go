// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/client"
	"github.com/juju/juju/internal/testhelpers"
)

type bootstrapSuite struct {
	testhelpers.IsolationSuite
}

func TestBootstrapSuite(t *tctesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestBootstrapSuccess(c *tc.C) {
	mgr := &testNodeManager{c: c}

	// check tests the variadic operation functionality
	// and ensures that bootstrap applied the DDL.
	check := func(db *sql.DB) error {
		rows, err := db.Query("SELECT COUNT(*) FROM lease_type")
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()

		if !rows.Next() {
			return errors.New("no rows in lease_type")
		}

		var count int
		err = rows.Scan(&count)
		if err != nil {
			return err
		}

		if count != 2 {
			return fmt.Errorf("expected 2 rows, got %d", count)
		}

		return nil
	}

	err := BootstrapDqlite(c.Context(), mgr, stubLogger{}, check)
	c.Assert(err, tc.ErrorIsNil)
}

type testNodeManager struct {
	c       *tc.C
	dataDir string
	port    int
}

func (f *testNodeManager) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testNodeManager) IsLoopbackPreferred() bool {
	return true
}

func (f *testNodeManager) WithPreferredCloudLocalAddressOption(network.ConfigSource) (app.Option, error) {
	return f.WithLoopbackAddressOption(), nil
}

func (f *testNodeManager) WithLoopbackAddressOption() app.Option {
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, tc.ErrorIsNil)
		f.c.Assert(l.Close(), tc.ErrorIsNil)
		f.port = l.Addr().(*net.TCPAddr).Port
	}
	return app.WithAddress(fmt.Sprintf("127.0.0.1:%d", f.port))
}

func (f *testNodeManager) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(_ client.LogLevel, msg string, args ...interface{}) {
		f.c.Logf(msg, args...)
	})
}

func (f *testNodeManager) WithTracingOption() app.Option {
	return app.WithTracing(client.LogNone)
}

func (f *testNodeManager) WithTLSOption() (app.Option, error) {
	return nil, nil
}
