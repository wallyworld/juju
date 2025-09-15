// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"errors"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type BufferedLoggerSuite struct {
	testhelpers.IsolationSuite
}

func TestBufferedLoggerSuite(t *tctesting.T) {
	tc.Run(t, &BufferedLoggerSuite{})
}

func (s *BufferedLoggerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *BufferedLoggerSuite) waitFlush(c *tc.C, mock *mockLogger) []corelogger.LogRecord {
	select {
	case records := <-mock.called:
		return records
	case <-time.After(coretesting.LongWait):
	}
	c.Fatal("timed out waiting for logs to be flushed")
	panic("unreachable")
}

func (s *BufferedLoggerSuite) assertNoFlush(c *tc.C, mock *mockLogger, clock *testclock.Clock) {
	err := clock.WaitAdvance(0, 0, 0) // There should be no active timers
	c.Assert(err, tc.ErrorIsNil)
	select {
	case records := <-mock.called:
		c.Fatalf("unexpected log records: %v", records)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *BufferedLoggerSuite) TestLogFlushes(c *tc.C) {
	const bufsz = 3
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogger(&mock, bufsz, time.Minute, clock)
	in := []corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in[:2])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	err = b.Log(in[2:])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{"Log", []interface{}{in}},
	})

	err = clock.WaitAdvance(0, coretesting.LongWait, 0)
	c.Assert(err, tc.ErrorIsNil)
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLoggerSuite) TestLogFlushesMultiple(c *tc.C) {
	const bufsz = 1
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogger(&mock, bufsz, time.Minute, clock)
	in := []corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in)
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{"Log", []interface{}{in[:1]}},
		{"Log", []interface{}{in[1:2]}},
		{"Log", []interface{}{in[2:]}},
	})
}

func (s *BufferedLoggerSuite) TestTimerFlushes(c *tc.C) {
	const bufsz = 10
	const flushInterval = time.Minute
	mock := mockLogger{called: make(chan []corelogger.LogRecord)}
	clock := testclock.NewClock(time.Time{})

	b := corelogger.NewBufferedLogger(&mock, bufsz, flushInterval, clock)
	in := []corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}}

	err := b.Log(in[:1])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance, but not far enough to trigger the flush.
	clock.WaitAdvance(30*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)

	// Log again; the timer should not have been reset.
	err = b.Log(in[1:])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance to to the flush interval.
	clock.Advance(30 * time.Second)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{"Log", []interface{}{in}},
	})
	s.assertNoFlush(c, &mock, clock)
	mock.ResetCalls()

	// Logging again, the timer resets to the time at which
	// the new log records are inserted.
	err = b.Log(in)
	c.Assert(err, tc.ErrorIsNil)
	clock.WaitAdvance(59*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)
	clock.Advance(1 * time.Second)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{"Log", []interface{}{in}},
	})
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLoggerSuite) TestLogOverCapacity(c *tc.C) {
	const bufsz = 2
	const flushInterval = time.Minute
	mock := mockLogger{called: make(chan []corelogger.LogRecord, 1)}
	clock := testclock.NewClock(time.Time{})

	// The buffer has a capacity of 2, so writing 3 logs will
	// cause 2 to be flushed, with 1 remaining in the buffer
	// until the timer triggers.
	b := corelogger.NewBufferedLogger(&mock, bufsz, flushInterval, clock)
	in := []corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in[:bufsz])

	clock.WaitAdvance(time.Minute, coretesting.LongWait, 1)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in[bufsz:])

	mock.CheckCalls(c, []testhelpers.StubCall{
		{"Log", []interface{}{in[:bufsz]}},
		{"Log", []interface{}{in[bufsz:]}},
	})
}

func (s *BufferedLoggerSuite) TestFlushNothing(c *tc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogger(&mock, 1, time.Minute, clock)
	err := b.Flush()
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)
}

func (s *BufferedLoggerSuite) TestFlushReportsError(c *tc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := corelogger.NewBufferedLogger(&mock, 2, time.Minute, clock)
	err := b.Log([]corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, tc.ErrorIsNil)
	err = b.Flush()
	c.Assert(err, tc.ErrorMatches, "nope")
}

func (s *BufferedLoggerSuite) TestLogReportsError(c *tc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := corelogger.NewBufferedLogger(&mock, 1, time.Minute, clock)
	err := b.Log([]corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, tc.ErrorMatches, "nope")
}

type mockLogger struct {
	testhelpers.Stub
	called chan []corelogger.LogRecord
}

func (m *mockLogger) Log(in []corelogger.LogRecord) error {
	incopy := make([]corelogger.LogRecord, len(in))
	copy(incopy, in)
	m.MethodCall(m, "Log", incopy)
	if m.called != nil {
		m.called <- incopy
	}
	return m.NextErr()
}
