// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASBroker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/unitfacade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASUnitProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/runner_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner Runner
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/ops_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner ApplicationOps

type mockNotifyWorker struct {
	worker.Worker
	testhelpers.Stub
}

func (w *mockNotifyWorker) Notify() {
	w.MethodCall(w, "Notify")
}

type mockTomb struct {
	done chan struct{}
}

func (t *mockTomb) Dying() <-chan struct{} {
	return t.done
}

func (t *mockTomb) ErrDying() error {
	select {
	case <-t.done:
		return tomb.ErrDying
	default:
		return tomb.ErrStillAlive
	}
}
