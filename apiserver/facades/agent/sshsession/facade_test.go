// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/agent/sshsession"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

func TestSshreqconnSuite(t *tctesting.T) {
	tc.Run(t, &sshreqconnSuite{})
}

type sshreqconnSuite struct {
	ctxMock        *MockContext
	backendMock    *MockBackend
	resourceMock   *MockResources
	authorizerMock *MockAuthorizer
}

func (s *sshreqconnSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.ctxMock = NewMockContext(ctrl)
	s.backendMock = NewMockBackend(ctrl)
	s.resourceMock = NewMockResources(ctrl)
	s.authorizerMock = NewMockAuthorizer(ctrl)
	return ctrl
}

func (s *sshreqconnSuite) TestAuth(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Auth().Return(s.authorizerMock)
	s.authorizerMock.EXPECT().AuthMachineAgent().Return(false)

	_, err := sshsession.NewExternalFacade(s.ctxMock)
	c.Assert(err, tc.ErrorMatches, `permission denied`)
}

func (s *sshreqconnSuite) TestGetSSHConnRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	s.backendMock.EXPECT().GetSSHConnRequest("doc-id").Return(state.SSHConnRequest{
		Username: "username",
		Password: "password",
	}, nil)

	result, err := f.GetSSHConnRequest("doc-id")
	c.Assert(err, tc.IsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.SSHConnRequest.Username, tc.Equals, "username")
	c.Assert(result.SSHConnRequest.Password, tc.Equals, "password")
}

func (s *sshreqconnSuite) TestWatchSSHConnReq(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sshConnChanges := make(chan []string, 1)
	watcher := statetesting.NewMockStringsWatcher(sshConnChanges)
	defer workertest.DirtyKill(c, watcher)

	s.ctxMock.EXPECT().Resources().Return(s.resourceMock)
	s.backendMock.EXPECT().WatchSSHConnRequest("").Return(watcher).AnyTimes()
	s.resourceMock.EXPECT().Register(watcher).Return("id").AnyTimes()

	f := sshsession.NewFacade(s.ctxMock, s.backendMock)

	sshConnChanges <- []string{"doc-id"}
	result, err := f.WatchSSHConnRequest("")
	c.Assert(err, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "id")
	c.Assert(result.Changes, tc.DeepEquals, []string{"doc-id"})

	sshConnChanges <- []string{"doc-id2"}
	result, err = f.WatchSSHConnRequest("")
	c.Assert(err, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "id")
	c.Assert(result.Changes, tc.DeepEquals, []string{"doc-id2"})
}
