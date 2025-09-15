// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

type PayloadsSuite struct {
	ConnSuite
}

func TestPayloadsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &PayloadsSuite{})
}

func (s *PayloadsSuite) TestLookUp(c *tc.C) {
	fix := s.newFixture(c)

	result, err := fix.UnitPayloads.LookUp("returned", "ignored")
	c.Check(result, tc.Equals, "returned")
	c.Check(err, tc.ErrorIsNil)
}

func (s *PayloadsSuite) TestListPartial(c *tc.C) {
	// Note: List and ListAll are extensively tested via the Check
	// methods on payloadFixture, used throughout the suite. But
	// they don't cover this feature...
	fix, initial := s.newPayloadFixture(c)
	results, err := fix.UnitPayloads.List("whatever", initial.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)

	missing := results[0]
	c.Check(missing.ID, tc.Equals, "whatever")
	c.Check(missing.Payload, tc.IsNil)
	c.Check(missing.NotFound, tc.IsTrue)
	c.Check(missing.Error, tc.Satisfies, errors.IsNotFound)
	c.Check(missing.Error, tc.ErrorMatches, "whatever not found")

	found := results[1]
	c.Check(found.ID, tc.Equals, initial.Name)
	c.Assert(found.Payload, tc.NotNil)
	c.Assert(*found.Payload, tc.DeepEquals, fix.FullPayload(initial))
	c.Check(found.NotFound, tc.IsFalse)
	c.Check(found.Error, tc.ErrorIsNil)
}

func (s *PayloadsSuite) TestNoPayloads(c *tc.C) {
	fix := s.newFixture(c)

	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackInvalidPayload(c *tc.C) {
	// not an exhaustive test, just an indication we do Validate()
	fix := s.newFixture(c)
	pl := fix.SamplePayload("")

	err := fix.UnitPayloads.Track(pl)
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `missing ID not valid`)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackInvalidUnit(c *tc.C) {

	// Note: this is STUPID, but none of the unit-specific contexts
	// between `api/context/register.go` and here ever check that
	// the track request is correctly targeted. So we overwrite it
	// unconditionally... because register is unconditionally
	// sending a garbage unit name for some reason.

	fix := s.newFixture(c)
	expect := fix.SamplePayload("some-docker-id")
	track := expect
	track.Unit = "different/0"

	err := fix.UnitPayloads.Track(track)
	// In a sensible implementation, this would be:
	//
	//    c.Check(err, jc.Satisfies, errors.IsNotValid)
	//    c.Check(err, gc.ErrorMatches, `unexpected Unit "different/0" not valid`)
	//
	//    fix.CheckUnitPayloads(c)
	//    fix.CheckModelPayloads(c)
	//
	// ...but instead we have:
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestTrackInsertPayload(c *tc.C) {
	fix := s.newFixture(c)
	desired := fix.SamplePayload("some-docker-id")

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackUpdatePayload(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)
	replacement := initial
	replacement.ID = "new-exciting-different"

	err := fix.UnitPayloads.Track(replacement)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, replacement)
}

func (s *PayloadsSuite) TestTrackMultiplePayloads(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)
	additional := fix.SamplePayload("another-docker-id")
	additional.Name = "app"

	err := fix.UnitPayloads.Track(additional)
	c.Assert(err, tc.ErrorIsNil)

	full1 := fix.FullPayload(initial)
	full2 := fix.FullPayload(additional)
	fix.CheckUnitPayloads(c, full1, full2)
	fix.CheckModelPayloads(c, full1, full2)
}

func (s *PayloadsSuite) TestTrackMultipleUnits(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	// Create a new unit to add another payload to.
	applicationName := fix.Unit.ApplicationName()
	application, err := s.State.Application(applicationName)
	c.Assert(err, tc.ErrorIsNil)
	machine2 := s.Factory.MakeMachine(c, nil)
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Machine:     machine2,
	})

	// Add a payload which should be independent of the
	// UnitPayloads in the fixture.
	unit2Payloads, err := s.State.UnitPayloads(unit2)
	c.Assert(err, tc.ErrorIsNil)
	additional := initial
	additional.Unit = unit2.Name()
	err = unit2Payloads.Track(additional)
	c.Assert(err, tc.ErrorIsNil)

	// Check the independent payload only shows up in
	// the fixture's ModelPayloads, not its UnitPayloads.
	full1 := fix.FullPayload(initial)
	full2 := payloads.FullPayloadInfo{
		Payload: additional,
		Machine: machine2.Id(),
	}
	fix.CheckUnitPayloads(c, full1)
	fix.CheckModelPayloads(c, full1, full2)
}

func (s *PayloadsSuite) TestSetStatusInvalid(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	err := fix.UnitPayloads.SetStatus(initial.Name, "twirling")
	c.Check(err, tc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), tc.Equals, `status "twirling" not supported; expected one of ["running", "starting", "stopped", "stopping"]`)

	fix.CheckOnePayload(c, initial)
}

func (s *PayloadsSuite) TestSetStatus(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)
	expect := initial
	expect.Status = "stopping"

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
	c.Assert(err, tc.ErrorIsNil)

	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestUntrackMissing(c *tc.C) {
	fix := s.newFixture(c)

	err := fix.UnitPayloads.Untrack("whatever")
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestRemoveUnitUntracksPayloads(c *tc.C) {
	fix, _ := s.newPayloadFixture(c)
	additional := fix.SamplePayload("another-docker-id")
	additional.Name = "app"
	err := fix.UnitPayloads.Track(additional)
	c.Assert(err, tc.ErrorIsNil)

	err = fix.Unit.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceDyingUnit(c *tc.C) {
	fix := s.newFixture(c)
	preventUnitDestroyRemove(c, fix.Unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("this-is-fine")
	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceDeadUnit(c *tc.C) {
	fix := s.newFixture(c)
	preventUnitDestroyRemove(c, fix.Unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, tc.ErrorIsNil)
		err = fix.Unit.EnsureDead()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("sorry-too-late")
	err := fix.UnitPayloads.Track(desired)
	c.Check(err, tc.ErrorMatches, fix.DeadUnitMessage())
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceRemovedUnit(c *tc.C) {
	fix := s.newFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("sorry-too-late")
	err := fix.UnitPayloads.Track(desired)
	c.Check(err, tc.ErrorMatches, fix.DeadUnitMessage())
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceTrack(c *tc.C) {
	fix := s.newFixture(c)
	desired := fix.SamplePayload("wanted")
	interloper := fix.SamplePayload("not-wanted")

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(interloper)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceSetStatus(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)
	desired := initial
	desired.Status = "starting"

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceUntrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(initial)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, initial)
}

func (s *PayloadsSuite) TestSetStatusRaceTrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)
	expect := initial
	expect.Status = "stopped"

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(initial)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopped")
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestSetStatusRaceUntrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopped")
	c.Check(errors.Cause(err), tc.Equals, payloads.ErrNotFound)
	c.Check(err, tc.ErrorMatches, "payload not found")
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceTrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(initial)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceSetStatus(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceUntrack(c *tc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, tc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

// -------------------------
// test helpers

type payloadsFixture struct {
	ModelPayloads state.ModelPayloads
	UnitPayloads  state.UnitPayloads
	Machine       *state.Machine
	Unit          *state.Unit
}

func (s *PayloadsSuite) newFixture(c *tc.C) payloadsFixture {
	machine := s.Factory.MakeMachine(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Machine: machine})
	modelPayloads, err := s.State.ModelPayloads()
	c.Assert(err, tc.ErrorIsNil)
	unitPayloads, err := s.State.UnitPayloads(unit)
	c.Assert(err, tc.ErrorIsNil)
	return payloadsFixture{
		ModelPayloads: modelPayloads,
		UnitPayloads:  unitPayloads,
		Machine:       machine,
		Unit:          unit,
	}
}

func (s *PayloadsSuite) newPayloadFixture(c *tc.C) (payloadsFixture, payloads.Payload) {
	fix := s.newFixture(c)
	initial := fix.SamplePayload("some-docker-id")
	err := fix.UnitPayloads.Track(initial)
	c.Assert(err, tc.ErrorIsNil)
	return fix, initial
}

func (fix payloadsFixture) SamplePayload(id string) payloads.Payload {
	return payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "database",
			Type: "docker",
		},
		Status: payloads.StateRunning,
		ID:     id,
		Unit:   fix.Unit.Name(),
	}
}

func (fix payloadsFixture) DeadUnitMessage() string {
	return fmt.Sprintf("unit %q no longer available", fix.Unit.Name())
}

func (fix payloadsFixture) FullPayload(pl payloads.Payload) payloads.FullPayloadInfo {
	return payloads.FullPayloadInfo{
		Payload: pl,
		Machine: fix.Machine.Id(),
	}
}

func (fix payloadsFixture) CheckNoPayload(c *tc.C) {
	fix.CheckModelPayloads(c)
	fix.CheckUnitPayloads(c)
}

func (fix payloadsFixture) CheckOnePayload(c *tc.C, expect payloads.Payload) {
	full := fix.FullPayload(expect)
	fix.CheckModelPayloads(c, full)
	fix.CheckUnitPayloads(c, full)
}

func (fix payloadsFixture) CheckModelPayloads(c *tc.C, expect ...payloads.FullPayloadInfo) {
	actual, err := fix.ModelPayloads.ListAll()
	c.Check(err, tc.ErrorIsNil)
	sort.Sort(byPayloadInfo(actual))
	sort.Sort(byPayloadInfo(expect))
	c.Check(actual, tc.DeepEquals, expect)
}

func (fix payloadsFixture) CheckUnitPayloads(c *tc.C, expect ...payloads.FullPayloadInfo) {
	actual, err := fix.UnitPayloads.List()
	c.Check(err, tc.ErrorIsNil)
	extracted := fix.extractInfos(c, actual)
	sort.Sort(byPayloadInfo(extracted))
	sort.Sort(byPayloadInfo(expect))
	c.Check(extracted, tc.DeepEquals, expect)
}

func (payloadsFixture) extractInfos(c *tc.C, results []payloads.Result) []payloads.FullPayloadInfo {
	fulls := make([]payloads.FullPayloadInfo, 0, len(results))
	for _, result := range results {
		c.Assert(result.ID, tc.Equals, result.Payload.Name)
		c.Assert(result.Payload, tc.NotNil)
		c.Assert(result.NotFound, tc.IsFalse)
		c.Assert(result.Error, tc.ErrorIsNil)
		fulls = append(fulls, *result.Payload)
	}
	return fulls
}

type byPayloadInfo []payloads.FullPayloadInfo

func (s byPayloadInfo) Len() int      { return len(s) }
func (s byPayloadInfo) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byPayloadInfo) Less(i, j int) bool {
	if s[i].Machine != s[j].Machine {
		return s[i].Machine < s[j].Machine
	}
	if s[i].Payload.Unit != s[j].Payload.Unit {
		return s[i].Payload.Unit < s[j].Payload.Unit
	}
	return s[i].Payload.Name < s[j].Payload.Name
}

// ----------------------------------------------------------
// original functional tests

type PayloadsFunctionalSuite struct {
	ConnSuite
}

func TestPayloadsFunctionalSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &PayloadsFunctionalSuite{})
}

func (s *PayloadsFunctionalSuite) TestModelPayloads(c *tc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:       "dummy",
		application: "a-application",
		metadata:    payloadsMetaYAML,
		machine:     machine,
	})

	ust, err := s.State.UnitPayloads(unit)
	c.Assert(err, tc.ErrorIsNil)

	st, err := s.State.ModelPayloads()
	c.Assert(err, tc.ErrorIsNil)

	payloadInfo, err := st.ListAll()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payloadInfo, tc.HasLen, 0)

	err = ust.Track(payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		Status: payloads.StateRunning,
		ID:     "xyz",
		Unit:   "a-application/0",
	})
	c.Assert(err, tc.ErrorIsNil)

	unitPayloads, err := ust.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitPayloads, tc.HasLen, 1)

	payloadInfo, err = st.ListAll()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payloadInfo, tc.DeepEquals, []payloads.FullPayloadInfo{{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "payloadA",
				Type: "docker",
			},
			ID:     "xyz",
			Status: payloads.StateRunning,
			Labels: []string{},
			Unit:   "a-application/0",
		},
		Machine: machine,
	}})

	id, err := ust.LookUp("payloadA", "xyz")
	c.Assert(err, tc.ErrorIsNil)

	err = ust.Untrack(id)
	c.Assert(err, tc.ErrorIsNil)

	payloadInfo, err = st.ListAll()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(payloadInfo, tc.HasLen, 0)
}

func (s *PayloadsFunctionalSuite) TestUnitPayloads(c *tc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:       "dummy",
		application: "a-application",
		metadata:    payloadsMetaYAML,
		machine:     machine,
	})

	st, err := s.State.UnitPayloads(unit)
	c.Assert(err, tc.ErrorIsNil)

	results, err := st.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)

	pl := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		ID:     "xyz",
		Status: payloads.StateRunning,
		Unit:   "a-application/0",
	}
	err = st.Track(pl)
	c.Assert(err, tc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, tc.ErrorIsNil)
	// TODO(ericsnow) Once Track returns the new ID we can drop
	// the following two lines.
	c.Assert(results, tc.HasLen, 1)
	id := results[0].ID
	c.Check(results, tc.DeepEquals, []payloads.Result{{
		ID: id,
		Payload: &payloads.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	lookedUpID, err := st.LookUp("payloadA", "xyz")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lookedUpID, tc.Equals, id)

	c.Logf("using ID %q", id)
	results, err = st.List(id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []payloads.Result{{
		ID: id,
		Payload: &payloads.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	err = st.SetStatus(id, "running")
	c.Assert(err, tc.ErrorIsNil)

	results, err = st.List(id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []payloads.Result{{
		ID: id,
		Payload: &payloads.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	// Ensure existing ones are replaced.
	update := pl
	update.ID = "abc"
	err = st.Track(update)
	c.Check(err, tc.ErrorIsNil)
	results, err = st.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []payloads.Result{{
		ID: id,
		Payload: &payloads.FullPayloadInfo{
			Payload: update,
			Machine: machine,
		},
	}})

	err = st.Untrack(id)
	c.Assert(err, tc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

const payloadsMetaYAML = `
name: a-charm
summary: a charm...
description: a charm...
payloads:
  payloadA:
    type: docker
`

type unitArgs struct {
	charm       string
	application string
	metadata    string
	machine     string
}

func addUnit(c *tc.C, s ConnSuite, args unitArgs) *state.Unit {
	s.AddTestingCharm(c, args.charm)
	ch := s.AddMetaCharm(c, args.charm, args.metadata, 2)

	app := s.AddTestingApplication(c, args.application, ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	// TODO(ericsnow) Explicitly: call unit.AssignToMachine(m)?
	c.Assert(args.machine, tc.Equals, "0")
	err = unit.AssignToNewMachine() // machine "0"
	c.Assert(err, tc.ErrorIsNil)

	return unit
}
