// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"
	"sort"
	"strings"
)

// Level represents the log level.
type Level uint32

// The severity levels. Higher values are more considered more
// important.
const (
	UNSPECIFIED Level = iota
	TRACE
	DEBUG
	INFO
	WARNING
	ERROR
	CRITICAL
)

// String implements Stringer.
func (level Level) String() string {
	switch level {
	case UNSPECIFIED:
		return "UNSPECIFIED"
	case TRACE:
		return "TRACE"
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	case CRITICAL:
		return "CRITICAL"
	default:
		return "<unknown>"
	}
}

// ParseLevelFromString returns the log level from the given string.
func ParseLevelFromString(level string) (Level, bool) {
	level = strings.ToUpper(level)
	switch level {
	case "UNSPECIFIED":
		return UNSPECIFIED, true
	case "TRACE":
		return TRACE, true
	case "DEBUG":
		return DEBUG, true
	case "INFO":
		return INFO, true
	case "WARN", "WARNING":
		return WARNING, true
	case "ERROR":
		return ERROR, true
	case "CRITICAL":
		return CRITICAL, true
	default:
		return UNSPECIFIED, false
	}
}

// Labels represents key values which are assigned to a log entry.
type Labels map[string]string

const (
	rootString = "<root>"
)

// Config represents the configuration of the loggers.
type Config map[string]Level

// String returns a logger configuration string that may be parsed
// using ParseConfigurationString.
func (c Config) String() string {
	if c == nil {
		return ""
	}
	// output in alphabetical order.
	var names []string
	for name := range c {
		names = append(names, name)
	}
	sort.Strings(names)

	var entries []string
	for _, name := range names {
		level := c[name]
		if name == "" {
			name = rootString
		}
		entry := fmt.Sprintf("%s=%s", name, level)
		entries = append(entries, entry)
	}
	return strings.Join(entries, ";")
}

// Logger is an interface that provides logging methods.
type Logger interface {
	// Criticalf logs a message at the critical level.
	Criticalf(msg string, args ...any)

	// Errorf logs a message at the error level.
	Errorf(msg string, args ...any)

	// Warningf logs a message at the warning level.
	Warningf(msg string, args ...any)

	// Infof logs a message at the info level.
	Infof(msg string, args ...any)

	// Debugf logs a message at the debug level.
	Debugf(msg string, args ...any)

	// Tracef logs a message at the trace level.
	Tracef(msg string, args ...any)

	// Logf logs information at the given level.
	// The provided arguments are assembled together into a string with
	// fmt.Sprintf.
	Logf(level Level, labels Labels, format string, args ...any)

	// IsLevelEnabled returns true if the given level is enabled for the logger.
	IsLevelEnabled(Level) bool

	// Child returns a new logger with the given name.
	Child(name string, tags ...string) Logger

	// GetChildByName returns a child logger with the given name.
	GetChildByName(name string) Logger

	// Helper marks the caller as a helper function.
	Helper()
}

// LoggerContext is an interface that provides a method to get loggers.
type LoggerContext interface {
	// GetLogger returns a logger with the given name and tags.
	GetLogger(name string, tags ...string) Logger

	// ConfigureLoggers configures loggers according to the given string
	// specification, which specifies a set of modules and their associated
	// logging levels. Loggers are colon- or semicolon-separated; each module is
	// specified as <modulename>=<level>.  White space outside of module names
	// and levels is ignored. The root module is specified with the name
	// "<root>".
	//
	// An example specification:
	//
	//  <root>=ERROR; foo.bar=WARNING
	//
	// Label matching can be applied to the loggers by providing a set of labels
	// to the function. If a logger has a label that matches the provided
	// labels, then the logger will be configured with the provided level. If
	// the logger does not have a label that matches the provided labels, then
	// the logger will not be configured. No labels will configure all loggers
	// in the specification.
	ConfigureLoggers(specification string) error

	// ResetLoggerLevels iterates through the known logging modules and sets the
	// levels of all to UNSPECIFIED, except for <root> which is set to WARNING.
	// If labels are provided, then only loggers that have the provided labels
	// will be reset.
	ResetLoggerLevels()

	// Config returns the current configuration of the Loggers. Loggers
	// with UNSPECIFIED level will not be included.
	Config() Config
}

// LogWriter provides an interface for writing log records.
type LogWriter interface {
	// Log writes the given log records to the logger's storage.
	Log([]LogRecord) error
}

// XXXX
//// LoggerContextGetter is an interface that is used to get a LoggerContext.
//type LoggerContextGetter interface {
//	// GetLoggerContext returns a LoggerContext for the given name.
//	GetLoggerContext(modelUUID model.UUID) (LoggerContext, error)
//}
