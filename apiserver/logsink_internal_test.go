// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/rpc/params"
)

type loggingStrategySuite struct{}

func TestLoggingStrategySuite(t *tctesting.T) {
	tc.Run(t, &loggingStrategySuite{})
}

func (s *loggingStrategySuite) TestLoggingOfDBInsertFailures(c *tc.C) {
	var logBuf bytes.Buffer
	strategy := &agentLoggingStrategy{
		recordLogger: failingRecordLogger{},
		fileLogger:   &logBuf,
	}

	err := strategy.WriteLog(params.LogRecord{
		Time:    time.Now(),
		Level:   "WARN",
		Message: "running low on resources",
	})

	// The captured DB error should be surfaced from WriteLog
	c.Assert(err, tc.ErrorMatches, ".*spawn more overlords")

	// Ensure that the DB error was also written to the sink
	c.Assert(logBuf.String(), tc.Matches, "(?m).*spawn more overlords.*")
}

type failingRecordLogger struct{}

func (failingRecordLogger) Log([]corelogger.LogRecord) error {
	return errors.New("spawn more overlords")
}
