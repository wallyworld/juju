// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/pubsub/centralhub"
)

type CentralHubSuite struct{}

func TestCentralHubSuite(t *tctesting.T) {
	tc.Run(t, &CentralHubSuite{})
}

func (*CentralHubSuite) waitForSubscribers(c *tc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("subscribers not finished")
	}
}

func (s *CentralHubSuite) TestSetsOrigin(c *tc.C) {
	hub := centralhub.New(names.NewControllerAgentTag("42"), centralhub.PubsubNoOpMetrics{})
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, tc.Equals, topic)
		expected := map[string]interface{}{
			"key":    "value",
			"origin": "controller-42",
		}
		c.Check(data, tc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	done, err := hub.Publish(topic, map[string]interface{}{"key": "value"})
	c.Assert(err, tc.ErrorIsNil)
	s.waitForSubscribers(c, pubsub.Wait(done))
	c.Assert(called, tc.IsTrue)
}

type IntStruct struct {
	Key int `json:"key"`
}

func (s *CentralHubSuite) TestYAMLMarshalling(c *tc.C) {
	hub := centralhub.New(names.NewMachineTag("42"), centralhub.PubsubNoOpMetrics{})
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, tc.Equals, topic)
		expected := map[string]interface{}{
			"key":    1234,
			"origin": "machine-42",
		}
		c.Check(data, tc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// With the default JSON marshalling, integers are marshalled to floats into the map.
	done, err := hub.Publish(topic, IntStruct{1234})
	c.Assert(err, tc.ErrorIsNil)
	s.waitForSubscribers(c, pubsub.Wait(done))
	c.Assert(called, tc.IsTrue)
}

type NestedStruct struct {
	Key    string    `yaml:"key"`
	Nested IntStruct `yaml:"nested"`
}

func (s *CentralHubSuite) TestPostProcessingMaps(c *tc.C) {
	// Due to the need to send the resulting maps over the API, nested structs
	// need to be map[string]interface{} not map[interface{}]interface{},
	// which is what the YAML marshaller will give us.
	hub := centralhub.New(names.NewMachineTag("42"), centralhub.PubsubNoOpMetrics{})
	topic := "testing"
	var called bool
	unsub, err := hub.SubscribeMatch(pubsub.MatchAll, func(t string, data map[string]interface{}) {
		c.Check(t, tc.Equals, topic)
		expected := map[string]interface{}{
			"key": "value",
			"nested": map[string]interface{}{
				"key": 1234,
			},
			"origin": "machine-42",
		}
		c.Check(data, tc.DeepEquals, expected)
		called = true
	})

	c.Assert(err, tc.ErrorIsNil)
	defer unsub()

	// With the default JSON marshalling, integers are marshalled to floats into the map.
	done, err := hub.Publish(topic, NestedStruct{
		Key:    "value",
		Nested: IntStruct{1234}})
	c.Assert(err, tc.ErrorIsNil)
	s.waitForSubscribers(c, pubsub.Wait(done))
	c.Assert(called, tc.IsTrue)
}
