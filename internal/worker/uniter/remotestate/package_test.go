// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/worker/v3"
)

func SecretRotateWatcher(w *RemoteStateWatcher) worker.Worker {
	return w.secretRotateWatcher
}

func SecretExpiryWatcherFunc(w *RemoteStateWatcher) worker.Worker {
	return w.secretExpiryWatcher
}
