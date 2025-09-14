// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = tc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestAssignUnits(c *tc.C) {
	f := &fakeState{}
	f.results = []state.UnitAssignmentResult{{Unit: "foo/0"}}
	api := API{st: f, res: common.NewResources()}
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-foo-0"}, {Tag: "unit-bar-1"}}}
	res, err := api.AssignUnits(args)
	c.Assert(f.ids, tc.DeepEquals, []string{"foo/0", "bar/1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[1].Error, tc.ErrorMatches, `unit "unit-bar-1" not found`)
}

func (testsuite) TestWatchUnitAssignment(c *tc.C) {
	f := &fakeState{}
	api := API{st: f, res: common.NewResources()}
	f.ids = []string{"boo", "far"}
	res, err := api.WatchUnitAssignments()
	c.Assert(f.watchCalled, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Changes, tc.DeepEquals, f.ids)
}

func (testsuite) TestSetStatus(c *tc.C) {
	f := &fakeStatusSetter{
		res: params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "boo"}}}}}
	api := API{statusSetter: f}
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{Tag: "foo/0"}},
	}
	res, err := api.SetAgentStatus(args)
	c.Assert(args, tc.DeepEquals, f.args)
	c.Assert(res, tc.DeepEquals, f.res)
	c.Assert(err, tc.Equals, f.err)
}

type fakeState struct {
	watchCalled bool
	ids         []string
	results     []state.UnitAssignmentResult
	err         error
}

func (f *fakeState) WatchForUnitAssignment() state.StringsWatcher {
	f.watchCalled = true
	return fakeWatcher{f.ids}
}

func (f *fakeState) AssignStagedUnits(ids []string) ([]state.UnitAssignmentResult, error) {
	f.ids = ids
	return f.results, f.err
}

type fakeWatcher struct {
	changes []string
}

func (f fakeWatcher) Changes() <-chan []string {
	changes := make(chan []string, 1)
	changes <- f.changes
	return changes
}
func (fakeWatcher) Kill() {}

func (fakeWatcher) Wait() error { return nil }

func (fakeWatcher) Stop() error { return nil }

func (fakeWatcher) Err() error { return nil }

type fakeStatusSetter struct {
	args params.SetStatus
	res  params.ErrorResults
	err  error
}

func (f *fakeStatusSetter) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	f.args = args
	return f.res, f.err
}
