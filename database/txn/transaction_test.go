// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	"context"
	"database/sql"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/database/txn"
)

type transactionRunnerSuite struct {
	testing.ControllerSuite
}

func TestTransactionRunnerSuite(t *tctesting.T) {
	tc.Run(t, &transactionRunnerSuite{})
}

func (s *transactionRunnerSuite) TestTxn(c *tc.C) {
	runner := txn.NewTransactionRunner()

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT 1")
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *transactionRunnerSuite) TestTxnWithCancelledContext(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	runner := txn.NewTransactionRunner()

	err := runner.Txn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("should not be called")
		return nil
	})
	c.Assert(err, tc.ErrorMatches, "context canceled")
}

func (s *transactionRunnerSuite) TestTxnInserts(c *tc.C) {
	runner := txn.NewTransactionRunner()

	s.createTable(c)

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, tc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Equals, 1)
	}
}

func (s *transactionRunnerSuite) TestTxnRollback(c *tc.C) {
	runner := txn.NewTransactionRunner()

	s.createTable(c)

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, tc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Equals, 0)
	}
}

func (s *transactionRunnerSuite) TestRetryForNonRetryableError(c *tc.C) {
	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(context.TODO(), func() error {
		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")
	c.Assert(count, tc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryWithACancelledContext(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())

	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(ctx, func() error {
		defer cancel()

		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")
	c.Assert(count, tc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryForRetryableError(c *tc.C) {
	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(context.TODO(), func() error {
		count++
		return sqlite3.ErrBusy
	})
	c.Assert(err, tc.ErrorMatches, "attempt count exceeded: .*")
	c.Assert(count, tc.Equals, 250)
}

func (s *transactionRunnerSuite) createTable(c *tc.C) {
	_, err := s.DB().Exec("CREATE TEMP TABLE foo (id INT PRIMARY KEY, name VARCHAR(255))")
	c.Assert(err, tc.ErrorIsNil)
}
