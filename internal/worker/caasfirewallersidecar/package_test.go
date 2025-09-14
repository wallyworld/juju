// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallersidecar

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasfirewallersidecar CAASBroker,PortMutator,ServiceUpdater

type (
	ApplicationWorkerCreator = applicationWorkerCreator
)

var (
	NewApplicationWorker = newApplicationWorker
)

func NewWorkerForTest(config Config, f ApplicationWorkerCreator) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := newFirewaller(config, f)
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}
