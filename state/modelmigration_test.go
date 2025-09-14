// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type MigrationSuite struct {
	ConnSuite
	State2  *state.State
	stdSpec state.MigrationSpec
}

func TestMigrationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MigrationSuite{})
}

func (s *MigrationSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	// Create a hosted model to migrate.
	s.State2 = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { s.State2.Close() })

	targetControllerTag := names.NewControllerTag(utils.MustNewUUID().String())

	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)

	// Plausible migration arguments to test with.
	s.stdSpec = state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag:   targetControllerTag,
			ControllerAlias: "target-controller",
			Addrs:           []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:          "cert",
			AuthTag:         names.NewUserTag("user"),
			Password:        "password",
			Macaroons:       []macaroon.Slice{{mac}},
			Token:           "token",
			SkipUserChecks:  true,
		},
	}
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())
}

func (s *MigrationSuite) TestCreate(c *tc.C) {
	model, err := s.State2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeNone)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(mig.ModelUUID(), tc.Equals, s.State2.ModelUUID())
	checkIdAndAttempt(c, mig, 0)

	c.Check(mig.StartTime().IsZero(), tc.IsFalse)
	c.Check(mig.StartTime().Before(s.Clock.Now()), tc.IsTrue)
	c.Check(mig.SuccessTime().IsZero(), tc.IsTrue)
	c.Check(mig.EndTime().IsZero(), tc.IsTrue)
	c.Check(mig.StatusMessage(), tc.Equals, "starting")
	c.Check(mig.InitiatedBy(), tc.Equals, "admin")

	info, err := mig.TargetInfo()
	c.Assert(err, tc.ErrorIsNil)
	// Extract macaroons so we can compare them separately
	// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
	infoMacs := info.Macaroons
	info.Macaroons = nil
	assertMacaroonsEqual(c, infoMacs, s.stdSpec.TargetInfo.Macaroons)
	s.stdSpec.TargetInfo.Macaroons = nil
	c.Check(*info, tc.DeepEquals, s.stdSpec.TargetInfo)
	c.Check(info.ControllerAlias, tc.Equals, s.stdSpec.TargetInfo.ControllerAlias)
	c.Check(info.SkipUserChecks, tc.Equals, s.stdSpec.TargetInfo.SkipUserChecks)

	assertPhase(c, mig, migration.QUIESCE)
	c.Check(mig.PhaseChangedTime(), tc.Equals, mig.StartTime())

	assertMigrationActive(c, s.State2)

	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Check(model.MigrationMode(), tc.Equals, state.MigrationModeExporting)
}

func (s *MigrationSuite) TestIsMigrationActive(c *tc.C) {
	check := func(expected bool) {
		isActive, err := s.State2.IsMigrationActive()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(isActive, tc.Equals, expected)

		isActive2, err := state.IsMigrationActive(s.State, s.State2.ModelUUID())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(isActive2, tc.Equals, expected)
	}

	check(false)

	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	check(true)
}

func (s *MigrationSuite) TestIdSequencesAreIndependent(c *tc.C) {
	st2 := s.State2
	st3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { st3.Close() })

	mig2, err := st2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	checkIdAndAttempt(c, mig2, 0)

	mig3, err := st3.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	checkIdAndAttempt(c, mig3, 0)
}

func (s *MigrationSuite) TestIdSequencesIncrement(c *tc.C) {
	for attempt := 0; attempt < 3; attempt++ {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, tc.ErrorIsNil)
		checkIdAndAttempt(c, mig, attempt)
		c.Check(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
		c.Check(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)
	}
}

func (s *MigrationSuite) TestIdSequencesIncrementOnlyWhenNecessary(c *tc.C) {
	// Ensure that sequence numbers aren't "used up" unnecessarily
	// when the create txn is going to fail.

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 0)

	// This attempt will fail because a migration is already in
	// progress.
	_, err = s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorMatches, ".+already in progress")

	// Now abort the migration and create another. The Id sequence
	// should have only incremented by 1.
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)

	mig, err = s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	checkIdAndAttempt(c, mig, 1)
}

func (s *MigrationSuite) TestSpecValidation(c *tc.C) {
	tests := []struct {
		label        string
		tweakSpec    func(*state.MigrationSpec)
		errorPattern string
	}{{
		"invalid InitiatedBy",
		func(spec *state.MigrationSpec) {
			spec.InitiatedBy = names.UserTag{}
		},
		"InitiatedBy not valid",
	}, {
		"TargetInfo is validated",
		func(spec *state.MigrationSpec) {
			spec.TargetInfo.Addrs = nil
		},
		"empty Addrs not valid",
	}}
	for _, test := range tests {
		c.Logf("---- %s -----------", test.label)

		// Set up spec.
		spec := s.stdSpec
		test.tweakSpec(&spec)

		// Check Validate directly.
		err := spec.Validate()
		c.Check(errors.IsNotValid(err), tc.IsTrue)
		c.Check(err, tc.ErrorMatches, test.errorPattern)

		// Ensure that CreateMigration rejects the bad spec too.
		mig, err := s.State2.CreateMigration(spec)
		c.Check(mig, tc.IsNil)
		c.Check(errors.IsNotValid(err), tc.IsTrue)
		c.Check(err, tc.ErrorMatches, test.errorPattern)
	}
}

func (s *MigrationSuite) TestCreateWithControllerModel(c *tc.C) {
	// This is the State for the controller
	mig, err := s.State.CreateMigration(s.stdSpec)
	c.Check(mig, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "controllers can't be migrated")
}

func (s *MigrationSuite) TestCreateMigrationInProgress(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(mig, tc.Not(tc.IsNil))
	c.Assert(err, tc.ErrorIsNil)

	mig2, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig2, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *MigrationSuite) TestCreateMigrationRace(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(mig, tc.Not(tc.IsNil))
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "failed to create migration: already in progress")
}

func (s *MigrationSuite) TestCreateMigrationWhenModelNotAlive(c *tc.C) {
	// Set the hosted model to Dying.
	model, err := s.State2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Check(mig, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "failed to create migration: model is not alive")
}

func (s *MigrationSuite) TestMigrationToSameController(c *tc.C) {
	spec := s.stdSpec
	spec.TargetInfo.ControllerTag = s.State.ControllerTag()

	mig, err := s.State2.CreateMigration(spec)
	c.Check(mig, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "model already attached to target controller")
}

func (s *MigrationSuite) TestLatestMigration(c *tc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(mig1.Id(), tc.Equals, mig2.Id())
}

func (s *MigrationSuite) TestLatestMigrationNotExist(c *tc.C) {
	mig, err := s.State.LatestMigration()
	c.Check(mig, tc.IsNil)
	c.Check(errors.IsNotFound(err), tc.IsTrue)
}

func (s *MigrationSuite) TestGetsLatestAttempt(c *tc.C) {
	modelUUID := s.State2.ModelUUID()

	for i := 0; i < 10; i++ {
		c.Logf("loop %d", i)
		_, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, tc.ErrorIsNil)

		mig, err := s.State2.LatestMigration()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(mig.Id(), tc.Equals, fmt.Sprintf("%s:%d", modelUUID, i))

		c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)
	}
}

func (s *MigrationSuite) TestLatestMigrationWithPrevious(c *tc.C) {
	// Check the scenario of a model having been migrated away and
	// then migrated back several times. The previous migrations
	// shouldn't be reported by LatestMigration.

	// Make it appear as if the model has been successfully
	// migrated. Don't actually remove model documents to simulate it
	// having been migrated back to the controller.
	phases := []migration.Phase{
		migration.IMPORT,
		migration.PROCESSRELATIONS,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.DONE,
		// Check that it is idempotent on DONE.
		migration.DONE,
	}
	for i := 0; i < 10; i++ {
		mig, err := s.State2.CreateMigration(s.stdSpec)
		c.Assert(err, tc.ErrorIsNil)
		for _, phase := range phases {
			c.Assert(mig.SetPhase(phase), tc.ErrorIsNil)
		}
		state.ResetMigrationMode(c, s.State2)
	}

	// Previous migration shouldn't be reported.
	_, err := s.State2.LatestMigration()
	c.Check(errors.IsNotFound(err), tc.IsTrue)
	c.Check(err, tc.ErrorMatches, "migration not found")

	// Start a new migration attempt, which should be reported.
	migNext, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	migNextb, err := s.State2.LatestMigration()
	c.Check(err, tc.ErrorIsNil)
	c.Check(migNextb.Id(), tc.Equals, migNext.Id())
	phase, err := migNextb.Phase()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phase, tc.Equals, migration.QUIESCE)
}

func (s *MigrationSuite) TestLatestRemovedModelMigration(c *tc.C) {
	model, err := s.State2.Model()
	c.Assert(err, tc.ErrorIsNil)

	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig1.SetPhase(phase), tc.ErrorIsNil)
	}

	// CompletedMigration should fail as the model docs are still there
	_, err = s.State2.CompletedMigration()
	c.Assert(errors.IsNotFound(err), tc.Equals, true)

	// Delete the model and check that we get back the MigrationModel
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(s.State2.RemoveDyingModel(), tc.ErrorIsNil)

	mig2, err := s.State2.CompletedMigration()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig2, tc.DeepEquals, mig1)

	// Check that LatestMigration works with the model removed
	mig3, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig3, tc.DeepEquals, mig1)
}

func (s *MigrationSuite) TestMigration(c *tc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	mig2, err := s.State2.Migration(mig1.Id())
	c.Check(err, tc.ErrorIsNil)
	c.Check(mig1.Id(), tc.Equals, mig2.Id())
	c.Check(mig2.StartTime().IsZero(), tc.IsFalse)
	c.Check(mig2.StartTime().Before(s.Clock.Now()), tc.IsTrue)
}

func (s *MigrationSuite) TestMigrationNotFound(c *tc.C) {
	_, err := s.State2.Migration("does not exist")
	c.Check(err, tc.Satisfies, errors.IsNotFound)
	c.Check(err, tc.ErrorMatches, "migration not found")
}

func (s *MigrationSuite) TestRefresh(c *tc.C) {
	mig1, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	err = mig1.SetPhase(migration.IMPORT)
	c.Assert(err, tc.ErrorIsNil)

	assertPhase(c, mig2, migration.QUIESCE)
	err = mig2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	assertPhase(c, mig2, migration.IMPORT)
}

func (s *MigrationSuite) TestSuccessfulPhaseTransitions(c *tc.C) {
	st := s.State2

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig, tc.NotNil)

	mig2, err := st.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	phases := migration.SuccessfulMigrationPhases()

	var successTime time.Time
	for _, phase := range phases[:len(phases)-1] {
		err := mig.SetPhase(phase)
		c.Assert(err, tc.ErrorIsNil)

		assertPhase(c, mig, phase)
		c.Check(mig.PhaseChangedTime().IsZero(), tc.IsFalse)
		c.Assert(mig.PhaseChangedTime().Before(s.Clock.Now()), tc.IsTrue)

		// Check success timestamp is set only when SUCCESS is
		// reached.
		if phase < migration.SUCCESS {
			c.Assert(mig.SuccessTime().IsZero(), tc.IsTrue)
		} else {
			if phase == migration.SUCCESS {
				successTime = s.Clock.Now()
			}
			if successTime.IsZero() {
				c.Assert(mig.SuccessTime().IsZero(), tc.IsTrue)
			} else {
				c.Assert(mig.SuccessTime().IsZero(), tc.IsFalse)
				c.Assert(mig.SuccessTime().Before(successTime), tc.IsTrue)
			}
		}

		// Check still marked as active.
		assertMigrationActive(c, s.State2)
		c.Assert(mig.EndTime().IsZero(), tc.IsTrue)

		// Ensure change was peristed.
		c.Assert(mig2.Refresh(), tc.ErrorIsNil)
		assertPhase(c, mig2, phase)

		s.Clock.Advance(time.Millisecond)
	}

	// Now move to the final phase (DONE) and ensure fields are set as
	// expected.
	err = mig.SetPhase(migration.DONE)
	c.Assert(err, tc.ErrorIsNil)
	assertPhase(c, mig, migration.DONE)
	s.assertMigrationCleanedUp(c, mig)
}

func (s *MigrationSuite) TestABORTCleanup(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	s.Clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	s.Clock.Advance(time.Millisecond)
	c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)

	s.assertMigrationCleanedUp(c, mig)

	// Model should be set back to active.
	model, err := s.State2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeNone)
}

func (s *MigrationSuite) TestREAPFAILEDCleanup(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	// Advance the migration to REAPFAILED.
	phases := []migration.Phase{
		migration.IMPORT,
		migration.PROCESSRELATIONS,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.REAPFAILED,
	}
	for _, phase := range phases {
		s.Clock.Advance(time.Millisecond)
		c.Assert(mig.SetPhase(phase), tc.ErrorIsNil)
	}

	s.assertMigrationCleanedUp(c, mig)
}

func (s *MigrationSuite) assertMigrationCleanedUp(c *tc.C, mig state.ModelMigration) {
	c.Check(mig.PhaseChangedTime().IsZero(), tc.IsFalse)
	c.Assert(mig.PhaseChangedTime().Before(s.Clock.Now()), tc.IsTrue)
	c.Check(mig.EndTime().IsZero(), tc.IsFalse)
	c.Assert(mig.EndTime().Before(s.Clock.Now()), tc.IsTrue)
	assertMigrationNotActive(c, s.State2)
}

func (s *MigrationSuite) TestIllegalPhaseTransition(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	err = mig.SetPhase(migration.SUCCESS)
	c.Check(err, tc.ErrorMatches, "illegal phase change: QUIESCE -> SUCCESS")
}

func (s *MigrationSuite) TestPhaseChangeRace(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig, tc.Not(tc.IsNil))

	defer state.SetBeforeHooks(c, s.State2, func() {
		mig, err := s.State2.LatestMigration()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mig.SetPhase(migration.IMPORT), tc.ErrorIsNil)
	}).Check()

	err = mig.SetPhase(migration.IMPORT)
	c.Assert(err, tc.ErrorMatches, "phase already changed")
	assertPhase(c, mig, migration.QUIESCE)

	// After a refresh it the phase change should be ok.
	c.Assert(mig.Refresh(), tc.ErrorIsNil)
	err = mig.SetPhase(migration.IMPORT)
	c.Assert(err, tc.ErrorIsNil)
	assertPhase(c, mig, migration.IMPORT)
}

func (s *MigrationSuite) TestStatusMessage(c *tc.C) {
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig, tc.Not(tc.IsNil))

	mig2, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(mig.StatusMessage(), tc.Equals, "starting")
	c.Check(mig2.StatusMessage(), tc.Equals, "starting")

	err = mig.SetStatusMessage("foo bar")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(mig.StatusMessage(), tc.Equals, "foo bar")

	c.Assert(mig2.Refresh(), tc.ErrorIsNil)
	c.Check(mig2.StatusMessage(), tc.Equals, "foo bar")
}

func (s *MigrationSuite) TestWatchForMigration(c *tc.C) {
	// Start watching for migration.
	w, wc := s.createMigrationWatcher(c, s.State2)
	wc.AssertOneChange()

	// Create the migration - should be reported.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Mere phase changes should not be reported.
	c.Check(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	wc.AssertNoChange()

	// Ending the migration should be reported.
	c.Check(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MigrationSuite) TestWatchForMigrationInProgress(c *tc.C) {
	// Create a migration.
	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())

	// Start watching for a migration - the in progress one should be reported.
	_, wc := s.createMigrationWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *MigrationSuite) TestWatchForMigrationMultiModel(c *tc.C) {
	_, wc2 := s.createMigrationWatcher(c, s.State2)
	wc2.AssertOneChange()

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { State3.Close() })
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, State3.ModelUUID())
	_, wc3 := s.createMigrationWatcher(c, State3)
	wc3.AssertOneChange()

	// Create a migration for 2.
	_, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()
}

func (s *MigrationSuite) createMigrationWatcher(c *tc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	w := st.WatchForMigration()
	s.AddCleanup(func(c *tc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, w)
}

func (s *MigrationSuite) TestWatchMigrationStatus(c *tc.C) {
	w, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertOneChange() // Initial event.

	// Create a migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	wc.AssertOneChange()
	c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)
	wc.AssertOneChange()

	// Start another.
	mig2, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Change phase.
	c.Assert(mig2.SetPhase(migration.IMPORT), tc.ErrorIsNil)
	wc.AssertOneChange()

	// End it.
	c.Assert(mig2.SetPhase(migration.ABORT), tc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MigrationSuite) TestWatchMigrationStatusPreexisting(c *tc.C) {
	// Create an aborted migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.State2.ModelUUID())

	_, wc := s.createStatusWatcher(c, s.State2)
	wc.AssertOneChange()
}

func (s *MigrationSuite) TestWatchMigrationStatusMultiModel(c *tc.C) {
	_, wc2 := s.createStatusWatcher(c, s.State2)
	wc2.AssertOneChange() // initial event

	// Create another hosted model to migrate and watch for
	// migrations.
	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { State3.Close() })
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, State3.ModelUUID())

	_, wc3 := s.createStatusWatcher(c, State3)
	wc3.AssertOneChange() // initial event

	// Create a migration for 2.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()

	// Create a migration for 3.
	_, err = State3.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	wc2.AssertNoChange()
	wc3.AssertOneChange()

	// Update the migration for 2.
	err = mig.SetPhase(migration.ABORT)
	c.Assert(err, tc.ErrorIsNil)
	wc2.AssertOneChange()
	wc3.AssertNoChange()
}

func (s *MigrationSuite) TestMinionReports(c *tc.C) {
	// Create some machines and units to report with.
	factory2 := factory.NewFactory(s.State2, s.StatePool)
	m0 := factory2.MakeMachine(c, nil)
	u0 := factory2.MakeUnit(c, &factory.UnitParams{Machine: m0})
	m1 := factory2.MakeMachine(c, nil)
	m2 := factory2.MakeMachine(c, nil)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(m0.Tag(), phase, true), tc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(m1.Tag(), phase, false), tc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(u0.Tag(), phase, true), tc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []names.Tag{m0.Tag(), u0.Tag()})
	c.Check(reports.Failed, tc.SameContents, []names.Tag{m1.Tag()})
	c.Check(reports.Unknown, tc.SameContents, []names.Tag{m2.Tag()})
}

func (s *MigrationSuite) TestMinionReportsCAASLegacy(c *tc.C) {
	// Create some machines and units to report with.
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	factory2 := factory.NewFactory(st, s.StatePool)
	ch := factory2.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	a0 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a0", Charm: ch})
	a1 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a1", Charm: ch})
	a2 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a2", Charm: ch})

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(a0.Tag(), phase, true), tc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(a1.Tag(), phase, false), tc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []names.Tag{a0.Tag()})
	c.Check(reports.Failed, tc.SameContents, []names.Tag{a1.Tag()})
	c.Check(reports.Unknown, tc.SameContents, []names.Tag{a2.Tag()})
}

func (s *MigrationSuite) TestMinionReportsCAASEmbedded(c *tc.C) {
	// Create some machines and units to report with.
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	factory2 := factory.NewFactory(st, s.StatePool)
	ch := factory2.MakeCharmV2(c, &factory.CharmParams{
		Name:   "snappass-test",
		Series: "quantal",
	})
	a0 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a0", Charm: ch})
	u1a0, err := a0.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id0")})
	c.Assert(err, tc.ErrorIsNil)
	a1 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a1", Charm: ch})
	u1a1, err := a1.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id1")})
	c.Assert(err, tc.ErrorIsNil)
	a2 := factory2.MakeApplication(c, &factory.ApplicationParams{Name: "a2", Charm: ch})
	u1a2, err := a2.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id2")})
	c.Assert(err, tc.ErrorIsNil)

	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	const phase = migration.QUIESCE
	c.Assert(mig.SubmitMinionReport(u1a0.Tag(), phase, true), tc.ErrorIsNil)
	c.Assert(mig.SubmitMinionReport(u1a1.Tag(), phase, false), tc.ErrorIsNil)

	reports, err := mig.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []names.Tag{u1a0.Tag()})
	c.Check(reports.Failed, tc.SameContents, []names.Tag{u1a1.Tag()})
	c.Check(reports.Unknown, tc.SameContents, []names.Tag{u1a2.Tag()})
}

func (s *MigrationSuite) TestDuplicateMinionReportsSameSuccess(c *tc.C) {
	// It should be OK for a minion report to arrive more than once
	// for the same migration, agent and phase as long as the value of
	// "success" is the same.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewMachineTag("42")
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), tc.ErrorIsNil)
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), tc.ErrorIsNil)
}

func (s *MigrationSuite) TestDuplicateMinionReportsDifferingSuccess(c *tc.C) {
	// It is not OK for a minion report to arrive more than once for
	// the same migration, agent and phase when the "success" value
	// changes.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	tag := names.NewMachineTag("42")
	c.Check(mig.SubmitMinionReport(tag, migration.QUIESCE, true), tc.ErrorIsNil)
	err = mig.SubmitMinionReport(tag, migration.QUIESCE, false)
	c.Check(err, tc.ErrorMatches,
		fmt.Sprintf("conflicting reports received for %s/QUIESCE/machine-42", mig.Id()))
}

func (s *MigrationSuite) TestMinionReportWithOldPhase(c *tc.C) {
	// It is OK for a report to arrive for even a migration has moved
	// on.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	// Get another reference to the same migration.
	migalt, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	// Confirm that there's no reports when starting.
	reports, err := mig.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.HasLen, 0)

	// Advance the migration
	c.Assert(mig.SetPhase(migration.IMPORT), tc.ErrorIsNil)

	// Submit minion report for the old phase.
	tag := names.NewMachineTag("42")
	c.Assert(mig.SubmitMinionReport(tag, migration.QUIESCE, true), tc.ErrorIsNil)

	// The report should still have been recorded.
	reports, err = migalt.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []names.Tag{tag})
}

func (s *MigrationSuite) TestMinionReportWithInactiveMigration(c *tc.C) {
	// Create a migration.
	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	// Get another reference to the same migration.
	migalt, err := s.State2.LatestMigration()
	c.Assert(err, tc.ErrorIsNil)

	// Abort the migration.
	c.Assert(mig.SetPhase(migration.ABORT), tc.ErrorIsNil)
	c.Assert(mig.SetPhase(migration.ABORTDONE), tc.ErrorIsNil)

	// Confirm that there's no reports when starting.
	reports, err := mig.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.HasLen, 0)

	// Submit a minion report for it.
	tag := names.NewMachineTag("42")
	c.Assert(mig.SubmitMinionReport(tag, migration.QUIESCE, true), tc.ErrorIsNil)

	// The report should still have been recorded.
	reports, err = migalt.MinionReports()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []names.Tag{tag})
}

func (s *MigrationSuite) TestWatchMinionReports(c *tc.C) {
	mig, wc := s.createMigAndWatchReports(c, s.State2)
	wc.AssertOneChange() // initial event

	// A report should trigger the watcher.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), tc.ErrorIsNil)
	wc.AssertOneChange()

	// A report for a different phase shouldn't trigger the watcher.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("1"), migration.IMPORT, true), tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *MigrationSuite) TestWatchMinionReportsMultiModel(c *tc.C) {
	mig, wc := s.createMigAndWatchReports(c, s.State2)
	wc.AssertOneChange() // initial event

	State3 := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*tc.C) { State3.Close() })
	mig3, wc3 := s.createMigAndWatchReports(c, State3)
	wc3.AssertOneChange() // initial event

	// Ensure the correct watchers are triggered.
	c.Assert(mig.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), tc.ErrorIsNil)
	wc.AssertOneChange()
	wc3.AssertNoChange()

	c.Assert(mig3.SubmitMinionReport(names.NewMachineTag("0"), migration.QUIESCE, true), tc.ErrorIsNil)
	wc.AssertNoChange()
	wc3.AssertOneChange()
}

func (s *MigrationSuite) TestModelUserAccess(c *tc.C) {
	model, err := s.State2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.MigrationMode(), tc.Equals, state.MigrationModeNone)

	// Get users that had access to the model before the migration
	modelUsers, err := model.Users()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(modelUsers), tc.Not(tc.Equals), 0)

	mig, err := s.State2.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)

	for _, modelUser := range modelUsers {
		c.Logf("check that migration doc lists user %q having permission %q", modelUser.UserTag, modelUser.Access)
		perm := mig.ModelUserAccess(modelUser.UserTag)
		c.Assert(perm, tc.Equals, modelUser.Access)
	}

	// Querying for any other user should yield permission.NoAccess
	perm := mig.ModelUserAccess(names.NewUserTag("bogus"))
	c.Assert(perm, tc.Equals, permission.NoAccess)
}

func (s *MigrationSuite) createStatusWatcher(c *tc.C, st *state.State) (
	state.NotifyWatcher, statetesting.NotifyWatcherC,
) {
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	w := st.WatchMigrationStatus()
	s.AddCleanup(func(c *tc.C) { statetesting.AssertStop(c, w) })
	return w, statetesting.NewNotifyWatcherC(c, w)
}

func (s *MigrationSuite) createMigAndWatchReports(c *tc.C, st *state.State) (
	state.ModelMigration, statetesting.NotifyWatcherC,
) {
	mig, err := st.CreateMigration(s.stdSpec)
	c.Assert(err, tc.ErrorIsNil)
	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, st.ModelUUID())

	w, err := mig.WatchMinionReports()
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { statetesting.AssertStop(c, w) })
	wc := statetesting.NewNotifyWatcherC(c, w)

	return mig, wc
}

func assertPhase(c *tc.C, mig state.ModelMigration, phase migration.Phase) {
	actualPhase, err := mig.Phase()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(actualPhase, tc.Equals, phase)
}

func assertMigrationActive(c *tc.C, st *state.State) {
	c.Check(isMigrationActive(c, st), tc.IsTrue)
}

func assertMigrationNotActive(c *tc.C, st *state.State) {
	c.Check(isMigrationActive(c, st), tc.IsFalse)
}

func isMigrationActive(c *tc.C, st *state.State) bool {
	isActive, err := st.IsMigrationActive()
	c.Assert(err, tc.ErrorIsNil)
	return isActive
}

func checkIdAndAttempt(c *tc.C, mig state.ModelMigration, expected int) {
	c.Check(mig.Id(), tc.Equals, fmt.Sprintf("%s:%d", mig.ModelUUID(), expected))
	c.Check(mig.Attempt(), tc.Equals, expected)
}
