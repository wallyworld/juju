// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	tctesting "testing"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"

	"github.com/juju/juju/state"
)

func TestSequenceSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &sequenceSuite{})
}

type sequenceSuite struct {
	ConnSuite
}

func (s *sequenceSuite) TestSequence(c *tc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	s.checkDocCount(c, 1)
	s.checkDoc(c, s.State.ModelUUID(), "foo", 1)

	s.incAndCheck(c, s.State, "foo", 1)
	s.checkDocCount(c, 1)
	s.checkDoc(c, s.State.ModelUUID(), "foo", 2)
}

func (s *sequenceSuite) TestMultipleSequences(c *tc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	s.incAndCheck(c, s.State, "bar", 0)
	s.incAndCheck(c, s.State, "bar", 1)
	s.incAndCheck(c, s.State, "foo", 1)
	s.incAndCheck(c, s.State, "bar", 2)

	s.checkDocCount(c, 2)
	s.checkDoc(c, s.State.ModelUUID(), "foo", 2)
	s.checkDoc(c, s.State.ModelUUID(), "bar", 3)
}

func (s *sequenceSuite) TestSequenceWithMultipleModels(c *tc.C) {
	state1 := s.State
	state2 := s.Factory.MakeModel(c, nil)
	defer state2.Close()

	s.incAndCheck(c, state1, "foo", 0)
	s.incAndCheck(c, state2, "foo", 0)
	s.incAndCheck(c, state1, "foo", 1)
	s.incAndCheck(c, state2, "foo", 1)
	s.incAndCheck(c, state1, "foo", 2)

	s.checkDocCount(c, 2)
	s.checkDoc(c, state1.ModelUUID(), "foo", 3)
	s.checkDoc(c, state2.ModelUUID(), "foo", 2)
}

func (s *sequenceSuite) TestSequences(c *tc.C) {
	state1 := s.State
	state2 := s.Factory.MakeModel(c, nil)
	defer state2.Close()

	s.incAndCheck(c, state1, "foo", 0)
	s.incAndCheck(c, state2, "foo", 0)
	s.incAndCheck(c, state1, "foo", 1)
	s.incAndCheck(c, state2, "foo", 1)
	s.incAndCheck(c, state1, "foo", 2)
	s.incAndCheck(c, state1, "bar", 0)
	s.incAndCheck(c, state2, "baz", 0)

	sequences, err := state1.Sequences()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sequences, tc.DeepEquals, map[string]int{
		"foo": 3, "bar": 1,
	})
}

func (s *sequenceSuite) TestSequenceWithMin(c *tc.C) {
	st := s.State
	modelUUID := st.ModelUUID()
	const name = "foo"

	value, err := state.SequenceWithMin(st, name, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 3)
	s.checkDoc(c, modelUUID, name, 4)

	value, err = state.SequenceWithMin(st, name, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 4)
	s.checkDoc(c, modelUUID, name, 5)

	value, err = state.SequenceWithMin(st, name, 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 5)
	s.checkDoc(c, modelUUID, name, 6)

	value, err = state.SequenceWithMin(st, name, 10)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 10)
	s.checkDoc(c, modelUUID, name, 11)
}

func (s *sequenceSuite) TestMultipleSequenceWithMin(c *tc.C) {
	st := s.State

	value, err := state.SequenceWithMin(st, "foo", 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 3)

	value, err = state.SequenceWithMin(st, "bar", 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 2)

	value, err = state.SequenceWithMin(st, "foo", 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 4)

	value, err = state.SequenceWithMin(st, "bar", 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 3)
}

func (s *sequenceSuite) TestMigrationSequenceWithMin(c *tc.C) {
	// Test local charm migration where incoming charms may have
	// the same curl with non-sequential revisions.
	st := s.State

	value, err := state.SequenceWithMin(st, "foo", 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 0)

	value, err = state.SequenceWithMin(st, "foo", 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, 2)

	found, err := state.Sequence(st, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.Equals, 3)
}

func (s *sequenceSuite) TestContention(c *tc.C) {
	const name = "foo"
	const goroutines = 2
	const iterations = 10
	st := s.State

	type results struct {
		values  []int
		numErrs int
	}
	resultsCh := make(chan results)

	workFunc := func(nextSeq func() (int, error)) {
		var r results
		for i := 0; i < iterations; i++ {
			v, err := nextSeq()
			if err != nil {
				c.Logf("sequence increment failed: %v", err)
				r.numErrs++
			}
			r.values = append(r.values, v)
		}
		resultsCh <- r
	}

	go workFunc(func() (int, error) {
		return state.Sequence(st, name)
	})

	go workFunc(func() (int, error) {
		return state.SequenceWithMin(st, name, 0)
	})

	var seenValues sort.IntSlice
	var seenErrs int
	for i := 0; i < goroutines; i++ {
		r := <-resultsCh
		seenValues = append(seenValues, r.values...)
		seenErrs += r.numErrs
	}
	c.Assert(seenErrs, tc.Equals, 0)

	numExpected := goroutines * iterations
	c.Assert(len(seenValues), tc.Equals, numExpected)
	seenValues.Sort()
	for i := 0; i < numExpected; i++ {
		c.Assert(seenValues[i], tc.Equals, i, tc.Commentf("index %d", i))
	}
}

func (s *sequenceSuite) TestEnsureCreate(c *tc.C) {
	err := state.SequenceEnsure(s.State, "foo", 3)
	c.Assert(err, tc.ErrorIsNil)
	s.incAndCheck(c, s.State, "foo", 3)
}

func (s *sequenceSuite) TestEnsureSet(c *tc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	err := state.SequenceEnsure(s.State, "foo", 5)
	c.Assert(err, tc.ErrorIsNil)
	s.incAndCheck(c, s.State, "foo", 5)
}

func (s *sequenceSuite) TestEnsureBackwards(c *tc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	s.incAndCheck(c, s.State, "foo", 1)
	s.incAndCheck(c, s.State, "foo", 2)

	err := state.SequenceEnsure(s.State, "foo", 1)
	c.Assert(err, tc.ErrorIsNil)

	s.incAndCheck(c, s.State, "foo", 3)
}

func (s *sequenceSuite) TestResetSequence(c *tc.C) {
	err := state.SequenceEnsure(s.State, "foo", 3)
	c.Assert(err, tc.ErrorIsNil)
	s.incAndCheck(c, s.State, "foo", 3)

	err = state.ResetSequence(s.State, "foo")
	c.Assert(err, tc.ErrorIsNil)

	s.incAndCheck(c, s.State, "foo", 0)
}
func (s *sequenceSuite) incAndCheck(c *tc.C, st *state.State, name string, expectedCount int) {
	value, err := state.Sequence(st, name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.Equals, expectedCount)
}

func (s *sequenceSuite) checkDocCount(c *tc.C, expectedCount int) {
	coll, closer := state.GetRawCollection(s.State, "sequence")
	defer closer()
	count, err := coll.Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expectedCount)
}

func (s *sequenceSuite) checkDoc(c *tc.C, modelUUID, name string, value int) {
	coll, closer := state.GetRawCollection(s.State, "sequence")
	defer closer()

	docID := modelUUID + ":" + name
	var doc bson.M
	err := coll.FindId(docID).One(&doc)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(doc["_id"], tc.Equals, docID)
	c.Check(doc["name"], tc.Equals, name)
	c.Check(doc["model-uuid"], tc.Equals, modelUUID)
	c.Check(doc["counter"], tc.Equals, value)
}
