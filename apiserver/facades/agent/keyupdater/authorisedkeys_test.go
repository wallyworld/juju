// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/keyupdater"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type authorisedKeysSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine       *state.Machine
	unrelatedMachine *state.Machine
	keyupdater       *keyupdater.KeyUpdaterAPI
	resources        *common.Resources
	authorizer       apiservertesting.FakeAuthorizer
}

func TestAuthorisedKeysSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &authorisedKeysSuite{})
}

func (s *authorisedKeysSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	// Create machines to work with
	var err error
	s.rawMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	s.unrelatedMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// The default auth is as a controller
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
	s.keyupdater, err = keyupdater.NewKeyUpdaterAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *authorisedKeysSuite) TestNewKeyUpdaterAPIAcceptsController(c *tc.C) {
	endPoint, err := keyupdater.NewKeyUpdaterAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(endPoint, tc.NotNil)
}

func (s *authorisedKeysSuite) TestNewKeyUpdaterAPIRefusesNonMachineAgent(c *tc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewUnitTag("ubuntu/1")
	endPoint, err := keyupdater.NewKeyUpdaterAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthoriser,
	})
	c.Assert(endPoint, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *authorisedKeysSuite) TestWatchAuthorisedKeysNothing(c *tc.C) {
	// Not an error to watch nothing
	results, err := s.keyupdater.WatchAuthorisedKeys(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *authorisedKeysSuite) setAuthorizedKeys(c *tc.C, keys string) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": keys}, nil)
	c.Assert(err, tc.ErrorIsNil)
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelConfig.AuthorizedKeys(), tc.Equals, keys)
}

func (s *authorisedKeysSuite) TestWatchAuthorisedKeys(c *tc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.rawMachine.Tag().String()},
			{Tag: s.unrelatedMachine.Tag().String()},
			{Tag: "machine-42"},
		},
	}
	results, err := s.keyupdater.WatchAuthorisedKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
	c.Assert(results.Results[0].NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Assert(results.Results[0].Error, tc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, tc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	s.setAuthorizedKeys(c, "key1\nkey2")

	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *authorisedKeysSuite) TestAuthorisedKeysForNoone(c *tc.C) {
	// Not an error to request nothing, dumb, but not an error.
	results, err := s.keyupdater.AuthorisedKeys(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *authorisedKeysSuite) TestAuthorisedKeys(c *tc.C) {
	s.setAuthorizedKeys(c, "key1\nkey2")

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.rawMachine.Tag().String()},
			{Tag: s.unrelatedMachine.Tag().String()},
			{Tag: "machine-42"},
		},
	}
	results, err := s.keyupdater.AuthorisedKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"key1", "key2"}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
