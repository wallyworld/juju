// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type RemoteInitSuite struct {
	testhelpers.IsolationSuite
}

func TestRemoteInitSuite(t *tctesting.T) {
	tc.Run(t, &RemoteInitSuite{})
}

func (s *RemoteInitSuite) TestRemoteInit(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.Equals, abort)

	newState, err = op.Commit(*newState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestRemoteInitWithHook(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.Equals, abort)

	newState, err = op.Commit(*newState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
}

func (s *RemoteInitSuite) TestRemoteInitFail(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: errors.New("ooops"),
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, tc.ErrorMatches, "ooops")
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.Equals, abort)
}

func (s *RemoteInitSuite) TestSkipRemoteInit(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(false)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitWithHook(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(false)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Commit(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitRetry(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(true)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Commit(operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitRetryWithHook(c *tc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(true)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Execute(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, tc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, tc.IsNil)

	newState, err = op.Commit(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.LeaderElected,
		},
	})
}
