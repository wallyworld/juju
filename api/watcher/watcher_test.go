// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	tctesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/charm/v12"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/migrationminion"
	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type watcherSuite struct {
	testing.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
}

func TestWatcherSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c, state.JobManageModel, state.JobHostUnits)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *watcherSuite) TestWatchInitialEventConsumed(c *tc.C) {
	// Machiner.Watch should send the initial event as part of the Watch
	// call (for NotifyWatchers there is no state to be transmitted). So a
	// call to Next() should not have anything to return.
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	// We expect the Call() to "Next" to block, so run it in a goroutine.
	done := make(chan error)
	go func() {
		ignored := struct{}{}
		done <- s.stateAPI.APICall("NotifyWatcher", s.stateAPI.BestFacadeVersion("NotifyWatcher"), result.NotifyWatcherId, "Next", nil, &ignored)
	}()

	select {
	case err := <-done:
		c.Errorf("Call(Next) did not block immediately after Watch(): err %v", err)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *watcherSuite) TestWatchMachine(c *tc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()
	wc.AssertOneChange()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *tc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	// params.NotifyWatcher conforms to the watcher.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertStops()
}

func (s *watcherSuite) TestWatchUnitsKeepsEvents(c *tc.C) {
	// Create two applications, relate them, and add one unit to each - a
	// principal and a subordinate.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	principal, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, tc.ErrorIsNil)
	relUnit, err := rel.Unit(principal)
	c.Assert(err, tc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	subordinate, err := s.State.Unit("logging/0")
	c.Assert(err, tc.ErrorIsNil)

	// Call the Deployer facade's WatchUnits for machine-0.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err = s.stateAPI.APICall("Deployer", s.stateAPI.BestFacadeVersion("Deployer"), "", "WatchUnits", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()

	// Now, without reading any changes advance the lifecycle of both
	// units.
	err = subordinate.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = subordinate.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = principal.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// Expect both changes are passed back.
	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()
}

func (s *watcherSuite) TestStringsWatcherStopsWithPendingSend(c *tc.C) {
	// Call the Deployer facade's WatchUnits for machine-0.
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Deployer", s.stateAPI.BestFacadeVersion("Deployer"), "", "WatchUnits", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Create an application, deploy a unit of it on the machine.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	principal, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, tc.ErrorIsNil)
}

// TODO(fwereade): 2015-11-18 lp:1517391
func (s *watcherSuite) TestWatchMachineStorage(c *tc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped",
				Size: 1024,
			},
		}},
	})

	var results params.MachineStorageIdsWatchResults
	args := params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
	}}}
	err := s.stateAPI.APICall(
		"StorageProvisioner",
		s.stateAPI.BestFacadeVersion("StorageProvisioner"),
		"", "WatchVolumeAttachments", args, &results)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.IsNil)

	w := watcher.NewVolumeAttachmentsWatcher(s.stateAPI, result)
	defer func() {

		// Check we can stop the watcher...
		w.Kill()
		wait := make(chan error)
		go func() {
			wait <- w.Wait()
		}()
		select {
		case err := <-wait:
			c.Assert(err, tc.ErrorIsNil)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher never stopped")
		}

		// ...and that its channel hasn't been closed.
		select {
		case change, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
		default:
		}

	}()

	// Check initial event;
	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
		c.Assert(changes, tc.SameContents, []corewatcher.MachineStorageId{{
			MachineTag:    "machine-1",
			AttachmentTag: "volume-0",
		}})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// check no subsequent event.
	select {
	case <-w.Changes():
		c.Fatalf("received unexpected change")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *watcherSuite) assertSetupRelationStatusWatch(
	c *tc.C, rel *state.Relation,
) (func(life life.Value, suspended bool, reason string), func()) {
	// Export the relation so it can be found with a token.
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(rel.Tag())
	c.Assert(err, tc.ErrorIsNil)

	// Create the offer connection details.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred"})
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		OfferUUID:       offer.OfferUUID,
		Username:        "fred",
		RelationKey:     rel.String(),
		RelationId:      rel.Id(),
		SourceModelUUID: s.State.ModelUUID(),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Add the consume permission for the offer so the macaroon
	// discharge can occur.
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(offer.OfferUUID),
		names.NewUserTag("fred"), permission.ConsumeAccess)
	c.Assert(err, tc.ErrorIsNil)

	// Create a macaroon for authorisation.
	store, err := s.State.NewBakeryStorage()
	c.Assert(err, tc.ErrorIsNil)
	b := bakery.New(bakery.BakeryParams{
		RootKeyStore: store,
	})
	c.Assert(err, tc.ErrorIsNil)
	mac, err := b.Oven.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.State.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", rel.String()),
			checkers.DeclaredCaveat("username", "fred"),
		}, bakery.NoOp)
	c.Assert(err, tc.ErrorIsNil)

	// Start watching for a relation change.
	client := crossmodelrelations.NewClient(s.stateAPI)
	w, err := client.WatchRelationSuspendedStatus(params.RemoteEntityArg{
		Token:     token,
		Macaroons: macaroon.Slice{mac.M()},
	})
	c.Assert(err, tc.ErrorIsNil)
	stop := func() {
		workertest.CleanKill(c, w)
	}
	modelUUID := s.BackingState.ModelUUID()
	assertNoChange := func() {
		s.WaitForModelWatchersIdle(c, modelUUID)
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(life life.Value, suspended bool, reason string) {
		s.WaitForModelWatchersIdle(c, modelUUID)
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, tc.IsTrue)
			c.Check(changes, tc.HasLen, 1)
			c.Check(changes[0].Life, tc.Equals, life)
			c.Check(changes[0].Suspended, tc.Equals, suspended)
			c.Check(changes[0].SuspendedReason, tc.Equals, reason)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(life.Alive, false, "")
	return assertChange, stop
}

func (s *watcherSuite) TestRelationStatusWatcher(c *tc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	u, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	m := s.Factory.MakeMachine(c, &factory.MachineParams{})
	err = u.AssignToMachine(m)
	c.Assert(err, tc.ErrorIsNil)
	relUnit, err := rel.Unit(u)
	c.Assert(err, tc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	assertChange, stop := s.assertSetupRelationStatusWatch(c, rel)
	defer stop()

	err = rel.SetSuspended(true, "another reason")
	c.Assert(err, tc.ErrorIsNil)
	assertChange(life.Alive, true, "another reason")

	err = rel.SetSuspended(false, "")
	c.Assert(err, tc.ErrorIsNil)
	assertChange(life.Alive, false, "")

	err = rel.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertChange(life.Dying, false, "")
}

func (s *watcherSuite) TestRelationStatusWatcherDeadRelation(c *tc.C) {
	// Create a pair of services and a relation between them.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	assertChange, stop := s.assertSetupRelationStatusWatch(c, rel)
	defer stop()

	err = rel.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertChange(life.Dead, false, "")
}

func (s *watcherSuite) setupOfferStatusWatch(
	c *tc.C,
) (func(status status.Status, message string), func(), func()) {
	// Create the offer connection details.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred"})
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Add the consume permission for the offer so the macaroon
	// discharge can occur.
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(offer.OfferUUID),
		names.NewUserTag("fred"), permission.ConsumeAccess)
	c.Assert(err, tc.ErrorIsNil)

	// Create a macaroon for authorisation.
	store, err := s.State.NewBakeryStorage()
	c.Assert(err, tc.ErrorIsNil)
	b := bakery.New(bakery.BakeryParams{
		RootKeyStore: store,
	})
	c.Assert(err, tc.ErrorIsNil)
	mac, err := b.Oven.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.State.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", offer.OfferUUID),
			checkers.DeclaredCaveat("username", "fred"),
		}, bakery.NoOp)
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// Start watching for a relation change.
	client := crossmodelrelations.NewClient(s.stateAPI)
	w, err := client.WatchOfferStatus(params.OfferArg{
		OfferUUID: offer.OfferUUID,
		Macaroons: macaroon.Slice{mac.M()},
	})
	c.Assert(err, tc.ErrorIsNil)
	stop := func() {
		workertest.CleanKill(c, w)
	}

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(status status.Status, message string) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, tc.IsTrue)
			if status == "" {
				c.Assert(changes, tc.HasLen, 0)
				break
			}
			c.Assert(changes, tc.HasLen, 1)
			c.Check(changes[0].Name, tc.Equals, "hosted-mysql")
			c.Check(changes[0].Status.Status, tc.Equals, status)
			c.Check(changes[0].Status.Message, tc.Equals, message)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
	}

	// Initial event.
	assertChange(status.Unknown, "")
	return assertChange, assertNoChange, stop
}

func (s *watcherSuite) TestOfferStatusWatcher(c *tc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))

	assertChange, assertNoChange, stop := s.setupOfferStatusWatch(c)
	defer stop()

	err := mysql.SetStatus(status.StatusInfo{Status: status.Unknown, Message: "another message"})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(status.Unknown, "another message")

	// Removing offer and application both trigger events.
	offers := state.NewApplicationOffers(s.State)
	err = offers.Remove("hosted-mysql", false)
	c.Assert(err, tc.ErrorIsNil)
	assertChange("terminated", "offer has been removed")
	err = mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertChange("terminated", "offer has been removed")
	assertNoChange()
}

func ptr[T any](v T) *T {
	return &v
}

func (s *watcherSuite) setupSecretRotationWatcher(
	c *tc.C,
) (*secrets.URI, func(corewatcher.SecretTriggerChange), func(), func()) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "mysql"})
	unit, password := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{
		Application: app,
	})
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	nexRotateTime := time.Now().Add(time.Hour)
	_, err := store.CreateSecret(uri, state.CreateSecretParams{
		Owner: unit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(nexRotateTime),
			Data:           map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	apiInfo := s.APIInfo(c)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiInfo.ModelTag = s.Model.ModelTag()

	apiConn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)

	client := secretsmanager.NewClient(apiConn)
	w, err := client.WatchSecretsRotationChanges(unit.Tag())
	if !c.Check(err, tc.ErrorIsNil) {
		_ = apiConn.Close()
		c.FailNow()
	}
	stop := func() {
		workertest.CleanKill(c, w)
		_ = apiConn.Close()
	}

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(change corewatcher.SecretTriggerChange) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, tc.IsTrue)
			c.Assert(changes, tc.HasLen, 1)
			c.Assert(changes[0], tc.DeepEquals, change)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
	}

	// Initial event.
	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: nexRotateTime.Round(time.Second).UTC(),
	})
	return uri, assertChange, assertNoChange, stop
}

type fakeToken struct{}

func (t *fakeToken) Check() error {
	return nil
}

func (s *watcherSuite) TestSecretsRotationWatcher(c *tc.C) {
	uri, assertChange, assertNoChange, stop := s.setupSecretRotationWatcher(c)
	defer stop()

	store := state.NewSecrets(s.State)

	nexRotateTime := time.Now().Add(24 * time.Hour).Round(time.Second)
	_, err := store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		NextRotateTime: ptr(nexRotateTime),
	})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: nexRotateTime,
	})
	assertNoChange()

	_, err = store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: time.Time{},
	})
	assertNoChange()
}

func (s *watcherSuite) setupSecretExpiryWatcher(
	c *tc.C,
) (*secrets.URI, func(corewatcher.SecretTriggerChange), func(), func()) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "mysql"})
	unit, password := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{
		Application: app,
	})
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	nexRotateTime := time.Now().Add(time.Hour)
	_, err := store.CreateSecret(uri, state.CreateSecretParams{
		Owner: unit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(nexRotateTime),
			Data:           map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	apiInfo := s.APIInfo(c)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiInfo.ModelTag = s.Model.ModelTag()

	apiConn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)

	client := secretsmanager.NewClient(apiConn)
	w, err := client.WatchSecretsRotationChanges(unit.Tag())
	if !c.Check(err, tc.ErrorIsNil) {
		_ = apiConn.Close()
		c.FailNow()
	}
	stop := func() {
		workertest.CleanKill(c, w)
		_ = apiConn.Close()
	}

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(change corewatcher.SecretTriggerChange) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, tc.IsTrue)
			c.Assert(changes, tc.HasLen, 1)
			c.Assert(changes[0], tc.DeepEquals, change)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
	}

	// Initial event.
	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: nexRotateTime.Round(time.Second).UTC(),
	})
	return uri, assertChange, assertNoChange, stop
}

func (s *watcherSuite) TestSecretsExpiryWatcher(c *tc.C) {
	uri, assertChange, assertNoChange, stop := s.setupSecretExpiryWatcher(c)
	defer stop()

	store := state.NewSecrets(s.State)

	nexRotateTime := time.Now().Add(24 * time.Hour).Round(time.Second)
	_, err := store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:    &fakeToken{},
		NextRotateTime: ptr(nexRotateTime),
	})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: nexRotateTime,
	})
	assertNoChange()

	_, err = store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken:  &fakeToken{},
		RotatePolicy: ptr(secrets.RotateNever),
	})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(corewatcher.SecretTriggerChange{
		URI:             uri,
		NextTriggerTime: time.Time{},
	})
	assertNoChange()
}

func (s *watcherSuite) setupSecretsRevisionWatcher(
	c *tc.C,
) (*secrets.URI, func(uri *secrets.URI, rev int), func(), func()) {
	// Set up the offer.
	app := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	unit, password := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{
		Application: app,
	})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred"})
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Add the consume permission for the offer so the macaroon
	// discharge can occur.
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(offer.OfferUUID),
		names.NewUserTag("fred"), permission.ConsumeAccess)
	c.Assert(err, tc.ErrorIsNil)

	// Create a macaroon for authorisation.
	bStore, err := s.State.NewBakeryStorage()
	c.Assert(err, tc.ErrorIsNil)
	b := bakery.New(bakery.BakeryParams{
		RootKeyStore: bStore,
	})
	c.Assert(err, tc.ErrorIsNil)
	mac, err := b.Oven.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.State.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", offer.OfferUUID),
			checkers.DeclaredCaveat("username", "fred"),
		}, bakery.Op{
			Entity: "offer-uuid",
			Action: "consume",
		})
	c.Assert(err, tc.ErrorIsNil)

	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: offer.OfferUUID, URL: "me/model.foo", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	remoteRel, err := s.State.AddRelation(
		state.Endpoint{"mysql", charm.Relation{Name: "server", Interface: "mysql", Role: charm.RoleProvider, Scope: charm.ScopeGlobal}},
		state.Endpoint{"foo", charm.Relation{Name: "db", Interface: "mysql", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal}},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: utils.MustNewUUID().String(),
		OfferUUID:       offer.OfferUUID,
		Username:        "fred",
		RelationId:      remoteRel.Id(),
		RelationKey:     remoteRel.Tag().Id(),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Export the remote entities so they can be found with a token.
	re := s.State.RemoteEntities()
	appToken, err := re.ExportLocalEntity(remoteApp.Tag())
	c.Assert(err, tc.ErrorIsNil)
	relToken, err := re.ExportLocalEntity(remoteRel.Tag())
	c.Assert(err, tc.ErrorIsNil)

	// Create a secret to watch.
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	_, err = store.CreateSecret(uri, state.CreateSecretParams{
		Owner: unit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:  &fakeToken{},
			RotatePolicy: ptr(secrets.RotateDaily),
			Data:         map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	apiInfo := s.APIInfo(c)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiInfo.ModelTag = s.Model.ModelTag()

	apiConn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)

	client := crossmodelrelations.NewClient(apiConn)
	w, err := client.WatchConsumedSecretsChanges(appToken, relToken, mac.M())
	if !c.Check(err, tc.ErrorIsNil) {
		_ = apiConn.Close()
		c.FailNow()
	}
	stop := func() {
		workertest.CleanKill(c, w)
		_ = apiConn.Close()
	}

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(uri *secrets.URI, rev int) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, tc.IsTrue)
			if uri == nil {
				c.Assert(changes, tc.HasLen, 0)
				break
			}
			c.Assert(changes, tc.HasLen, 1)
			c.Assert(changes[0].URI.String(), tc.Equals, uri.String())
			c.Assert(changes[0].Revision, tc.Equals, rev)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
	}
	return uri, assertChange, assertNoChange, stop
}

func (s *watcherSuite) TestCrossModelSecretsRevisionWatcher(c *tc.C) {
	uri, assertChange, assertNoChange, stop := s.setupSecretsRevisionWatcher(c)
	defer stop()

	store := state.NewSecrets(s.State)

	// Initial event - no changes since we're at rev 1 still.
	assertChange(nil, 0)

	err := s.State.SaveSecretRemoteConsumer(uri, names.NewUnitTag("foo/0"), &secrets.SecretConsumerMetadata{
		CurrentRevision: 1,
		LatestRevision:  1,
	})
	c.Assert(err, tc.ErrorIsNil)
	assertNoChange()

	_, err = store.UpdateSecret(uri, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		Data:        secrets.SecretData{"foo": "bar2"},
	})
	c.Assert(err, tc.ErrorIsNil)

	assertChange(uri, 2)
	assertNoChange()
}

type migrationSuite struct {
	testing.JujuConnSuite
}

func TestMigrationSuite(t *tctesting.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestMigrationStatusWatcher(c *tc.C) {
	const nonce = "noncey"

	// Create a model to migrate.
	hostedState := s.Factory.MakeModel(c, &factory.ModelParams{})
	defer hostedState.Close()
	hostedFactory := factory.NewFactory(hostedState, s.StatePool)

	// Create a machine in the hosted model to connect as.
	m, password := hostedFactory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: nonce,
	})

	// Connect as the machine to watch for migration status.
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = m.Tag()
	apiInfo.Password = password

	hostedModel, err := hostedState.Model()
	c.Assert(err, tc.ErrorIsNil)

	apiInfo.ModelTag = hostedModel.ModelTag()
	apiInfo.Nonce = nonce

	apiConn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	defer apiConn.Close()

	// Start watching for a migration.
	client := migrationminion.NewClient(apiConn)
	w, err := client.Watch()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), tc.ErrorIsNil)
	}()

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(id string, phase migration.Phase) {
		select {
		case status, ok := <-w.Changes():
			c.Assert(ok, tc.IsTrue)
			c.Check(status.MigrationId, tc.Equals, id)
			c.Check(status.Phase, tc.Equals, phase)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event with no migration in progress.
	assertChange("", migration.NONE)

	// Now create a migration, should trigger watcher.
	spec := state.MigrationSpec{
		InitiatedBy: names.NewUserTag("someone"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("dog"),
			Password:      "sekret",
		},
	}
	mig, err := hostedState.CreateMigration(spec)
	c.Assert(err, tc.ErrorIsNil)
	assertChange(mig.Id(), migration.QUIESCE)

	// Now abort the migration, this should be reported too.
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	assertChange(mig.Id(), migration.ABORT)
	c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)
	assertChange(mig.Id(), migration.ABORTDONE)

	// Start a new migration, this should also trigger.
	mig2, err := hostedState.CreateMigration(spec)
	c.Assert(err, tc.ErrorIsNil)
	assertChange(mig2.Id(), migration.QUIESCE)
}
