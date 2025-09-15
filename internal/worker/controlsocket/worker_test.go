// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	tctesting "testing"

	"github.com/juju/tc"
)

type workerSuite struct {
	logger *fakeLogger
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.logger = &fakeLogger{}
}

func (s *workerSuite) TestStartStopWorker(c *tc.C) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	worker, err := NewWorker(Config{
		State:      &fakeState{},
		Logger:     s.logger,
		SocketName: socket,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check socket is created with correct permissions
	fi, err := os.Stat(socket)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fi.Mode(), tc.Equals, fs.ModeSocket|0700)

	// Check server is up
	cl := client(socket)
	resp, err := cl.Get("http://a/foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, tc.ErrorIsNil)

	// Check server has stopped
	resp, err = cl.Get("http://a/foo")
	c.Assert(err, tc.ErrorMatches, ".*connection refused")

	// No warnings/errors should have been logged
	for _, entry := range s.logger.entries {
		if entry.level == "ERROR" || entry.level == "WARNING" {
			c.Errorf("%s: %s", entry.level, entry.msg)
		}
	}
}

type fakeLogger struct {
	entries []logEntry
}

type logEntry struct{ level, msg string }

func (f *fakeLogger) write(level string, format string, args ...any) {
	f.entries = append(f.entries, logEntry{level, fmt.Sprintf(format, args...)})
}

func (f *fakeLogger) Errorf(format string, args ...any) {
	f.write("ERROR", format, args...)
}

func (f *fakeLogger) Warningf(format string, args ...any) {
	f.write("WARNING", format, args...)
}

func (f *fakeLogger) Infof(format string, args ...any) {
	f.write("INFO", format, args...)
}

func (f *fakeLogger) Debugf(format string, args ...any) {
	f.write("DEBUG", format, args...)
}

func (f *fakeLogger) Tracef(format string, args ...any) {
	f.write("TRACE", format, args...)
}
