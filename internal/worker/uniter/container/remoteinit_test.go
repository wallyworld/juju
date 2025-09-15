// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/uniter/container"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type containerSuite struct{}

func TestContainerSuite(t *tctesting.T) {
	tc.Run(t, &containerSuite{})
}

func (s *containerSuite) TestNoRemoteInitRequired(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{}
	remoteState := remotestate.Snapshot{}
	_, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRunningStatusNil(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{}
	_, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRemoteInitRequiredContinue(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredRunHookPending(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredRunHookNotPending(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	_, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRemoteInitRequiredAndPending(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RemoteInit,
			Step: operation.Pending,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredAndDone(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RemoteInit,
			Step: operation.Done,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "skip remote init")
}

func (s *containerSuite) TestReinit(c *tc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	t := time.Now()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     false,
			InitialisingTime: t,
			PodName:          "pod-name",
			Running:          true,
		},
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: t.Add(time.Second),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "remote init")
}
