// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type mockcloudSpecAPI struct {
	// TODO - implement when remaining firewaller tests become unit tests
	cloudspec.CloudSpecAPI
}

type mockState struct {
	// TODO - implement when remaining firewaller tests become unit tests
	state.ModelMachinesWatcher
	state.ModelAccessor

	testing.Stub
	modelUUID      string
	remoteEntities map[names.Tag]string
	applications   map[string]*mockApplication
	units          map[string]*mockUnit
	machines       map[string]*mockMachine
	relations      map[string]*mockRelation
	subnetsWatcher *mockStringsWatcher
}

func newMockState(modelUUID string) *mockState {
	return &mockState{
		modelUUID:      modelUUID,
		relations:      make(map[string]*mockRelation),
		applications:   make(map[string]*mockApplication),
		units:          make(map[string]*mockUnit),
		machines:       make(map[string]*mockMachine),
		remoteEntities: make(map[names.Tag]string),
		subnetsWatcher: newMockStringsWatcher(),
	}
}

func (st *mockState) ModelUUID() string {
	return st.modelUUID
}

func (st *mockState) Application(id string) (firewaller.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	a, ok := st.applications[id]
	if !ok {
		return nil, errors.NotFoundf("application %q", id)
	}
	return a, nil
}

func (st *mockState) Unit(name string) (firewaller.Unit, error) {
	st.MethodCall(st, "Unit", name)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	u, ok := st.units[name]
	if !ok {
		return nil, errors.NotFoundf("unit %q", name)
	}
	return u, nil
}

func (st *mockState) Machine(id string) (firewaller.Machine, error) {
	st.MethodCall(st, "Machine", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	m, ok := st.machines[id]
	if !ok {
		return nil, errors.NotFoundf("machine %q", id)
	}
	return m, nil
}

func (st *mockState) WatchSubnets(func(id interface{}) bool) state.StringsWatcher {
	st.MethodCall(st, "WatchSubnets")
	return st.subnetsWatcher
}

func (st *mockState) WatchOpenedPorts() state.StringsWatcher {
	st.MethodCall(st, "WatchOpenedPorts")
	// TODO - implement when remaining firewaller tests become unit tests
	return nil
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity")
	// TODO - implement when remaining firewaller tests become unit tests
	return nil, errors.NotImplementedf("FindEntity")
}

func (st *mockState) GetModel(tag names.ModelTag) (*state.Model, error) {
	st.MethodCall(st, "GetModel", tag)
	// TODO - implement when remaining firewaller tests become unit tests
	return nil, errors.NotImplementedf("GetModel")
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) doneWhenDying() {
	<-w.Tomb.Dying()
	w.Tomb.Done()
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
}

func (w *mockWatcher) Stop() error {
	w.MethodCall(w, "Stop")
	if err := w.NextErr(); err != nil {
		return err
	}
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

func (w *mockWatcher) Err() error {
	w.MethodCall(w, "Err")
	return w.Tomb.Err()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	go w.doneWhenDying()
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockApplication struct {
	testing.Stub
	name  string
	units []*mockUnit
}

func newMockApplication(name string) *mockApplication {
	return &mockApplication{
		name: name,
	}
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.name
}

func (a *mockApplication) AllUnits() (results []firewaller.Unit, err error) {
	a.MethodCall(a, "AllUnits")
	for _, unit := range a.units {
		results = append(results, unit)
	}
	return results, a.NextErr()
}

type mockRelation struct {
	testing.Stub
	id        int
	key       string
	endpoints []state.Endpoint
	ruw       *mockRelationUnitsWatcher
	ruwApp    string
	inScope   set.Strings
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:      id,
		ruw:     newMockRelationUnitsWatcher(),
		inScope: make(set.Strings),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Endpoints() []state.Endpoint {
	r.MethodCall(r, "Endpoints")
	return r.endpoints
}

func (r *mockRelation) WatchUnits(applicationName string) (state.RelationUnitsWatcher, error) {
	if r.ruwApp != applicationName {
		return nil, errors.Errorf("unexpected app %v", applicationName)
	}
	return r.ruw, nil
}

func (r *mockRelation) UnitInScope(u firewaller.Unit) (bool, error) {
	return r.inScope.Contains(u.Name()), nil
}

func (r *mockRelation) WatchRelationIngressNetworks() state.StringsWatcher {
	w := newMockStringsWatcher()
	w.changes <- []string{"1.2.3.4/32"}
	return w
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{changes: make(chan params.RelationUnitsChange, 1)}
	go w.doneWhenDying()
	return w
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan params.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.changes
}

func (st *mockState) GetRemoteEntity(sourceModel names.ModelTag, token string) (names.Tag, error) {
	st.MethodCall(st, "GetRemoteEntity", sourceModel, token)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	for e, t := range st.remoteEntities {
		if t == token {
			return e, nil
		}
	}
	return nil, errors.NotFoundf("token %v", token)
}

func (st *mockState) KeyRelation(key string) (firewaller.Relation, error) {
	st.MethodCall(st, "KeyRelation", key)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	r, ok := st.relations[key]
	if !ok {
		return nil, errors.NotFoundf("relation %q", key)
	}
	return r, nil
}

type mockUnit struct {
	testing.Stub
	mu            sync.Mutex
	name          string
	assigned      bool
	publicAddress network.Address
	machineId     string
}

func newMockUnit(name string) *mockUnit {
	return &mockUnit{
		name:     name,
		assigned: true,
	}
}

func (u *mockUnit) Name() string {
	u.MethodCall(u, "Name")
	return u.name
}

func (u *mockUnit) PublicAddress() (network.Address, error) {
	u.MethodCall(u, "PublicAddress")
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.NextErr(); err != nil {
		return network.Address{}, err
	}
	if !u.assigned {
		return network.Address{}, errors.NotAssignedf(u.name)
	}
	if u.publicAddress.Value == "" {
		return network.Address{}, network.NoAddressError("public")
	}
	return u.publicAddress, nil
}

func (u *mockUnit) AssignedMachineId() (string, error) {
	u.MethodCall(u, "AssignedMachineId")
	if err := u.NextErr(); err != nil {
		return "", err
	}
	if !u.assigned {
		return "", errors.NotAssignedf(u.name)
	}
	return u.machineId, nil
}

func (u *mockUnit) updateAddress(value string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.publicAddress = network.Address{Value: value}
}

type mockMachine struct {
	testing.Stub
	id      string
	watcher *mockAddressWatcher
}

func newMockMachine(id string) *mockMachine {
	return &mockMachine{
		id:      id,
		watcher: newMockAddressWatcher(),
	}
}

func (m *mockMachine) Id() string {
	m.MethodCall(m, "Id")
	return m.id
}

func (m *mockMachine) WatchAddresses() state.NotifyWatcher {
	m.MethodCall(m, "WatchAddresses")
	return m.watcher
}

type mockAddressWatcher struct {
	mockWatcher
	changes chan struct{}
}

func newMockAddressWatcher() *mockAddressWatcher {
	w := &mockAddressWatcher{changes: make(chan struct{}, 1)}
	go w.doneWhenDying()
	return w
}

func (w *mockAddressWatcher) Changes() <-chan struct{} {
	w.MethodCall(w, "Changes")
	return w.changes
}
