// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/internal/testing"
)

type eventQueueSuite struct {
	baseSuite
}

func TestEventQueueSuite(t *tctesting.T) {
	tc.Run(t, &eventQueueSuite{})
}

func (s *eventQueueSuite) TestSubscribe(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).AnyTimes()

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestDispatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")
	s.dispatchEvent(c, changes)

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), tc.DeepEquals, changestream.Create)
		c.Assert(event.Namespace(), tc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeDuringDispatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")
	s.dispatchEvent(c, changes)

	select {
	case <-sub.Changes():
		s.unsubscribe(c, sub)
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestMultipleDispatch(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update))
}

func (s *eventQueueSuite) TestDispatchWithNoOptions(c *tc.C) {
	s.testMultipleDispatch(c)
}

func (s *eventQueueSuite) TestMultipleDispatchWithMultipleMasks(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Create|changestream.Update))
}

func (s *eventQueueSuite) TestMultipleDispatchWithMultipleOptions(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Create))
}

func (s *eventQueueSuite) TestMultipleDispatchWithOverlappingOptions(c *tc.C) {
	s.testMultipleDispatch(c, changestream.Namespace("topic", changestream.Update), changestream.Namespace("topic", changestream.Update|changestream.Create))
}

func (s *eventQueueSuite) TestSubscribeWithMatchingFilter(c *tc.C) {
	s.testMultipleDispatch(c, changestream.FilteredNamespace("topic", changestream.Update, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
}

func (s *eventQueueSuite) testMultipleDispatch(c *tc.C, opts ...changestream.SubscriptionOption) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	s.expectChangeEvent(changestream.Update, "topic")

	subs := make([]changestream.Subscription, 10)
	for i := 0; i < len(subs); i++ {
		sub, err := queue.Subscribe(opts...)
		c.Assert(err, tc.ErrorIsNil)

		subs[i] = sub
	}

	done := s.dispatchEvent(c, changes)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for dispatching event")
	}

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			defer wg.Done()

			select {
			case event := <-sub.Changes():
				c.Assert(event.Type(), tc.DeepEquals, changestream.Update)
				c.Assert(event.Namespace(), tc.DeepEquals, "topic")
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		s.unsubscribe(c, sub)
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")
	s.dispatchEvent(c, changes)

	select {
	case <-sub.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	s.unsubscribe(c, sub)
	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestTopicDoesNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.changeEvent.EXPECT().Namespace().Return("foo").MinTimes(1)

	done := s.dispatchEvent(c, changes)
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	s.unsubscribe(c, sub)

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestTopicMatchesOne(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub0, err := queue.Subscribe(changestream.Namespace("foo", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	sub1, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")
	done := s.dispatchEvent(c, changes)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub1.Changes():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	select {
	case <-sub0.Changes():
		c.Fatal("unexpected event on sub0")
	case <-time.After(time.Second):
	}

	s.unsubscribe(c, sub0)
	s.unsubscribe(c, sub1)

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestSubscriptionDoneWhenEventQueueKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, tc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")
	done := s.dispatchEvent(c, changes)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	workertest.CleanKill(c, queue)

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *eventQueueSuite) TestUnsubscribeOfOtherSubscription(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {

		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectChangeEvent(changestream.Create, "topic")
	s.dispatchEvent(c, changes)

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			defer wg.Done()

			select {
			case <-sub.Changes():
				subs[len(subs)-1-i].Unsubscribe()
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeOfOtherSubscriptionInAnotherGoroutine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue, err := New(s.stream, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, queue)

	subs := make([]changestream.Subscription, 2)
	for i := 0; i < len(subs); i++ {

		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, tc.ErrorIsNil)
		subs[i] = sub
	}

	s.expectChangeEvent(changestream.Create, "topic")
	s.dispatchEvent(c, changes)

	// The subscriptions are guaranteed to be out of order, so we need to just
	// wait on them all, and then check that they all got the event.
	wg := newWaitGroup(uint64(len(subs)))
	for i, sub := range subs {
		go func(sub changestream.Subscription, i int) {
			select {
			case <-sub.Changes():
				go func() {
					defer wg.Done()

					subs[len(subs)-1-i].Unsubscribe()
				}()
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out waiting for sub %d event", i)
			}
		}(sub, i)
	}

	select {
	case <-wg.Wait():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}

	for _, sub := range subs {
		select {
		case <-sub.Done():
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for event")
		}
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) unsubscribe(c *tc.C, sub changestream.Subscription) {
	sub.Unsubscribe()

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}
}
