// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/logfwd"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type LastSentSuite struct {
	testhelpers.IsolationSuite

	stub       *testhelpers.Stub
	state      *stubState
	authorizer apiservertesting.FakeAuthorizer
}

func TestLastSentSuite(t *tctesting.T) {
	tc.Run(t, &LastSentSuite{})
}

func (s *LastSentSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.state = &stubState{stub: s.stub}
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("99"),
		Controller: true,
	}
}

func (s *LastSentSuite) TestAuthRefusesUser(c *tc.C) {
	anAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("bob"),
	}

	_, err := logfwd.NewLogForwardingAPI(s.state, anAuthorizer)

	c.Check(err, tc.ErrorMatches, "permission denied")
}

func (s *LastSentSuite) TestAuthRefusesNonController(c *tc.C) {
	anAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}

	_, err := logfwd.NewLogForwardingAPI(s.state, anAuthorizer)

	c.Check(err, tc.ErrorMatches, "permission denied")
}

func (s *LastSentSuite) TestGetLastSentOne(c *tc.C) {
	tracker := s.state.addTracker()
	tracker.ReturnGet = 10
	api, err := logfwd.NewLogForwardingAPI(s.state, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag.String(),
			Sink:     "spam",
		}},
	})

	c.Check(res, tc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			RecordID:        10,
			RecordTimestamp: 100,
		}},
	})
	s.stub.CheckCallNames(c, "NewLastSentTracker", "Get", "Close")
	s.stub.CheckCall(c, 0, "NewLastSentTracker", modelTag, "spam")
}

func (s *LastSentSuite) TestGetLastSentBulk(c *tc.C) {
	trackerSpam := s.state.addTracker()
	trackerSpam.ReturnGet = 10
	trackerEggs := s.state.addTracker()
	trackerEggs.ReturnGet = 20
	s.state.addTracker() // ham
	s.stub.SetErrors(nil, nil, nil, nil, state.ErrNeverForwarded)
	api, err := logfwd.NewLogForwardingAPI(s.state, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag.String(),
			Sink:     "spam",
		}, {
			ModelTag: modelTag.String(),
			Sink:     "eggs",
		}, {
			ModelTag: modelTag.String(),
			Sink:     "ham",
		}},
	})

	c.Check(res, tc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			RecordID:        10,
			RecordTimestamp: 100,
		}, {
			RecordID:        20,
			RecordTimestamp: 200,
		}, {
			Error: &params.Error{
				Message: `cannot find ID of the last forwarded record`,
				Code:    params.CodeNotFound,
			},
		}},
	})
	s.stub.CheckCallNames(c,
		"NewLastSentTracker", "Get", "Close",
		"NewLastSentTracker", "Get", "Close",
		"NewLastSentTracker", "Get", "Close",
	)
	s.stub.CheckCall(c, 0, "NewLastSentTracker", modelTag, "spam")
	s.stub.CheckCall(c, 3, "NewLastSentTracker", modelTag, "eggs")
	s.stub.CheckCall(c, 6, "NewLastSentTracker", modelTag, "ham")
}

func (s *LastSentSuite) TestSetLastSentOne(c *tc.C) {
	s.state.addTracker()
	api, err := logfwd.NewLogForwardingAPI(s.state, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "spam",
			},
			RecordID:        10,
			RecordTimestamp: 100,
		}},
	})

	c.Check(res, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}},
	})
	s.stub.CheckCallNames(c, "NewLastSentTracker", "Set", "Close")
	s.stub.CheckCall(c, 0, "NewLastSentTracker", modelTag, "spam")
	s.stub.CheckCall(c, 1, "Set", int64(10), int64(100))
}

func (s *LastSentSuite) TestSetLastSentBulk(c *tc.C) {
	s.state.addTracker() // spam
	s.state.addTracker() // eggs
	s.state.addTracker() // ham
	failure := errors.New("<failed>")
	s.stub.SetErrors(nil, nil, failure)
	api, err := logfwd.NewLogForwardingAPI(s.state, s.authorizer)
	c.Assert(err, tc.ErrorIsNil)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "spam",
			},
			RecordID:        10,
			RecordTimestamp: 100,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "eggs",
			},
			RecordID:        20,
			RecordTimestamp: 200,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "ham",
			},
			RecordID:        15,
			RecordTimestamp: 150,
		}},
	})

	c.Check(res, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: apiservererrors.ServerError(failure),
		}, {
			Error: nil,
		}},
	})
	s.stub.CheckCallNames(c,
		"NewLastSentTracker", "Set", "Close",
		"NewLastSentTracker", "Set", "Close",
		"NewLastSentTracker", "Set", "Close",
	)
	s.stub.CheckCall(c, 0, "NewLastSentTracker", modelTag, "spam")
	s.stub.CheckCall(c, 1, "Set", int64(10), int64(100))
	s.stub.CheckCall(c, 3, "NewLastSentTracker", modelTag, "eggs")
	s.stub.CheckCall(c, 4, "Set", int64(20), int64(200))
	s.stub.CheckCall(c, 6, "NewLastSentTracker", modelTag, "ham")
	s.stub.CheckCall(c, 7, "Set", int64(15), int64(150))
}

type stubState struct {
	stub *testhelpers.Stub

	ReturnNewLastSentTracker []logfwd.LastSentTracker
}

func (s *stubState) addTracker() *stubTracker {
	tracker := &stubTracker{stub: s.stub}
	s.ReturnNewLastSentTracker = append(s.ReturnNewLastSentTracker, tracker)
	return tracker
}

func (s *stubState) NewLastSentTracker(tag names.ModelTag, sink string) logfwd.LastSentTracker {
	s.stub.AddCall("NewLastSentTracker", tag, sink)
	if len(s.ReturnNewLastSentTracker) == 0 {
		panic("ran out of trackers")
	}
	tracker := s.ReturnNewLastSentTracker[0]
	s.ReturnNewLastSentTracker = s.ReturnNewLastSentTracker[1:]
	return tracker
}

type stubTracker struct {
	stub *testhelpers.Stub

	ReturnGet int64
}

func (s *stubTracker) Get() (int64, int64, error) {
	s.stub.AddCall("Get")
	if err := s.stub.NextErr(); err != nil {
		return 0, 0, err
	}

	return s.ReturnGet, s.ReturnGet * 10, nil
}

func (s *stubTracker) Set(recID int64, recTimestamp int64) error {
	s.stub.AddCall("Set", recID, recTimestamp)
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}

func (s *stubTracker) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	return nil
}
