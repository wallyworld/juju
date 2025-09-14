// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"strconv"
	tctesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/internal/testhelpers"
)

type ControllerSuite struct {
	cache.BaseSuite
}

func TestControllerSuite(t *tctesting.T) {
	tc.Run(t, &ControllerSuite{})
}

func (s *ControllerSuite) TestConfigValid(c *tc.C) {
	err := s.Config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ControllerSuite) TestConfigMissingChanges(c *tc.C) {
	s.Config.Changes = nil
	err := s.Config.Validate()
	c.Check(err, tc.ErrorMatches, "nil Changes not valid")
	c.Check(err, tc.Satisfies, errors.IsNotValid)
}

func (s *ControllerSuite) TestController(c *tc.C) {
	controller, err := s.NewController()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(controller.ModelUUIDs(), tc.HasLen, 0)
	c.Check(controller.Report(), tc.HasLen, 0)

	workertest.CleanKill(c, controller)
}

func (s *ControllerSuite) TestAddModel(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, modelChange, events)

	c.Check(controller.ModelUUIDs(), tc.SameContents, []string{modelChange.ModelUUID})
	c.Check(controller.Report(), tc.DeepEquals, map[string]interface{}{
		"model-uuid": map[string]interface{}{
			"name":         "model-owner/test-model",
			"life":         "alive",
			"applications": make(map[string]interface{}),
			"charms":       make(map[string]interface{}),
			"machines":     make(map[string]interface{}),
			"units":        make(map[string]interface{}),
			"relations":    make(map[string]interface{}),
			"branch-count": 0,
		}})

	// The model has the first ID and is registered.
	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertResident(c, mod.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveModel(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, modelChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveModel{ModelUUID: modelChange.ModelUUID}
	s.ProcessChange(c, remove, events)

	c.Check(controller.ModelUUIDs(), tc.HasLen, 0)
	c.Check(controller.Report(), tc.HasLen, 0)
	s.AssertResident(c, mod.CacheId(), false)
}

func (s *ControllerSuite) TestWaitForModelExists(c *tc.C) {
	controller, events := s.New(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	// Process the change event before waiting on the model. This
	// way we know the model exists in the cache before we ask.
	s.ProcessChange(c, modelChange, events)

	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, tc.ErrorIsNil)
		c.Check(model.UUID(), tc.Equals, modelChange.ModelUUID)
	}()

	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Errorf("WaitForModel did not return after %s", testhelpers.LongWait)
	}
}

func (s *ControllerSuite) TestWaitForModelArrives(c *tc.C) {
	//loggo.GetLogger("juju.core.cache").SetLogLevel(loggo.TRACE)
	controller, events := s.New(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, tc.ErrorIsNil)
		c.Check(model.UUID(), tc.Equals, modelChange.ModelUUID)
	}()

	// Don't process the change until we know the model is selecting
	// on the clock.
	c.Assert(clock.WaitAdvance(time.Second, testhelpers.LongWait, 1), tc.ErrorIsNil)
	s.ProcessChange(c, modelChange, events)
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Errorf("WaitForModel did not return after %s", testhelpers.LongWait)
	}
}

func (s *ControllerSuite) TestWaitForModelTimeout(c *tc.C) {
	controller, events := s.New(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, tc.Satisfies, errors.IsTimeout)
		c.Check(model, tc.IsNil)
	}()

	c.Assert(clock.WaitAdvance(10*time.Second, testhelpers.LongWait, 1), tc.ErrorIsNil)

	s.ProcessChange(c, modelChange, events)
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Errorf("WaitForModel did not return after %s", testhelpers.LongWait)
	}
}

func (s *ControllerSuite) TestAddApplication(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, appChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["applications"], tc.HasLen, 1)

	app, err := mod.Application(appChange.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(app, tc.NotNil)

	s.AssertResident(c, app.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveApplication(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, appChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	app, err := mod.Application(appChange.Name)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveApplication{
		ModelUUID: modelChange.ModelUUID,
		Name:      appChange.Name,
	}
	s.ProcessChange(c, remove, events)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["applications"], tc.HasLen, 0)
	s.AssertResident(c, app.CacheId(), false)
}

func (s *ControllerSuite) TestAddCharm(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, charmChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["charms"], tc.HasLen, 1)

	ch, err := mod.Charm(charmChange.CharmURL)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.NotNil)
	s.AssertResident(c, ch.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveCharm(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, charmChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	ch, err := mod.Charm(charmChange.CharmURL)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveCharm{
		ModelUUID: modelChange.ModelUUID,
		CharmURL:  charmChange.CharmURL,
	}
	s.ProcessChange(c, remove, events)

	c.Check(mod.Report()["charms"], tc.HasLen, 0)
	s.AssertResident(c, ch.CacheId(), false)
}

func (s *ControllerSuite) TestAddMachine(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, machineChange, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["machines"], tc.HasLen, 1)

	machine, err := mod.Machine(machineChange.Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machine, tc.NotNil)
	s.AssertResident(c, machine.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveMachine(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, machineChange, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	machine, err := mod.Machine(machineChange.Id)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveMachine{
		ModelUUID: machineChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.ProcessChange(c, remove, events)

	c.Check(mod.Report()["machines"], tc.HasLen, 0)
	s.AssertResident(c, machine.CacheId(), false)
}

func (s *ControllerSuite) TestAddUnit(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, unitChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["units"], tc.HasLen, 1)

	unit, err := mod.Unit(unitChange.Name)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertResident(c, unit.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveUnit(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, unitChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	unit, err := mod.Unit(unitChange.Name)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveUnit{
		ModelUUID: modelChange.ModelUUID,
		Name:      unitChange.Name,
	}
	s.ProcessChange(c, remove, events)

	c.Check(mod.Report()["units"], tc.HasLen, 0)
	s.AssertResident(c, unit.CacheId(), false)
}

func (s *ControllerSuite) TestAddRelation(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, relationChange, events)

	mod, err := controller.Model(relationChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["relations"], tc.HasLen, 1)

	relation, err := mod.Relation(relationChange.Key)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertResident(c, relation.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveRelation(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, relationChange, events)

	mod, err := controller.Model(relationChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	relation, err := mod.Relation(relationChange.Key)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveRelation{
		ModelUUID: modelChange.ModelUUID,
		Key:       relationChange.Key,
	}
	s.ProcessChange(c, remove, events)

	c.Check(mod.Report()["relations"], tc.HasLen, 0)
	s.AssertResident(c, relation.CacheId(), false)
}

func (s *ControllerSuite) TestAddBranch(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, branchChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mod.Report()["branch-count"], tc.Equals, 1)

	branch, err := mod.Branch(branchChange.Name)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertResident(c, branch.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveBranch(c *tc.C) {
	controller, events := s.New(c)
	s.ProcessChange(c, branchChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	branch, err := mod.Branch(branchChange.Name)
	c.Assert(err, tc.ErrorIsNil)

	remove := cache.RemoveBranch{
		ModelUUID: modelChange.ModelUUID,
		Id:        branchChange.Id,
	}
	s.ProcessChange(c, remove, events)

	c.Check(mod.Report()["units"], tc.HasLen, 0)
	s.AssertResident(c, branch.CacheId(), false)
}

func (s *ControllerSuite) TestMarkAndSweep(c *tc.C) {
	controller, events := s.New(c)

	// Note that the model change is processed last.
	s.ProcessChange(c, charmChange, events)
	s.ProcessChange(c, appChange, events)
	s.ProcessChange(c, machineChange, events)
	s.ProcessChange(c, unitChange, events)
	s.ProcessChange(c, modelChange, events)

	controller.Mark()

	done := make(chan struct{})
	go func() {
		// Removals are congruent with LIFO.
		// Model is last because models are added if they do not exist,
		// when we first get a delta for one of their entities.
		c.Check(s.NextChange(c, events), tc.FitsTypeOf, cache.RemoveUnit{})
		c.Check(s.NextChange(c, events), tc.FitsTypeOf, cache.RemoveMachine{})
		c.Check(s.NextChange(c, events), tc.FitsTypeOf, cache.RemoveApplication{})
		c.Check(s.NextChange(c, events), tc.FitsTypeOf, cache.RemoveCharm{})
		c.Check(s.NextChange(c, events), tc.FitsTypeOf, cache.RemoveModel{})
		close(done)
	}()

	controller.Sweep()
	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timeout waiting for sweep removal messages")
	}

	s.AssertNoResidents(c)
}

func (s *ControllerSuite) TestSweepWithConcurrentUpdates(c *tc.C) {
	controller, events := s.New(c)
	done := make(chan struct{})

	s.ProcessChange(c, modelChange, events)

	// As long as the channel is open, keep running mark/sweep.
	// This will generate model summaries repeatedly.
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				controller.Mark()
				controller.Sweep()
			}
		}
	}()

	// Keep pulling processed changes off the events channel.
	// We can't be deterministic about what events will come,
	// because marking and sweeping will evict residents,
	// causing an arbitrary number of removal events.
	// Once we see an application change, we are done.
	go func() {
		for {
			select {
			case change := <-events:
				if _, ok := change.(cache.ApplicationChange); ok {
					return
				}
			}
		}
	}()

	// Add a bunch of machines while we are sweeping.
	// Many will be evicted, but we don't care; we are flexing a data race
	// scenario by causing writes the the model's machines map.
	for i := 0; i < 100; i++ {
		m := machineChange
		m.Id = strconv.Itoa(i)
		s.SendChange(c, m)
	}

	select {
	case done <- struct{}{}:
	case <-time.After(testhelpers.ShortWait):
		c.Fatal("test did not complete mark/sweep goroutine.")
	}

	// We need to ensure all change processing is completed,
	// so send the flagging change to conclude.
	s.SendChange(c, appChange)
}

func (s *ControllerSuite) TestWatchMachineStops(c *tc.C) {
	controller, _ := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)

	w, err := m.WatchMachines()
	c.Assert(err, tc.ErrorIsNil)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, m.Resident, resourceId, true)
	wc.AssertStops()
	s.AssertWorkerResource(c, m.Resident, resourceId, false)
}

func (s *ControllerSuite) TestWatchMachineAddMachine(c *tc.C) {
	w, events := s.setupWithWatchMachine(c)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.ProcessChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineAddContainerNoChange(c *tc.C) {
	w, events := s.setupWithWatchMachine(c)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2/lxd/0",
	}
	s.ProcessChange(c, change, events)
	change2 := change
	change2.Id = "3"
	s.ProcessChange(c, change2, events)
	wc.AssertOneChange([]string{change2.Id})
}

func (s *ControllerSuite) TestWatchMachineRemoveMachine(c *tc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.RemoveMachine{
		ModelUUID: modelChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.ProcessChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineChangeMachine(c *tc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "0",
	}
	s.ProcessChange(c, change, events)
	wc.AssertNoChange()
}

func (s *ControllerSuite) TestWatchMachineGatherMachines(c *tc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.ProcessChange(c, change, events)
	change2 := change
	change2.Id = "3"
	s.ProcessChange(c, change2, events)
	wc.AssertMaybeCombinedChanges([]string{change.Id, change2.Id})
}

func (s *ControllerSuite) newWithMachine(c *tc.C) (*cache.Controller, <-chan interface{}) {
	controller, events := s.New(c)
	s.ProcessChange(c, modelChange, events)
	s.ProcessChange(c, machineChange, events)
	return controller, events
}

func (s *ControllerSuite) setupWithWatchMachine(c *tc.C) (*cache.PredicateStringsWatcher, <-chan interface{}) {
	controller, events := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, tc.ErrorIsNil)

	containerChange := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2/lxd/0",
	}
	s.ProcessChange(c, containerChange, events)

	w, err := m.WatchMachines()
	c.Assert(err, tc.ErrorIsNil)
	return w, events
}
