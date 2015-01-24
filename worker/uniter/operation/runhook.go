// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

type runHook struct {
	info hook.Info

	callbacks     Callbacks
	runnerFactory runner.Factory

	name   string
	runner runner.Runner
}

// String is part of the Operation interface.
func (rh *runHook) String() string {
	suffix := ""
	if rh.info.Kind.IsRelation() {
		if rh.info.RemoteUnit == "" {
			suffix = fmt.Sprintf(" (%d)", rh.info.RelationId)
		} else {
			suffix = fmt.Sprintf(" (%d; %s)", rh.info.RelationId, rh.info.RemoteUnit)
		}
	}
	return fmt.Sprintf("run %s%s hook", rh.info.Kind, suffix)
}

// Prepare ensures the hook can be executed.
// Prepare is part of the Operation interface.
func (rh *runHook) Prepare(state State) (*State, error) {
	name, err := rh.callbacks.PrepareHook(rh.info)
	if err != nil {
		return nil, err
	}
	if rh.info.Kind.IsStorage() {
		// TODO(axw) add utility function to juju/names for extracting
		// storage name from ID.
		storageName := rh.info.StorageId[:strings.IndexRune(rh.info.StorageId, '/')]
		name = fmt.Sprintf("%s-%s", storageName, name)
	}
	rnr, err := rh.runnerFactory.NewHookRunner(rh.info)
	if err != nil {
		return nil, err
	}
	rh.name = name
	rh.runner = rnr
	return stateChange{
		Kind:       RunHook,
		Step:       Pending,
		Hook:       &rh.info,
		StorageIds: state.StorageIds,
	}.apply(state), nil
}

// Execute runs the hook.
// Execute is part of the Operation interface.
func (rh *runHook) Execute(state State) (*State, error) {
	message := fmt.Sprintf("running hook %s", rh.name)
	unlock, err := rh.callbacks.AcquireExecutionLock(message)
	if err != nil {
		return nil, err
	}
	defer unlock()

	ranHook := true
	step := Done

	err = rh.runner.RunHook(rh.name)
	cause := errors.Cause(err)
	switch {
	case runner.IsMissingHookError(cause):
		ranHook = false
		err = nil
	case cause == runner.ErrRequeueAndReboot:
		step = Queued
		fallthrough
	case cause == runner.ErrReboot:
		err = ErrNeedsReboot
	case err == nil:
	default:
		logger.Errorf("hook %q failed: %v", rh.name, err)
		rh.callbacks.NotifyHookFailed(rh.name, rh.runner.Context())
		return nil, ErrHookFailed
	}

	if ranHook {
		logger.Infof("ran %q hook", rh.name)
		rh.callbacks.NotifyHookCompleted(rh.name, rh.runner.Context())
	} else {
		logger.Infof("skipped %q hook (missing)", rh.name)
	}
	return stateChange{
		Kind:       RunHook,
		Step:       step,
		Hook:       &rh.info,
		StorageIds: state.StorageIds,
	}.apply(state), err
}

// Commit updates relation state to include the fact of the hook's execution,
// and records the impact of start and collect-metrics hooks.
// Commit is part of the Operation interface.
func (rh *runHook) Commit(state State) (*State, error) {
	if err := rh.callbacks.CommitHook(rh.info); err != nil {
		return nil, err
	}
	newState := stateChange{
		Kind: Continue,
		Step: Pending,
		Hook: &rh.info,
	}.apply(state)
	switch rh.info.Kind {
	case hooks.Start:
		newState.Started = true
	case hooks.CollectMetrics:
		newState.CollectMetricsTime = time.Now().Unix()
	}
	if rh.info.Kind.IsStorage() {
		// We need to update AttachedStorage and StorageIds in the
		// operation state, so we can continue firing storage hooks
		// for the remaining storage changes.
		switch rh.info.Kind {
		case hooks.StorageAttached:
			if newState.AttachedStorage == nil {
				newState.AttachedStorage = set.NewStrings(rh.info.StorageId)
			} else {
				newState.AttachedStorage.Add(rh.info.StorageId)
			}
		case hooks.StorageDetached:
			newState.AttachedStorage.Remove(rh.info.StorageId)
		}
		state.StorageIds.Remove(rh.info.StorageId)
		newState.StorageIds = state.StorageIds
	}
	return newState, nil
}
