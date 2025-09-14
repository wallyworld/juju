// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"

	"github.com/juju/juju/database/driver"
	"github.com/juju/juju/database/txn"
	"github.com/juju/juju/internal/testhelpers"
)

type isErrRetryableSuite struct {
	testhelpers.IsolationSuite
}

func TestIsErrRetryableSuite(t *tctesting.T) {
	tc.Run(t, &isErrRetryableSuite{})
}

func (s *isErrRetryableSuite) TestIsErrRetryable(c *tc.C) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "driver error busy error",
			err:      &driver.Error{Code: driver.ErrBusy},
			expected: true,
		},
		{
			name:     "sqlite3 error busy error",
			err:      sqlite3.ErrBusy,
			expected: true,
		},
		{
			name:     "sqlite3 err locked",
			err:      sqlite3.ErrBusy,
			expected: true,
		},
		{
			name:     "database is locked",
			err:      errors.Errorf("database is locked"),
			expected: true,
		},
		{
			name:     "cannot start a transaction within a transaction",
			err:      errors.Errorf("cannot start a transaction within a transaction"),
			expected: true,
		},
		{
			name:     "bad connection",
			err:      errors.Errorf("bad connection"),
			expected: true,
		},
		{
			name:     "checkpoint in progress",
			err:      errors.Errorf("checkpoint in progress"),
			expected: true,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		c.Check(txn.IsErrRetryable(test.err), tc.Equals, test.expected)
	}
}
