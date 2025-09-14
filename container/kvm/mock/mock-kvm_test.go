// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mock_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/kvm/mock"
	"github.com/juju/juju/internal/testing"
)

type MockSuite struct {
	testing.BaseSuite
}

func TestMockSuite(t *tctesting.T) {
	tc.Run(t, &MockSuite{})
}

func (*MockSuite) TestListInitiallyEmpty(c *tc.C) {
	factory := mock.MockFactory()
	containers, err := factory.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.HasLen, 0)
}

func (*MockSuite) TestNewContainersInList(c *tc.C) {
	factory := mock.MockFactory()
	added := []kvm.Container{}
	added = append(added, factory.New("first"))
	added = append(added, factory.New("second"))
	containers, err := factory.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.SameContents, added)
}

func (*MockSuite) TestContainers(c *tc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	c.Assert(container.Name(), tc.Equals, "first")
	c.Assert(container.IsRunning(), tc.IsFalse)
}

func (*MockSuite) TestContainerStoppingStoppedErrors(c *tc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Stop()
	c.Assert(err, tc.ErrorMatches, "container is not running")
}

func (*MockSuite) TestContainerStartStarts(c *tc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(container.IsRunning(), tc.IsTrue)
}

func (*MockSuite) TestContainerStartingRunningErrors(c *tc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = container.Start(kvm.StartParams{})
	c.Assert(err, tc.ErrorMatches, "container is already running")
}

func (*MockSuite) TestContainerStoppingRunningStops(c *tc.C) {
	factory := mock.MockFactory()
	container := factory.New("first")
	err := container.Start(kvm.StartParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = container.Stop()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(container.IsRunning(), tc.IsFalse)
}

func (*MockSuite) TestAddListener(c *tc.C) {
	listener := make(chan mock.Event)
	factory := mock.MockFactory()
	factory.AddListener(listener)
	c.Assert(factory.HasListener(listener), tc.IsTrue)
}

func (*MockSuite) TestRemoveFirstListener(c *tc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(first)
	c.Assert(factory.HasListener(first), tc.IsFalse)
	c.Assert(factory.HasListener(second), tc.IsTrue)
	c.Assert(factory.HasListener(third), tc.IsTrue)
}

func (*MockSuite) TestRemoveMiddleListener(c *tc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(second)
	c.Assert(factory.HasListener(first), tc.IsTrue)
	c.Assert(factory.HasListener(second), tc.IsFalse)
	c.Assert(factory.HasListener(third), tc.IsTrue)
}

func (*MockSuite) TestRemoveLastListener(c *tc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event)
	factory.AddListener(first)
	second := make(chan mock.Event)
	factory.AddListener(second)
	third := make(chan mock.Event)
	factory.AddListener(third)
	factory.RemoveListener(third)
	c.Assert(factory.HasListener(first), tc.IsTrue)
	c.Assert(factory.HasListener(second), tc.IsTrue)
	c.Assert(factory.HasListener(third), tc.IsFalse)
}

func (*MockSuite) TestEvents(c *tc.C) {
	factory := mock.MockFactory()
	listener := make(chan mock.Event, 5)
	factory.AddListener(listener)

	first := factory.New("first")
	second := factory.New("second")
	first.Start(kvm.StartParams{})
	second.Start(kvm.StartParams{})
	second.Stop()
	first.Stop()

	c.Assert(<-listener, tc.Equals, mock.Event{mock.Started, "first"})
	c.Assert(<-listener, tc.Equals, mock.Event{mock.Started, "second"})
	c.Assert(<-listener, tc.Equals, mock.Event{mock.Stopped, "second"})
	c.Assert(<-listener, tc.Equals, mock.Event{mock.Stopped, "first"})
}

func (*MockSuite) TestEventsGoToAllListeners(c *tc.C) {
	factory := mock.MockFactory()
	first := make(chan mock.Event, 5)
	factory.AddListener(first)
	second := make(chan mock.Event, 5)
	factory.AddListener(second)

	container := factory.New("container")
	container.Start(kvm.StartParams{})
	container.Stop()

	c.Assert(<-first, tc.Equals, mock.Event{mock.Started, "container"})
	c.Assert(<-second, tc.Equals, mock.Event{mock.Started, "container"})
	c.Assert(<-first, tc.Equals, mock.Event{mock.Stopped, "container"})
	c.Assert(<-second, tc.Equals, mock.Event{mock.Stopped, "container"})
}
