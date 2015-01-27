// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/utils/set"
)

// storageChanged implements storage attached/detached operations.
type storageChanged struct {
	storageIds set.Strings
}

// String is part of the Operation interface.
func (s *storageChanged) String() string {
	return fmt.Sprintf("storage changed")
}

// Prepare is part of the Operation interface.
func (s *storageChanged) Prepare(state State) (*State, error) {
	if err := s.checkAlreadyDone(state); err != nil {
		return nil, err
	}
	return stateChange{
		Kind:       StorageChanged,
		Step:       Pending,
		StorageIds: s.storageIds,
	}.apply(state), nil
}

// Execute is part of the Operation interface.
func (s *storageChanged) Execute(state State) (*State, error) {
	// Nothing to do.
	return stateChange{
		Kind:       StorageChanged,
		Step:       Done,
		StorageIds: state.StorageIds,
	}.apply(state), nil
}

// Commit queues a storage-attached or storage-detaching hook for one of
// the changed storage instances. Commit does not update AttachedStorage
// in state; this is done when the storage hook is committed.
func (s *storageChanged) Commit(state State) (*State, error) {
	id, kind, ok := s.oneChange(state)
	var change *stateChange
	if !ok {
		// None of the storage instances have actually
		// changed, so clear out StorageIds now.
		change = &stateChange{
			Kind: Continue,
			Step: Done,
			Hook: &hook.Info{
				Kind: hooks.StorageAttached,
				// TODO(axw) determine what the appropriate
				// course of action is here. For now, this
				// works.
				StorageId: "fake/0",
			},
		}
	} else {
		change = &stateChange{
			Kind: RunHook,
			Step: Queued,
			Hook: &hook.Info{
				Kind:      kind,
				StorageId: id,
			},
			StorageIds: state.StorageIds,
		}
	}
	return change.apply(state), nil
}

func (s *storageChanged) oneChange(state State) (id string, kind hooks.Kind, ok bool) {
	// TODO(axw) we need to fetch the current life of the storage
	// instances that are said to have changed, and determine
	// the transition. For now, we're assuming the transition
	// is from unknown to attached.
	for id := range s.storageIds {
		if !state.AttachedStorage.Contains(id) {
			return id, hooks.StorageAttached, true
		}
	}
	return "", "", false
}

func (s *storageChanged) checkAlreadyDone(state State) error {
	if _, _, ok := s.oneChange(state); ok {
		return nil
	}
	if state.Step == Done {
		return ErrSkipExecute
	}
	return nil
}
