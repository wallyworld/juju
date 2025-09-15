// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite
}

func TestModelMachinesWatcherSuite(t *tctesting.T) {
	tc.Run(t, &modelMachinesWatcherSuite{})
}

type fakeModelMachinesWatcher struct {
	state.ModelMachinesWatcher
	initial []string
}

func (f *fakeModelMachinesWatcher) WatchModelMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes}
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{initial: []string{"foo"}},
		resources,
		authorizer,
	)
	result, err := e.WatchModelMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{"1", []string{"foo"}, nil})
	c.Assert(resources.Count(), tc.Equals, 1)
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{},
		resources,
		authorizer,
	)
	_, err := e.WatchModelMachines()
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), tc.Equals, 0)
}
