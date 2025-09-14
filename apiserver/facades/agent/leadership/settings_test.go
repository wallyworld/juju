// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/agent/leadership"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

// TODO(fwereade): this is *severely* undertested.
type settingsSuite struct {
	testhelpers.IsolationSuite
}

func TestSettingsSuite(t *tctesting.T) {
	tc.Run(t, &settingsSuite{})
}

func (s *settingsSuite) TestReadSettings(c *tc.C) {

	settingsToReturn := params.Settings(map[string]string{"foo": "bar"})
	numGetSettingCalls := 0
	getSettings := func(serviceId string) (map[string]string, error) {
		numGetSettingCalls++
		c.Check(serviceId, tc.Equals, StubAppNm)
		return settingsToReturn, nil
	}
	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, getSettings, nil, nil)

	results, err := accessor.Read(params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(StubAppNm).String()},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(numGetSettingCalls, tc.Equals, 1)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[0].Settings, tc.DeepEquals, settingsToReturn)
}

func (s *settingsSuite) TestWriteSettings(c *tc.C) {

	expectToken := &fakeToken{}

	numLeaderCheckCalls := 0
	leaderCheck := func(appName, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		c.Check(appName, tc.Equals, StubAppNm)
		c.Check(unitId, tc.Equals, StubUnitNm)
		return expectToken
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		c.Check(serviceId, tc.Equals, StubAppNm)
		c.Check(token, tc.Equals, expectToken)
		c.Check(settings, tc.DeepEquals, map[string]string{"baz": "biz"})
		return nil
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
	c.Check(numWriteSettingCalls, tc.Equals, 1)
	c.Check(numLeaderCheckCalls, tc.Equals, 1)
}

func (s *settingsSuite) TestWriteSettingsWrongUnit(c *tc.C) {

	numLeaderCheckCalls := 0
	leaderCheck := func(appName, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		return &fakeToken{}
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		return nil
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				UnitTag:        names.NewUnitTag("foo/0").String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Check(numWriteSettingCalls, tc.Equals, 0)
	c.Check(numLeaderCheckCalls, tc.Equals, 0)
}

func (s *settingsSuite) TestWriteSettingsError(c *tc.C) {

	expectToken := &fakeToken{}

	numLeaderCheckCalls := 0
	leaderCheck := func(serviceId, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		c.Check(serviceId, tc.Equals, StubAppNm)
		c.Check(unitId, tc.Equals, StubUnitNm)
		return expectToken
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		c.Check(serviceId, tc.Equals, StubAppNm)
		c.Check(token, tc.Equals, expectToken)
		c.Check(settings, tc.DeepEquals, map[string]string{"baz": "biz"})
		return errors.New("zap blort")
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "zap blort")
	c.Check(numWriteSettingCalls, tc.Equals, 1)
	c.Check(numLeaderCheckCalls, tc.Equals, 1)
}

func (s *settingsSuite) TestBlockUntilChanges(c *tc.C) {

	numSettingsWatcherCalls := 0
	registerWatcher := func(appName string) (string, error) {
		numSettingsWatcherCalls++
		c.Check(appName, tc.Equals, StubAppNm)
		return "foo", nil
	}

	authorizer := &stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, registerWatcher, nil, nil, nil)

	results, err := accessor.WatchLeadershipSettings(params.Entities{Entities: []params.Entity{
		{Tag: names.NewApplicationTag(StubAppNm).String()},
	}})
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

type fakeToken struct {
	coreleadership.Token
}
