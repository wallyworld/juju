// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import "io"

const (
	SyslogName   = "syslog"
	DatabaseName = "database"
)

// LoggerCloser is a Logger that can be closed.
type LoggerCloser interface {
	LogWriter
	io.Closer
}

// LoggersConfig defines a set of loggers that can be used to construct the
// final loggers.
type LoggersConfig struct {
	SysLogger func() LogWriter
	DBLogger  func() LogWriter
}

// MakeLoggers creates loggers from a given LoggersConfig.
func MakeLoggers(outputs []string, config LoggersConfig) LoggerCloser {
	results := make(map[string]LogWriter)
loop:
	for _, output := range outputs {
		switch output {
		case SyslogName:
			results[SyslogName] = config.SysLogger()
		default:
			// We only ever want one db logger.
			if _, ok := results[DatabaseName]; ok {
				continue loop
			}
			results[DatabaseName] = config.DBLogger()
		}
	}
	resultSlice := make([]LogWriter, 0, len(results))
	for _, output := range results {
		resultSlice = append(resultSlice, output)
	}
	return NewTeeLogger(resultSlice...)
}
