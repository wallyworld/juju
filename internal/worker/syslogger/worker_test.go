// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/syslogger"
)

type WorkerSuite struct {
	stub testhelpers.Stub
}

func TestWorkerSuite(t *tctesting.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.stub.ResetCalls()
}

func (s *WorkerSuite) TestLogCreation(c *tc.C) {
	_, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			s.stub.MethodCall(s, "NewLogger", priority, tag)
			return nil, nil
		},
	})
	c.Assert(err, tc.IsNil)
	s.stub.CheckCallNames(c, strings.Split(strings.Repeat("NewLogger,", 7), ",")[:7]...)
	for _, call := range s.stub.Calls() {
		arg := call.Args[0].(syslogger.Priority)
		c.Assert(arg >= syslogger.LOG_CRIT && arg <= syslogger.LOG_DEBUG, tc.Equals, true)
	}
}

func (s *WorkerSuite) TestLog(c *tc.C) {
	now := time.Now()
	buf := new(bytes.Buffer)
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return closer{buf}, nil
		},
	})
	c.Assert(err, tc.IsNil)
	wrk := w.(syslogger.SysLogger)
	err = wrk.Log([]corelogger.LogRecord{{
		Time:      now,
		Entity:    "foo",
		Module:    "bar",
		Message:   "baz",
		ModelUUID: coretesting.ModelTag.Id(),
	}})
	c.Assert(err, tc.IsNil)

	dateTime := now.In(time.UTC).Format("2006-01-02 15:04:05")
	c.Assert(buf.String(), tc.Equals, fmt.Sprintf("%s foo bar.deadbe baz\n", dateTime))
}

func (s *WorkerSuite) TestClosingLogBeforeWriting(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockWriter := syslogger.NewMockWriteCloser(ctrl)
	mockWriter.EXPECT().Close().Times(7)

	now := time.Now()
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return mockWriter, nil
		},
	})
	c.Assert(err, tc.IsNil)

	w.Kill()
	c.Assert(w.Wait(), tc.IsNil)

	wrk := w.(syslogger.SysLogger)
	err = wrk.Log([]corelogger.LogRecord{{
		Time:      now,
		Entity:    "foo",
		Module:    "bar",
		Message:   "baz",
		ModelUUID: coretesting.ModelTag.Id(),
	}})
	c.Assert(err, tc.IsNil)
}

func (s *WorkerSuite) TestClosingLogWhilstWriting(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockWriter := syslogger.NewMockWriteCloser(ctrl)
	mockWriter.EXPECT().Write(gomock.Any()).MinTimes(1)
	mockWriter.EXPECT().Close().Times(7)

	now := time.Now()
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return mockWriter, nil
		},
	})
	c.Assert(err, tc.IsNil)

	done := make(chan struct{})
	go func() {
		c.Assert(w.Wait(), tc.IsNil)
		close(done)
	}()
	go func() {
		wrk := w.(syslogger.SysLogger)
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond):
				err = wrk.Log([]corelogger.LogRecord{{
					Time:      now,
					Entity:    "foo",
					Module:    "bar",
					Message:   "baz",
					ModelUUID: coretesting.ModelTag.Id(),
				}})
				c.Assert(err, tc.IsNil)
			}
		}
	}()
	go func() {
		<-time.After(time.Millisecond * 10)
		w.Kill()
	}()
	select {
	case <-done:
	case <-time.After(testhelpers.ShortWait):
		c.Fatal("failed waiting for test to complete")
	}
}

type closer struct {
	io.Writer
}

func (c closer) Close() error { return nil }
