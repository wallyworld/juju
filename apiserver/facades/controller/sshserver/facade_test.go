// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/sshserver"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
)

func TestSshserverSuite(t *tctesting.T) {
	tc.Run(t, &sshserverSuite{})
}

type sshserverSuite struct {
	ctxMock       *MockContext
	backendMock   *MockBackend
	resourcesMock *MockResources
}

func (s *sshserverSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctxMock = NewMockContext(ctrl)
	s.backendMock = NewMockBackend(ctrl)
	s.resourcesMock = NewMockResources(ctrl)
	return ctrl
}

func (s *sshserverSuite) TestAuth(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	authorizer := NewMockAuthorizer(ctrl)

	s.ctxMock.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := sshserver.NewExternalFacade(s.ctxMock)
	c.Assert(err, tc.ErrorMatches, `permission denied`)
}

func (s *sshserverSuite) TestControllerConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().ControllerConfig().Return(
		controller.Config{"hi": "bye"},
		nil,
	)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	cfg, err := f.ControllerConfig()
	c.Assert(err, tc.IsNil)
	c.Assert(cfg, tc.DeepEquals, params.ControllerConfigResult{Config: params.ControllerConfig{"hi": "bye"}})
}

func (s *sshserverSuite) TestWatchControllerConfig(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	watcher := workertest.NewFakeWatcher(1, 0)
	watcher.Ping() // Send some changes

	s.ctxMock.EXPECT().Resources().Return(s.resourcesMock)
	s.backendMock.EXPECT().WatchControllerConfig().Return(watcher, nil)
	s.resourcesMock.EXPECT().Register(watcher).Return("id")

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	result, err := f.WatchControllerConfig()
	c.Assert(err, tc.IsNil)
	c.Assert(result.NotifyWatcherId, tc.Equals, "id")

	// Now we close the channel expecting err
	watcher.Close()
	s.backendMock.EXPECT().WatchControllerConfig().Return(watcher, nil)

	_, err = f.WatchControllerConfig()
	c.Assert(err, tc.ErrorMatches, "An error")
}

func (s *sshserverSuite) TestSSHServerHostKey(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().SSHServerHostKey().Return("hostkey", nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	key, err := f.SSHServerHostKey()
	c.Assert(err, tc.IsNil)
	c.Assert(key, tc.Equals, params.StringResult{Result: "hostkey"})
}

func (s *sshserverSuite) TestHostKeyForTarget(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().HostKeyForVirtualHostname(gomock.Any()).Return([]byte("hostkey"), nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	key, err := f.VirtualHostKey(params.SSHVirtualHostKeyRequestArg{Hostname: "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local"})
	c.Assert(err, tc.IsNil)
	c.Assert(key, tc.DeepEquals, params.SSHHostKeyResult{HostKey: []byte("hostkey")})
}

func (s *sshserverSuite) TestAuthorizedKeysForModel(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.ctxMock.EXPECT().Resources().Times(1)
	s.backendMock.EXPECT().AuthorizedKeysForModel("abcd").Return(
		[]string{"key1", "key2"}, nil)

	s.backendMock.EXPECT().AuthorizedKeysForModel("not-existing").Return(
		[]string{""}, nil)

	f := sshserver.NewFacade(s.ctxMock, s.backendMock)

	testCases := []struct {
		name            string
		expectKeys      []string
		modelUUID       string
		expectedSuccess bool
		expectedError   string
	}{
		{
			name:            "test for key added to a model",
			modelUUID:       "abcd",
			expectKeys:      []string{"key1", "key2"},
			expectedSuccess: true,
		},
		{
			name:       "test for not-existing model",
			modelUUID:  "not-existing",
			expectKeys: []string{""},
		},
	}

	for _, tc := range testCases {
		c.Logf("test: %s", tc.name)
		arg := params.ListAuthorizedKeysArgs{
			ModelUUID: tc.modelUUID,
		}
		results, err := f.ListAuthorizedKeysForModel(arg)
		c.Assert(err, tc.IsNil)
		c.Assert(results.Error, tc.IsNil)
		c.Assert(results.AuthorizedKeys, tc.DeepEquals, tc.expectKeys)
	}
}
