// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ModelWatcher interface {
	WatchForModelConfigChanges() (params.NotifyWatchResult, error)
	ModelConfig() (params.ModelConfigResult, error)
}

type ModelWatcherTest struct {
	modelWatcher ModelWatcher
	st           *state.State
	// We can't call this "resources" as it conflicts
	// when embedded in other test suites.
	res *common.Resources
}

func NewModelWatcherTest(
	modelWatcher ModelWatcher,
	st *state.State,
	resources *common.Resources,
) *ModelWatcherTest {
	return &ModelWatcherTest{modelWatcher, st, resources}
}

// AssertModelConfig provides a method to test the config from the
// modelWatcher.  This allows other tests that embed this type to have
// more than just the default test.
func (s *ModelWatcherTest) AssertModelConfig(c *tc.C, modelWatcher ModelWatcher) {
	model, err := s.st.Model()
	c.Assert(err, tc.ErrorIsNil)

	modelConfig, err := model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)

	result, err := modelWatcher.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)

	configAttributes := modelConfig.AllAttrs()
	c.Assert(result.Config, tc.DeepEquals, params.ModelConfig(configAttributes))
}

func (s *ModelWatcherTest) TestModelConfig(c *tc.C) {
	s.AssertModelConfig(c, s.modelWatcher)
}

func (s *ModelWatcherTest) TestWatchForModelConfigChanges(c *tc.C) {
	c.Assert(s.res.Count(), tc.Equals, 0)

	result, err := s.modelWatcher.WatchForModelConfigChanges()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.res.Count(), tc.Equals, 1)
	resource := s.res.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
