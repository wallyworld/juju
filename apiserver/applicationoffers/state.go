// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// StatePool provides the subset of a state pool.
type StatePool interface {
	// Get returns a State for a given model from the pool.
	Get(modelUUID string) (Backend, func(), error)
}

var GetStatePool = func(sp *state.StatePool) StatePool {
	return &statePoolShim{sp}

}

type statePoolShim struct {
	*state.StatePool
}

func (pool statePoolShim) Get(modelUUID string) (Backend, func(), error) {
	st, closer, err := pool.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &stateShim{st}, closer, nil
}

// Backend provides selected methods off the state.State struct.
type Backend interface {
	ControllerTag() names.ControllerTag
	Charm(*charm.URL) (Charm, error)
	AddRemoteApplication(args state.AddRemoteApplicationParams) (RemoteApplication, error)
	RemoteApplication(name string) (RemoteApplication, error)
	Application(name string) (Application, error)
	ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error)
	Model() (Model, error)
	AllModels() ([]Model, error)
	ModelUUID() string
	ModelTag() names.ModelTag
	RemoteConnectionStatus(offerName string) (RemoteConnectionStatus, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)

	GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error)
	CreateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error
	UpdateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error
	RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error
}

var GetStateAccess = func(st *state.State) Backend {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) NewStorage() storage.Storage {
	return storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
}

func (s *stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	return &modelShim{m}, err
}

func (s *stateShim) AllModels() ([]Model, error) {
	all, err := s.State.AllModels()
	if err != nil {
		return nil, err
	}
	var result []Model
	for _, m := range all {
		result = append(result, &modelShim{m})
	}
	return result, err
}

func (s *stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	app, err := s.State.RemoteApplication(name)
	return &remoteApplicationShim{app}, err
}

func (s *stateShim) AddRemoteApplication(args state.AddRemoteApplicationParams) (RemoteApplication, error) {
	app, err := s.State.AddRemoteApplication(args)
	return &remoteApplicationShim{app}, err
}

type stateCharmShim struct {
	*state.Charm
}

func (s stateShim) Charm(curl *charm.URL) (Charm, error) {
	ch, err := s.State.Charm(curl)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{ch}, nil
}

func (s *stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	return &applicationShim{app}, err
}

func (s *stateShim) ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.State)
	return offers.ApplicationOffer(name)
}

var GetApplicationOffers = func(backend interface{}) crossmodel.ApplicationOffers {
	switch st := backend.(type) {
	case *state.State:
		return state.NewApplicationOffers(st)
	case *stateShim:
		return state.NewApplicationOffers(st.State)
	}
	return nil
}

type Application interface {
	Charm() (ch Charm, force bool, err error)
	CharmURL() (curl *charm.URL, force bool)
	Name() string
	Endpoints() ([]state.Endpoint, error)
}

type applicationShim struct {
	*state.Application
}

func (a *applicationShim) Charm() (ch Charm, force bool, err error) {
	return a.Application.Charm()
}

type remoteApplicationShim struct {
	*state.RemoteApplication
}

type RemoteApplication interface {
	Name() string
	SourceModel() names.ModelTag
	Endpoints() ([]state.Endpoint, error)
	AddEndpoints(eps []charm.Relation) error
}

type Charm interface {
	Meta() *charm.Meta
	StoragePath() string
}

type Model interface {
	UUID() string
	ModelTag() names.ModelTag
	Name() string
	Owner() names.UserTag
}

type modelShim struct {
	*state.Model
}

func (s *stateShim) RemoteConnectionStatus(offerName string) (RemoteConnectionStatus, error) {
	status, err := s.State.RemoteConnectionStatus(offerName)
	return &remoteConnectionStatusShim{status}, err
}

type RemoteConnectionStatus interface {
	ConnectionCount() int
}

type remoteConnectionStatusShim struct {
	*state.RemoteConnectionStatus
}
