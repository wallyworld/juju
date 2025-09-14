// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type PodSpecSuite struct {
	CAASFixture

	Model   *state.CAASModel
	State   *state.State
	Factory *factory.Factory

	application *state.Application
}

func TestPodSpecSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &PodSpecSuite{})
}

func (s *PodSpecSuite) SetUpTest(c *tc.C) {
	s.CAASFixture.SetUpTest(c)
	s.Model, s.State = s.newCAASModel(c)
	s.Factory = factory.NewFactory(s.State, s.StatePool)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

}

func (s *PodSpecSuite) assertPodSpec(c *tc.C, tag names.ApplicationTag, expect string) {
	spec, err := s.Model.PodSpec(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec, tc.Equals, expect)
}

func (s *PodSpecSuite) assertRawK8sSpec(c *tc.C, tag names.ApplicationTag, expect string) {
	rs, err := s.Model.RawK8sSpec(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rs, tc.Equals, expect)
}

func (s *PodSpecSuite) assertPodSpecNotFound(c *tc.C, tag names.ApplicationTag) {
	_, err := s.Model.PodSpec(tag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *PodSpecSuite) applySetRawK8sSpecOperation(token leadership.Token, appTag names.ApplicationTag, spec *string) error {
	modelOp := s.Model.SetRawK8sSpecOperation(token, appTag, spec)
	return s.State.ApplyOperation(modelOp)
}

func (s *PodSpecSuite) assertRawK8sSpecNotFound(c *tc.C, tag names.ApplicationTag) {
	_, err := s.Model.RawK8sSpec(tag)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplication(c *tc.C) {
	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertRawK8sSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationOperator(c *tc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorMatches, "cannot set k8s spec on an operator charm")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationDying(c *tc.C) {
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)

	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertRawK8sSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationDead(c *tc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.EnsureDead(), tc.ErrorIsNil)
	c.Assert(unit.Remove(), tc.ErrorIsNil)
	assertCleanupCount(c, s.State, 1)
	assertLife(c, s.application, state.Dead)

	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorMatches, "setting pod-spec on dead application gitlab not valid")
	s.assertRawK8sSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationUpdates(c *tc.C) {
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), &spec)
		c.Assert(err, tc.ErrorIsNil)
		s.assertRawK8sSpec(c, s.application.ApplicationTag(), spec)
	}
}

func (s *PodSpecSuite) TestRemoveApplicationRemovesRawK8sSpec(c *tc.C) {
	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("spec"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// App removal requires cluster resources to be cleared.
	err = s.application.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, s.State, 2)

	s.assertRawK8sSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetPodSpecApplication(c *tc.C) {
	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertPodSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetPodSpecApplicationOperator(c *tc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorMatches, "cannot set k8s spec on an operator charm")

	// Nil spec allowed.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *PodSpecSuite) TestSetPodSpecApplicationDying(c *tc.C) {
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)

	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorIsNil)
	s.assertPodSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetPodSpecApplicationDead(c *tc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.EnsureDead(), tc.ErrorIsNil)
	c.Assert(unit.Remove(), tc.ErrorIsNil)
	assertCleanupCount(c, s.State, 1)
	assertLife(c, s.application, state.Dead)

	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, tc.ErrorMatches, "setting pod-spec on dead application gitlab not valid")
	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetPodSpecUpdates(c *tc.C) {
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), &spec)
		c.Assert(err, tc.ErrorIsNil)
		s.assertPodSpec(c, s.application.ApplicationTag(), spec)
	}
}

func (s *PodSpecSuite) TestRemoveApplicationRemovesPodSpec(c *tc.C) {
	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// App removal requires cluster resources to be cleared.
	err = s.application.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, s.State, 2)

	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestWatchPodSpec(c *tc.C) {
	w, err := s.Model.WatchPodSpec(s.application.ApplicationTag())
	c.Assert(err, tc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec0"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// No change to spec but still a change because of incremented counter.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec0"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Nil spec also triggers a change because of incremented counter.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Multiple changes coalesced.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec1"))
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec2"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *PodSpecSuite) TestWatchRawK8sSpec(c *tc.C) {
	w, err := s.Model.WatchPodSpec(s.application.ApplicationTag())
	c.Assert(err, tc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 0"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// No change to spec but still a change because of incremented counter.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 0"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Nil spec also triggers a change because of incremented counter.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Multiple changes coalesced.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 1"))
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 2"))
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
