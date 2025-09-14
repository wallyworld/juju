// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type ApplicationLeaderSuite struct {
	ConnSuite
	application *state.Application
}

func TestApplicationLeaderSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ApplicationLeaderSuite{})
}

func (s *ApplicationLeaderSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.application = s.Factory.MakeApplication(c, nil)
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ApplicationLeaderSuite) TestReadEmpty(c *tc.C) {
	s.checkSettings(c, map[string]string{})
}

func (s *ApplicationLeaderSuite) TestWrite(c *tc.C) {
	s.writeSettings(c, map[string]string{
		"foo":     "bar",
		"baz.qux": "ping",
		"pong":    "",
		"$unset":  "foo",
	})

	s.checkSettings(c, map[string]string{
		"foo":     "bar",
		"baz.qux": "ping",
		// pong: "" value is ignored
		"$unset": "foo",
	})
}

func (s *ApplicationLeaderSuite) TestOverwrite(c *tc.C) {
	s.writeSettings(c, map[string]string{
		"one":    "foo",
		"2.0":    "bar",
		"$three": "baz",
		"fo-ur":  "qux",
	})

	s.writeSettings(c, map[string]string{
		"one":    "",
		"2.0":    "ping",
		"$three": "pong",
		"$unset": "2.0",
	})

	s.checkSettings(c, map[string]string{
		// one: "" value is cleared
		"2.0":    "ping",
		"$three": "pong",
		"fo-ur":  "qux",
		"$unset": "2.0",
	})
}

func (s *ApplicationLeaderSuite) TestTxnRevnoChange(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		s.writeSettings(c, map[string]string{
			"other":   "values",
			"slipped": "in",
			"before":  "we",
			"managed": "to",
		})
	}).Check()

	s.writeSettings(c, map[string]string{
		"but":       "we",
		"overwrite": "those",
		"before":    "",
	})

	s.checkSettings(c, map[string]string{
		"other":     "values",
		"slipped":   "in",
		"but":       "we",
		"managed":   "to",
		"overwrite": "those",
	})
}

func (s *ApplicationLeaderSuite) TestTokenError(c *tc.C) {
	err := s.application.UpdateLeaderSettings(&failToken{}, map[string]string{"blah": "blah"})
	c.Check(err, tc.ErrorMatches, `application "mysql": checking leadership continuity: something bad happened`)
}

func (s *ApplicationLeaderSuite) TestReadWriteDying(c *tc.C) {
	s.preventRemove(c)
	s.destroyApplication(c)

	s.writeSettings(c, map[string]string{
		"this":  "should",
		"still": "work",
	})
	s.checkSettings(c, map[string]string{
		"this":  "should",
		"still": "work",
	})
}

func (s *ApplicationLeaderSuite) TestReadRemoved(c *tc.C) {
	s.destroyApplication(c)

	actual, err := s.application.LeaderSettings()
	c.Check(err, tc.ErrorMatches, `application "mysql" not found`)
	c.Check(err, tc.Satisfies, errors.IsNotFound)
	c.Check(actual, tc.IsNil)
}

func (s *ApplicationLeaderSuite) TestWriteRemoved(c *tc.C) {
	s.destroyApplication(c)

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"should": "fail",
	})
	c.Check(err, tc.ErrorMatches, `application "mysql" not found`)
	c.Check(err, tc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationLeaderSuite) TestWatchInitialEvent(c *tc.C) {
	w := s.application.WatchLeaderSettings()
	defer testing.AssertStop(c, w)

	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) TestWatchDetectChange(c *tc.C) {
	w := s.application.WatchLeaderSettings()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) TestWatchIgnoreNullChange(c *tc.C) {
	w := s.application.WatchLeaderSettings()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	err = s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationLeaderSuite) TestWatchCoalesceChanges(c *tc.C) {
	w := s.application.WatchLeaderSettings()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"very": "excitingly",
	})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) writeSettings(c *tc.C, update map[string]string) {
	err := s.application.UpdateLeaderSettings(&fakeToken{}, update)
	c.Check(err, tc.ErrorIsNil)
}

func (s *ApplicationLeaderSuite) checkSettings(c *tc.C, expect map[string]string) {
	actual, err := s.application.LeaderSettings()
	c.Check(err, tc.ErrorIsNil)
	c.Check(actual, tc.DeepEquals, expect)
}

func (s *ApplicationLeaderSuite) preventRemove(c *tc.C) {
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
}

func (s *ApplicationLeaderSuite) destroyApplication(c *tc.C) {
	killApplication, err := s.State.Application(s.application.Name())
	c.Assert(err, tc.ErrorIsNil)
	err = killApplication.Destroy()
	c.Assert(err, tc.ErrorIsNil)
}

// fakeToken implements leadership.Token.
type fakeToken struct {
	err error
}

// Check is part of the leadership.Token interface. It returns its
// contained error (which defaults to nil), and never checks or writes
// the userdata.
func (t *fakeToken) Check() error {
	return t.err
}

// failToken implements leadership.Token.
type failToken struct{}

// Check is part of the leadership.Token interface. It always returns an error.
func (*failToken) Check() error {
	return errors.New("something bad happened")
}
