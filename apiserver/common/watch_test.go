// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type agentEntityWatcherSuite struct{}

func TestAgentEntityWatcherSuite(t *tctesting.T) {
	tc.Run(t, &agentEntityWatcherSuite{})
}

type fakeAgentEntityWatcher struct {
	state.Entity
	fetchError
}

func (a *fakeAgentEntityWatcher) Watch() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}

func (*agentEntityWatcherSuite) TestWatch(c *tc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeAgentEntityWatcher{fetchError: "x0 fails"},
			u("x/1"): &fakeAgentEntityWatcher{},
			u("x/2"): &fakeAgentEntityWatcher{},
		},
	}
	getCanWatch := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1
		}, nil
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"},
	}}
	result, err := a.Watch(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{"1", nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*agentEntityWatcherSuite) TestWatchError(c *tc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	_, err := a.Watch(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (*agentEntityWatcherSuite) TestWatchNoArgsNoError(c *tc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	result, err := a.Watch(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

type multiNotifyWatcherSuite struct{}

func TestMultiNotifyWatcherSuite(t *tctesting.T) {
	tc.Run(t, &multiNotifyWatcherSuite{})
}

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcher(c *tc.C) {
	w0 := apiservertesting.NewFakeNotifyWatcher()
	w1 := apiservertesting.NewFakeNotifyWatcher()

	mw := common.NewMultiNotifyWatcher(w0, w1)
	defer statetesting.AssertStop(c, mw)

	wc := statetesting.NewNotifyWatcherC(c, mw)
	wc.AssertOneChange()

	w0.C <- struct{}{}
	wc.AssertOneChange()
	w1.C <- struct{}{}
	wc.AssertOneChange()

	w0.C <- struct{}{}
	w1.C <- struct{}{}
	wc.AssertOneChange()
}

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcherStop(c *tc.C) {
	w0 := apiservertesting.NewFakeNotifyWatcher()
	w1 := apiservertesting.NewFakeNotifyWatcher()

	mw := common.NewMultiNotifyWatcher(w0, w1)
	wc := statetesting.NewNotifyWatcherC(c, mw)
	wc.AssertOneChange()
	statetesting.AssertCanStopWhenSending(c, mw)
	wc.AssertClosed()
}
