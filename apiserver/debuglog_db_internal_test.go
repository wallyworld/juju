// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"

	corelogger "github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type debugLogDBIntSuite struct {
	coretesting.BaseSuite
	sock    *fakeDebugLogSocket
	clock   *testclock.Clock
	timeout time.Duration
}

func TestDebugLogDBIntSuite(t *tctesting.T) {
	tc.Run(t, &debugLogDBIntSuite{})
}

func (s *debugLogDBIntSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.sock = newFakeDebugLogSocket()
	s.clock = testclock.NewClock(time.Now())
	s.timeout = time.Minute
}

func (s *debugLogDBIntSuite) TestParamConversion(c *tc.C) {
	t1 := time.Date(2016, 11, 30, 10, 51, 0, 0, time.UTC)
	reqParams := debugLogParams{
		fromTheStart:  false,
		noTail:        true,
		initialLines:  11,
		startTime:     t1,
		filterLevel:   loggo.INFO,
		includeEntity: []string{"foo"},
		includeModule: []string{"bar"},
		includeLabel:  []string{"xxx"},
		excludeEntity: []string{"baz"},
		excludeModule: []string{"qux"},
		excludeLabel:  []string{"yyy"},
	}

	called := false
	s.PatchValue(&newLogTailer, func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
		called = true

		// Start time will be used once the client is extended to send
		// time range arguments.
		c.Assert(params.StartTime, tc.Equals, t1)
		c.Assert(params.NoTail, tc.IsTrue)
		c.Assert(params.MinLevel, tc.Equals, loggo.INFO)
		c.Assert(params.InitialLines, tc.Equals, 11)
		c.Assert(params.IncludeEntity, tc.DeepEquals, []string{"foo"})
		c.Assert(params.IncludeModule, tc.DeepEquals, []string{"bar"})
		c.Assert(params.IncludeLabel, tc.DeepEquals, []string{"xxx"})
		c.Assert(params.ExcludeEntity, tc.DeepEquals, []string{"baz"})
		c.Assert(params.ExcludeModule, tc.DeepEquals, []string{"qux"})
		c.Assert(params.ExcludeLabel, tc.DeepEquals, []string{"yyy"})

		return newFakeLogTailer(), nil
	})

	stop := make(chan struct{})
	close(stop) // Stop the request immediately.
	err := handleDebugLogDBRequest(s.clock, s.timeout, nil, reqParams, s.sock, stop, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *debugLogDBIntSuite) TestParamConversionReplay(c *tc.C) {
	reqParams := debugLogParams{
		fromTheStart: true,
		initialLines: 123,
	}

	called := false
	s.PatchValue(&newLogTailer, func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
		called = true

		c.Assert(params.StartTime.IsZero(), tc.IsTrue)
		c.Assert(params.InitialLines, tc.Equals, 123)

		return newFakeLogTailer(), nil
	})

	stop := make(chan struct{})
	close(stop) // Stop the request immediately.
	err := handleDebugLogDBRequest(s.clock, s.timeout, nil, reqParams, s.sock, nil, stop)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *debugLogDBIntSuite) TestFullRequest(c *tc.C) {
	// Set up a fake log tailer with a 2 log records ready to send.
	tailer := newFakeLogTailer()
	tailer.logsCh <- &corelogger.LogRecord{
		Time:     time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
		Entity:   "machine-99",
		Module:   "some.where",
		Location: "code.go:42",
		Level:    loggo.INFO,
		Message:  "stuff happened",
	}
	tailer.logsCh <- &corelogger.LogRecord{
		Time:     time.Date(2015, 6, 19, 15, 36, 40, 0, time.UTC),
		Entity:   "unit-foo-2",
		Module:   "else.where",
		Location: "go.go:22",
		Level:    loggo.ERROR,
		Message:  "whoops",
	}
	s.PatchValue(
		&newLogTailer,
		func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
			return tailer, nil
		},
	)

	stop := make(chan struct{})
	done := s.runRequest(debugLogParams{}, stop)

	s.assertOutput(c, []string{
		"ok", // sendOk() call needs to happen first.
		"machine-99: 2015-06-19 15:34:37 INFO some.where code.go:42 stuff happened\n",
		"unit-foo-2: 2015-06-19 15:36:40 ERROR else.where go.go:22 whoops\n",
	})

	// Check the request stops when requested.
	close(stop)
	s.assertStops(c, done, tailer)
}

func (s *debugLogDBIntSuite) TestTimeout(c *tc.C) {
	// Set up a fake log tailer with a 2 log records ready to send.
	tailer := newFakeLogTailer()
	tailer.logsCh <- &corelogger.LogRecord{
		Time:     time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
		Entity:   "machine-99",
		Module:   "some.where",
		Location: "code.go:42",
		Level:    loggo.INFO,
		Message:  "stuff happened",
	}
	tailer.logsCh <- &corelogger.LogRecord{
		Time:     time.Date(2015, 6, 19, 15, 36, 40, 0, time.UTC),
		Entity:   "unit-foo-2",
		Module:   "else.where",
		Location: "go.go:22",
		Level:    loggo.ERROR,
		Message:  "whoops",
	}
	s.PatchValue(&newLogTailer, func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
		return tailer, nil
	})

	stop := make(chan struct{})
	done := s.runRequest(debugLogParams{}, stop)

	s.assertOutput(c, []string{
		"ok", // sendOk() call needs to happen first.
		"machine-99: 2015-06-19 15:34:37 INFO some.where code.go:42 stuff happened\n",
		"unit-foo-2: 2015-06-19 15:36:40 ERROR else.where go.go:22 whoops\n",
	})

	s.assertRunning(c, done, tailer)
	s.clock.Advance(s.timeout)

	// Check the request stops when requested.
	s.assertStops(c, done, tailer)
}

func (s *debugLogDBIntSuite) TestRequestStopsWhenTailerStops(c *tc.C) {
	tailer := newFakeLogTailer()
	s.PatchValue(&newLogTailer, func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
		close(tailer.logsCh) // make the request stop immediately
		return tailer, nil
	})

	err := handleDebugLogDBRequest(s.clock, s.timeout, nil, debugLogParams{}, s.sock, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tailer.stopped, tc.IsTrue)
}

func (s *debugLogDBIntSuite) TestMaxLines(c *tc.C) {
	// Set up a fake log tailer with a 5 log records ready to send.
	tailer := newFakeLogTailer()
	for i := 0; i < 5; i++ {
		tailer.logsCh <- &corelogger.LogRecord{
			Time:     time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
			Entity:   "machine-99",
			Module:   "some.where",
			Location: "code.go:42",
			Level:    loggo.INFO,
			Message:  "stuff happened",
		}
	}
	s.PatchValue(&newLogTailer, func(_ state.LogTailerState, params corelogger.LogTailerParams) (corelogger.LogTailer, error) {
		return tailer, nil
	})

	done := s.runRequest(debugLogParams{maxLines: 3}, nil)

	s.assertOutput(c, []string{
		"ok", // sendOk() call needs to happen first.
		"machine-99: 2015-06-19 15:34:37 INFO some.where code.go:42 stuff happened\n",
		"machine-99: 2015-06-19 15:34:37 INFO some.where code.go:42 stuff happened\n",
		"machine-99: 2015-06-19 15:34:37 INFO some.where code.go:42 stuff happened\n",
	})

	// The tailer should now stop by itself after the line limit was reached.
	s.assertStops(c, done, tailer)
}

func (s *debugLogDBIntSuite) runRequest(params debugLogParams, stop chan struct{}) chan error {
	done := make(chan error)
	go func() {
		done <- handleDebugLogDBRequest(s.clock, s.timeout, &fakeState{}, params, s.sock, stop, nil)
	}()
	return done
}

func (s *debugLogDBIntSuite) assertOutput(c *tc.C, expectedWrites []string) {
	timeout := time.After(coretesting.LongWait)
	for i, expectedWrite := range expectedWrites {
		select {
		case actualWrite := <-s.sock.writes:
			c.Assert(actualWrite, tc.Equals, expectedWrite)
		case <-timeout:
			c.Errorf("timed out waiting for socket write (received %d)", i)
		}
	}
}

func (s *debugLogDBIntSuite) assertStops(c *tc.C, done chan error, tailer *fakeLogTailer) {
	select {
	case err := <-done:
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(tailer.stopped, tc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Error("timed out waiting for request handler to stop")
	}
}

func (s *debugLogDBIntSuite) assertRunning(c *tc.C, done chan error, tailer *fakeLogTailer) {
	select {
	case err := <-done:
		c.Errorf("unexpected exit, %v", errors.ErrorStack(err))
	case <-time.After(coretesting.ShortWait):
		c.Assert(tailer.stopped, tc.IsFalse)
	}
}

type fakeState struct {
	state.LogTailerState
}

func newFakeLogTailer() *fakeLogTailer {
	return &fakeLogTailer{
		logsCh: make(chan *corelogger.LogRecord, 10),
	}
}

type fakeLogTailer struct {
	corelogger.LogTailer
	logsCh  chan *corelogger.LogRecord
	stopped bool
}

func (t *fakeLogTailer) Logs() <-chan *corelogger.LogRecord {
	return t.logsCh
}

func (t *fakeLogTailer) Stop() error {
	t.stopped = true
	return nil
}

func (t *fakeLogTailer) Err() error {
	return nil
}

func newFakeDebugLogSocket() *fakeDebugLogSocket {
	return &fakeDebugLogSocket{
		writes: make(chan string, 10),
	}
}

type fakeDebugLogSocket struct {
	writes chan string
}

func (s *fakeDebugLogSocket) sendOk() {
	s.writes <- "ok"
}

func (s *fakeDebugLogSocket) sendError(err error) {
	s.writes <- fmt.Sprintf("err: %v", err)
}

func (s *fakeDebugLogSocket) sendLogRecord(r *params.LogMessage) error {
	s.writes <- fmt.Sprintf("%s: %s %s %s %s %s\n",
		r.Entity,
		s.formatTime(r.Timestamp),
		r.Severity,
		r.Module,
		r.Location,
		r.Message)
	return nil
}

func (c *fakeDebugLogSocket) formatTime(t time.Time) string {
	return t.In(time.UTC).Format("2006-01-02 15:04:05")
}
