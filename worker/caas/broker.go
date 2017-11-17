// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.caas")

// ConfigObserver exposes a model configuration and a watch constructor
// that allows clients to be informed of changes to the configuration.
type ConfigObserver interface {
	CloudSpec() (environs.CloudSpec, error)
}

// Config describes the dependencies of a Tracker.
//
// It's arguable that it should be called TrackerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	Observer      ConfigObserver
	NewBrokerFunc caas.NewBrokerFunc
}

// Validate returns an error if the config cannot be used to start a Tracker.
func (config Config) Validate() error {
	if config.Observer == nil {
		return errors.NotValidf("nil Observer")
	}
	if config.NewBrokerFunc == nil {
		return errors.NotValidf("nil NewBrokerFunc")
	}
	return nil
}

// Tracker loads an environment, makes it available to clients, and updates
// the environment in response to config changes until it is killed.
type Tracker struct {
	config   Config
	catacomb catacomb.Catacomb
	broker   caas.Broker
}

// NewTracker returns a new Tracker, or an error if anything goes wrong.
// If a tracker is returned, its Broker() method is immediately usable.
//
// The caller is responsible for Kill()ing the returned Tracker and Wait()ing
// for any errors it might return.
func NewTracker(config Config) (*Tracker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec, err := config.Observer.CloudSpec()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get cloud information")
	}
	broker, err := config.NewBrokerFunc(cloudSpec)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create caas broker")
	}

	t := &Tracker{
		config: config,
		broker: broker,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Broker returns the encapsulated Broker. It will continue to be updated in
// the background for as long as the Tracker continues to run.
func (t *Tracker) Broker() caas.Broker {
	return t.broker
}

func (t *Tracker) loop() error {
	// TODO(caas) - watch for config and credential changes
	for {
		logger.Debugf("waiting for config and credential notifications")
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (t *Tracker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Tracker) Wait() error {
	return t.catacomb.Wait()
}
