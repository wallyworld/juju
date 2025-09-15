// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder_test

import (
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/utils/stringforwarder"
)

type StringForwarderSuite struct{}

func TestStringForwarderSuite(t *tctesting.T) {
	tc.Run(t, &StringForwarderSuite{})
}

// waitFor event to happen, or timeout and fail the test
func waitFor(c *tc.C, event <-chan struct{}) {
	select {
	case <-event:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for event")
	}
}

// sendEvent will send a message on a channel, or timeout if the channel is
// never available and fail the test.
func sendEvent(c *tc.C, event chan struct{}) {
	select {
	case event <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("failed to send the event")
	}
}

func (*StringForwarderSuite) TestReceives(c *tc.C) {
	var messages []string
	received := make(chan struct{}, 10)
	forwarder := stringforwarder.New(func(msg string) {
		messages = append(messages, msg)
		received <- struct{}{}
	})
	forwarder.Forward("one")
	waitFor(c, received)
	c.Check(forwarder.Stop(), tc.Equals, uint64(0))
	c.Check(messages, tc.DeepEquals, []string{"one"})
}

func noopCallback(string) {
}

func (*StringForwarderSuite) TestStopIsReentrant(c *tc.C) {
	forwarder := stringforwarder.New(noopCallback)
	forwarder.Stop()
	forwarder.Stop()
}

func (*StringForwarderSuite) TestMessagesDroppedAfterStop(c *tc.C) {
	var messages []string
	forwarder := stringforwarder.New(func(msg string) {
		messages = append(messages, msg)
	})
	forwarder.Stop()
	forwarder.Forward("one")
	forwarder.Forward("two")
	forwarder.Stop()
	c.Check(messages, tc.HasLen, 0)
}

func (*StringForwarderSuite) TestAllDroppedWithNoCallback(c *tc.C) {
	forwarder := stringforwarder.New(nil)
	forwarder.Forward("one")
	forwarder.Forward("two")
	forwarder.Forward("three")
	c.Check(forwarder.Stop(), tc.Equals, uint64(3))
}

func (*StringForwarderSuite) TestMessagesDroppedWhenBusy(c *tc.C) {
	var messages []string
	received := make(chan struct{}, 10)
	next := make(chan struct{})
	blockingCallback := func(msg string) {
		waitFor(c, next)
		messages = append(messages, msg)
		sendEvent(c, received)
	}
	forwarder := stringforwarder.New(blockingCallback)
	forwarder.Forward("first")
	forwarder.Forward("second")
	forwarder.Forward("third")
	// At this point we should have started processing "first", but the
	// other two messages are dropped.
	sendEvent(c, next)
	waitFor(c, received)
	// now we should be ready to get another message
	forwarder.Forward("fourth")
	forwarder.Forward("fifth")
	// finish fourth
	sendEvent(c, next)
	waitFor(c, received)
	dropCount := forwarder.Stop()
	c.Check(messages, tc.DeepEquals, []string{"first", "fourth"})
	c.Check(dropCount, tc.Equals, uint64(3))
}

func (*StringForwarderSuite) TestRace(c *tc.C) {
	forwarder := stringforwarder.New(noopCallback)
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	f := func(wg *sync.WaitGroup) {
		wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				forwarder.Forward("next message")
			}
		}
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go f(wg)
	}
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	close(stop)
	count := forwarder.Stop()
	c.Check(count, tc.GreaterThan, uint64(0))
}

func (*StringForwarderSuite) TestSchedulerSensitivity(c *tc.C) {
	var wg sync.WaitGroup
	f := func() {
		defer wg.Done()
		forwarder := stringforwarder.New(noopCallback)
		forwarder.Forward("msg")
		n := forwarder.Stop()
		c.Check(n, tc.Equals, uint64(0))
	}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go f()
	}
	wg.Wait()
}
