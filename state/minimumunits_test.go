// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/mgo/v3"
	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

type MinUnitsSuite struct {
	ConnSuite
	application *state.Application
}

func TestMinUnitsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MinUnitsSuite{})
}

func (s *MinUnitsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.application = s.AddTestingApplication(c, "dummy-application", s.AddTestingCharm(c, "dummy"))
}

func (s *MinUnitsSuite) assertRevno(c *tc.C, expectedRevno int, expectedErr error) {
	revno, err := state.MinUnitsRevno(s.State, s.application.Name())
	c.Assert(err, tc.Equals, expectedErr)
	c.Assert(revno, tc.Equals, expectedRevno)
}

func (s *MinUnitsSuite) addUnits(c *tc.C, count int) {
	for i := 0; i < count; i++ {
		_, err := s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *MinUnitsSuite) TestSetMinUnits(c *tc.C) {
	application := s.application
	for i, t := range []struct {
		about   string
		initial int
		changes []int
		revno   int
		err     error
	}{{
		// Revno is set to zero on creation.
		about:   "setting minimum units",
		changes: []int{42},
	}, {
		// Revno is increased by the update operation.
		about:   "updating minimum units",
		initial: 1,
		changes: []int{42},
		revno:   1,
	}, {
		// Revno does not change.
		about:   "updating minimum units with the same value",
		initial: 42,
		changes: []int{42},
	}, {
		// Revno is increased by each update.
		about:   "increasing minimum units multiple times",
		initial: 1,
		changes: []int{2, 3, 4},
		revno:   3,
	}, {
		// Revno does not change.
		about:   "decreasing minimum units multiple times",
		initial: 5,
		changes: []int{3, 2, 1},
	}, {
		// No-op.
		about:   "removing not existent minimum units",
		changes: []int{0},
		err:     mgo.ErrNotFound,
	}, {
		// The document is deleted.
		about:   "removing existing minimum units",
		initial: 42,
		changes: []int{0},
		err:     mgo.ErrNotFound,
	}} {
		c.Logf("test %d. %s", i, t.about)
		// Set up initial minimum units if required.
		if t.initial > 0 {
			err := application.SetMinUnits(t.initial)
			c.Assert(err, tc.ErrorIsNil)
		}
		// Insert/update minimum units.
		for _, input := range t.changes {
			err := application.SetMinUnits(input)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(application.MinUnits(), tc.Equals, input)
			c.Assert(application.Refresh(), tc.IsNil)
			c.Assert(application.MinUnits(), tc.Equals, input)
		}
		// Check the document existence and revno.
		s.assertRevno(c, t.revno, t.err)
		// Clean up, if required, the minUnits document.
		err := application.SetMinUnits(0)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *MinUnitsSuite) TestInvalidMinUnits(c *tc.C) {
	err := s.application.SetMinUnits(-1)
	c.Assert(err, tc.ErrorMatches, `cannot set minimum units for application "dummy-application": cannot set a negative minimum number of units`)
}

func (s *MinUnitsSuite) TestMinUnitsInsertRetry(c *tc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.application.SetMinUnits(41)
		c.Assert(err, tc.ErrorIsNil)
		s.assertRevno(c, 0, nil)
	}, func() {
		s.assertRevno(c, 1, nil)
	}).Check()
	err := s.application.SetMinUnits(42)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.MinUnits(), tc.Equals, 42)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateRetry(c *tc.C) {
	err := s.application.SetMinUnits(41)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.application.SetMinUnits(0)
		c.Assert(err, tc.ErrorIsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}, func() {
		s.assertRevno(c, 0, nil)
	}).Check()
	err = s.application.SetMinUnits(42)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.MinUnits(), tc.Equals, 42)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveBefore(c *tc.C) {
	err := s.application.SetMinUnits(41)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.application.SetMinUnits(0)
		c.Assert(err, tc.ErrorIsNil)
		s.assertRevno(c, 0, mgo.ErrNotFound)
	}).Check()
	err = s.application.SetMinUnits(0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.MinUnits(), tc.Equals, 0)
}

func (s *MinUnitsSuite) testDestroyOrRemoveApplicationBefore(c *tc.C, initial, input int, preventRemoval bool) {
	err := s.application.SetMinUnits(initial)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `cannot set minimum units for application "dummy-application": application "dummy-application" not found`
	if preventRemoval {
		expectedErr = `cannot set minimum units for application "dummy-application": application is no longer alive`
		s.addUnits(c, 1)
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.application.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	err = s.application.SetMinUnits(input)
	c.Assert(err, tc.ErrorMatches, expectedErr)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func (s *MinUnitsSuite) TestMinUnitsInsertDestroyApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 0, 42, true)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateDestroyApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 1, 42, true)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveDestroyApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 1, 0, true)
}

func (s *MinUnitsSuite) TestMinUnitsInsertRemoveApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 0, 42, false)
}

func (s *MinUnitsSuite) TestMinUnitsUpdateRemoveApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 1, 42, false)
}

func (s *MinUnitsSuite) TestMinUnitsRemoveRemoveApplicationBefore(c *tc.C) {
	s.testDestroyOrRemoveApplicationBefore(c, 1, 0, false)
}

func (s *MinUnitsSuite) TestMinUnitsSetDestroyEntities(c *tc.C) {
	err := s.application.SetMinUnits(1)
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 0, nil)

	// Add two units to the application for later use.
	unit1, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	unit2, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Destroy a unit and ensure the revno has been increased.
	preventUnitDestroyRemove(c, unit1)
	err = unit1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 1, nil)

	// Remove a unit and ensure the revno has been increased..
	err = unit2.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 2, nil)

	// Destroy the application and ensure the minUnits document has been removed.
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func (s *MinUnitsSuite) TestMinUnitsNotSetDestroyEntities(c *tc.C) {
	// Add two units to the application for later use.
	unit1, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	unit2, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Destroy a unit and ensure the minUnits document has not been created.
	preventUnitDestroyRemove(c, unit1)
	err = unit1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)

	// Remove a unit and ensure the minUnits document has not been created.
	err = unit2.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)

	// Destroy the application and ensure the minUnits document is still missing.
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	s.assertRevno(c, 0, mgo.ErrNotFound)
}

func assertAllUnits(c *tc.C, application *state.Application, expected int) {
	units, err := application.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, expected)
}

func (s *MinUnitsSuite) TestEnsureMinUnits(c *tc.C) {
	application := s.application
	for i, t := range []struct {
		about    string // Test description.
		initial  int    // Initial number of units.
		minimum  int    // Minimum number of units for the application.
		destroy  int    // Number of units to be destroyed before calling EnsureMinUnits.
		expected int    // Expected number of units after calling EnsureMinUnits.
	}{{
		about: "no minimum units set",
	}, {
		about:    "initial units > minimum units",
		initial:  2,
		minimum:  1,
		expected: 2,
	}, {
		about:    "initial units == minimum units",
		initial:  2,
		minimum:  2,
		expected: 2,
	}, {
		about:    "initial units < minimum units",
		initial:  1,
		minimum:  2,
		expected: 2,
	}, {
		about:    "alive units < minimum units",
		initial:  2,
		minimum:  2,
		destroy:  1,
		expected: 3,
	}, {
		about:    "add multiple units",
		initial:  6,
		minimum:  5,
		destroy:  4,
		expected: 9,
	}} {
		c.Logf("test %d. %s", i, t.about)

		// Set up initial units if required.
		s.addUnits(c, t.initial)

		// Set up minimum units if required.
		err := application.SetMinUnits(t.minimum)
		c.Assert(err, tc.ErrorIsNil)

		// Destroy units if required.
		allUnits, err := application.AllUnits()
		c.Assert(err, tc.ErrorIsNil)
		for i := 0; i < t.destroy; i++ {
			preventUnitDestroyRemove(c, allUnits[i])
			err = allUnits[i].Destroy()
			c.Assert(err, tc.ErrorIsNil)
		}

		// Ensure the minimum number of units is correctly restored.
		c.Assert(application.Refresh(), tc.IsNil)
		err = application.EnsureMinUnits()
		c.Assert(err, tc.ErrorIsNil)
		assertAllUnits(c, application, t.expected)

		// Clean up the minUnits document and the units.
		err = application.SetMinUnits(0)
		c.Assert(err, tc.ErrorIsNil)
		removeAllUnits(c, application)
	}
}

func (s *MinUnitsSuite) TestEnsureMinUnitsApplicationNotAlive(c *tc.C) {
	err := s.application.SetMinUnits(2)
	c.Assert(err, tc.ErrorIsNil)
	s.addUnits(c, 1)
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `cannot ensure minimum units for application "dummy-application": application is not alive`

	// An error is returned if the application is not alive.
	c.Assert(s.application.EnsureMinUnits(), tc.ErrorMatches, expectedErr)

	// An error is returned if the application was removed.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.EnsureMinUnits(), tc.ErrorMatches, expectedErr)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsUpdateMinUnitsRetry(c *tc.C) {
	s.addUnits(c, 1)
	err := s.application.SetMinUnits(4)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.application.SetMinUnits(2)
		c.Assert(err, tc.ErrorIsNil)
	}, func() {
		assertAllUnits(c, s.application, 2)
	}).Check()
	err = s.application.EnsureMinUnits()
	c.Assert(err, tc.ErrorIsNil)

}

func (s *MinUnitsSuite) TestEnsureMinUnitsAddUnitsRetry(c *tc.C) {
	err := s.application.SetMinUnits(3)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetRetryHooks(c, s.State, func() {
		s.addUnits(c, 2)
	}, func() {
		assertAllUnits(c, s.application, 3)
	}).Check()
	err = s.application.EnsureMinUnits()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MinUnitsSuite) testEnsureMinUnitsBefore(c *tc.C, f func(), minUnits, expectedUnits int) {
	application := s.application
	err := application.SetMinUnits(minUnits)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, f).Check()
	err = application.EnsureMinUnits()
	c.Assert(err, tc.ErrorIsNil)
	assertAllUnits(c, application, expectedUnits)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsDecreaseMinUnitsBefore(c *tc.C) {
	f := func() {
		err := s.application.SetMinUnits(3)
		c.Assert(err, tc.ErrorIsNil)
	}
	s.testEnsureMinUnitsBefore(c, f, 42, 3)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsRemoveMinUnitsBefore(c *tc.C) {
	f := func() {
		err := s.application.SetMinUnits(0)
		c.Assert(err, tc.ErrorIsNil)
	}
	s.testEnsureMinUnitsBefore(c, f, 2, 0)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsAddUnitsBefore(c *tc.C) {
	f := func() {
		s.addUnits(c, 2)
	}
	s.testEnsureMinUnitsBefore(c, f, 2, 2)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsDestroyApplicationBefore(c *tc.C) {
	s.addUnits(c, 1)
	err := s.application.SetMinUnits(42)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.application.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	c.Assert(s.application.EnsureMinUnits(), tc.ErrorMatches,
		`cannot ensure minimum units for application "dummy-application": application is not alive`)
}

func (s *MinUnitsSuite) TestEnsureMinUnitsDecreaseMinUnitsAfter(c *tc.C) {
	s.addUnits(c, 2)
	application := s.application
	err := application.SetMinUnits(5)
	c.Assert(err, tc.ErrorIsNil)
	defer state.SetAfterHooks(c, s.State, func() {
		err := application.SetMinUnits(3)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	c.Assert(application.Refresh(), tc.IsNil)
	err = application.EnsureMinUnits()
	c.Assert(err, tc.ErrorIsNil)
	assertAllUnits(c, application, 3)
}
