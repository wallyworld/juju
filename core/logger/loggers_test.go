// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/logger/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type LoggersSuite struct {
	testhelpers.IsolationSuite
}

func TestLoggersSuite(t *tctesting.T) {
	tc.Run(t, &LoggersSuite{})
}

func (s *LoggersSuite) TestMakeLoggersWithOneLogger(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLoggerCloser(ctrl)
	mockLogger.EXPECT().Log([]logger.LogRecord{{
		Message: "hello",
	}})

	var called bool
	loggers := logger.MakeLoggers([]string{
		logger.DatabaseName,
	}, logger.LoggersConfig{
		DBLogger: func() logger.LogWriter {
			called = true
			return mockLogger
		},
		SysLogger: func() logger.LogWriter {
			c.Fail()
			return nil
		},
	})
	c.Assert(called, tc.Equals, true)

	loggers.Log([]logger.LogRecord{{
		Message: "hello",
	}})
}

func (s *LoggersSuite) TestMakeLoggersWithMultipleLoggers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLoggerCloser(ctrl)
	mockLogger.EXPECT().Log([]logger.LogRecord{{
		Message: "hello",
	}}).Times(2)

	loggers := logger.MakeLoggers([]string{
		logger.DatabaseName,
		logger.SyslogName,
	}, logger.LoggersConfig{
		DBLogger: func() logger.LogWriter {
			return mockLogger
		},
		SysLogger: func() logger.LogWriter {
			return mockLogger
		},
	})

	loggers.Log([]logger.LogRecord{{
		Message: "hello",
	}})
}
