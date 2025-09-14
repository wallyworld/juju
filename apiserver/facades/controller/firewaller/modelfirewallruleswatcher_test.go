// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/facades/controller/firewaller/mocks"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	statetesting "github.com/juju/juju/state/testing"
)

func TestModelFirewallRulesWatcherSuite(t *tctesting.T) {
	tc.Run(t, &ModelFirewallRulesWatcherSuite{})
}

type ModelFirewallRulesWatcherSuite struct {
	st *mocks.MockState
}

func (s *ModelFirewallRulesWatcherSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = mocks.NewMockState(ctrl)

	return ctrl
}

func cfg(c *tc.C, in map[string]interface{}) *config.Config {
	attrs := coretesting.FakeConfig().Merge(in)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func mockNotifyWatcher(ctrl *gomock.Controller) (*mocks.MockNotifyWatcher, chan struct{}) {
	ch := make(chan struct{})
	watcher := mocks.NewMockNotifyWatcher(ctrl)
	watcher.EXPECT().Changes().Return(ch).MinTimes(1)
	watcher.EXPECT().Wait().AnyTimes()
	watcher.EXPECT().Kill().AnyTimes()
	watcher.EXPECT().Stop().AnyTimes()
	return watcher, ch
}

func (s *ModelFirewallRulesWatcherSuite) TestInitial(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)
	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig().Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, tc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)
}

func (s *ModelFirewallRulesWatcherSuite) TestConfigChange(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)

	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig().Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.st.EXPECT().ModelConfig().Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "192.168.0.0/24"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, tc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)

	// Config change
	notifyCh <- struct{}{}
	wc.AssertChanges(1)
}

func (s *ModelFirewallRulesWatcherSuite) TestIrrelevantConfigChange(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)

	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig().Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.st.EXPECT().ModelConfig().Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, tc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)

	// Config change
	notifyCh <- struct{}{}
	wc.AssertNoChange()
}
