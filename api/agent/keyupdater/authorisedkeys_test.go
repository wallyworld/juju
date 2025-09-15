// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type keyupdaterSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	keyupdater *keyupdater.State
}

func TestKeyupdaterSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &keyupdaterSuite{})
}

func (s *keyupdaterSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var stateAPI api.Connection
	stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	c.Assert(stateAPI, tc.NotNil)
	s.keyupdater = keyupdater.NewState(stateAPI)
	c.Assert(s.keyupdater, tc.NotNil)
}

func (s *keyupdaterSuite) TestAuthorisedKeysNoSuchMachine(c *tc.C) {
	_, err := s.keyupdater.AuthorisedKeys(names.NewMachineTag("42"))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *keyupdaterSuite) TestAuthorisedKeysForbiddenMachine(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.keyupdater.AuthorisedKeys(m.Tag().(names.MachineTag))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *keyupdaterSuite) TestAuthorisedKeys(c *tc.C) {
	s.setAuthorisedKeys(c, "key1\nkey2")
	keys, err := s.keyupdater.AuthorisedKeys(s.rawMachine.Tag().(names.MachineTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.DeepEquals, []string{"key1", "key2"})
}

func (s *keyupdaterSuite) setAuthorisedKeys(c *tc.C, keys string) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": keys}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *keyupdaterSuite) TestWatchAuthorisedKeys(c *tc.C) {
	watcher, err := s.keyupdater.WatchAuthorisedKeys(s.rawMachine.Tag().(names.MachineTag))
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, watcher)
	defer wc.AssertStops()

	// Initial event
	wc.AssertOneChange()

	s.setAuthorisedKeys(c, "key1\nkey2")
	// One change noticing the new version
	wc.AssertOneChange()
	// Setting the version to the same value doesn't trigger a change
	s.setAuthorisedKeys(c, "key1\nkey2")
	wc.AssertNoChange()

	s.setAuthorisedKeys(c, "key1\nkey2\nkey3")
	wc.AssertOneChange()
}
