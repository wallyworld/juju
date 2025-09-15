// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	tctesting "testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

type logtransferSuite struct {
	apiserverBaseSuite
	userTag         names.UserTag
	password        string
	machineTag      names.MachineTag
	machinePassword string
	logs            loggo.TestWriter
	url             string
}

func TestLogtransferSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &logtransferSuite{})
}

func (s *logtransferSuite) SetUpTest(c *tc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.password = "jabberwocky"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: s.password})
	s.userTag = u.Tag().(names.UserTag)
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "nonce",
	})
	s.machineTag = m.Tag().(names.MachineTag)
	s.machinePassword = password
	s.setUserAccess(c, permission.SuperuserAccess)

	url := s.URL("/migrate/logtransfer", nil)
	url.Scheme = "wss"
	s.url = url.String()

	s.logs.Clear()
	writer := loggo.NewMinimumLevelWriter(&s.logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), tc.ErrorIsNil)
}

func (s *logtransferSuite) makeAuthHeader() http.Header {
	header := jujuhttp.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHTTPHeader, s.State.ModelUUID())
	header.Add(params.JujuClientVersion, version.Current.String())
	return header
}

func (s *logtransferSuite) dialWebsocket(c *tc.C) *websocket.Conn {
	return s.dialWebsocketInternal(c, s.makeAuthHeader())
}

func (s *logtransferSuite) dialWebsocketInternal(c *tc.C, header http.Header) *websocket.Conn {
	conn, _, err := dialWebsocketFromURL(c, s.url, header)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(_ *tc.C) { conn.Close() })
	return conn
}

func (s *logtransferSuite) checkAuthFails(c *tc.C, header http.Header, code int, message string) {
	_, resp, err := dialWebsocketFromURL(c, s.url, header)
	c.Assert(err, tc.Equals, websocket.ErrBadHandshake)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, code)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(body), tc.Matches, message+"\n")
}

func (s *logtransferSuite) TestRejectsMissingModelHeader(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.userTag.String(), s.password)
	ws := s.dialWebsocketInternal(c, header)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: unknown model: ""`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestRejectsBadMigratingModelUUID(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHTTPHeader, "does-not-exist")
	ws := s.dialWebsocketInternal(c, header)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: unknown model: "does-not-exist"`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestRejectsInvalidVersion(c *tc.C) {
	url := s.URL("/migrate/logtransfer", nil)
	url.Scheme = "wss"
	hdr := s.makeAuthHeader()
	hdr.Set("X-Juju-ClientVersion", "blah")
	conn, _, err := dialWebsocketFromURL(c, url.String(), hdr)
	c.Assert(err, tc.ErrorIsNil)
	defer conn.Close()
	websockettest.AssertJSONError(c, conn, `^initialising migration logsink session: invalid X-Juju-ClientVersion "blah".*`)
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logtransferSuite) TestRejectsMachineLogins(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.machineTag.String(), s.machinePassword)
	header.Add(params.MachineNonceHeader, "nonce")
	s.checkAuthFails(c, header, http.StatusForbidden, "authorization failed: machine 0 is not a user")
}

func (s *logtransferSuite) TestRejectsBadPasword(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.userTag.String(), "wrong")
	header.Add(params.MigrationModelHTTPHeader, s.State.ModelUUID())
	s.checkAuthFails(c, header, http.StatusUnauthorized, "authentication failed: invalid entity name or password")
}

func (s *logtransferSuite) TestRequiresSuperUser(c *tc.C) {
	s.setUserAccess(c, permission.LoginAccess)
	s.checkAuthFails(c, s.makeAuthHeader(), http.StatusForbidden, "authorization failed: user .* is not a controller admin")
}

func (s *logtransferSuite) TestRequiresMigrationModeNone(c *tc.C) {
	s.setMigrationMode(c, state.MigrationModeImporting)
	ws := s.dialWebsocket(c)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: model migration mode is "importing" instead of ""`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestLogging(c *tc.C) {
	conn := s.dialWebsocket(c)

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	})
	c.Assert(err, tc.ErrorIsNil)

	t1 := time.Date(2015, time.June, 1, 23, 2, 2, 0, time.UTC)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-101",
		Time:     t1,
		Module:   "else.where",
		Location: "bar.go:99",
		Level:    loggo.ERROR.String(),
		Message:  "oh noes",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Wait for the log documents to be written to the DB.
	logsColl := s.State.MongoSession().DB("logs").C("logs." + s.State.ModelUUID())
	var docs []bson.M
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		err := logsColl.Find(nil).Sort("t").All(&docs)
		c.Assert(err, tc.ErrorIsNil)
		if len(docs) == 2 {
			break
		}
		if len(docs) >= 2 {
			c.Fatalf("saw more log documents than expected")
		}
		if !a.HasNext() {
			c.Fatalf("timed out waiting for log writes")
		}
	}

	// Check the recorded logs are correct.
	c.Assert(docs[0]["t"], tc.Equals, t0.UnixNano())
	c.Assert(docs[0]["n"], tc.Equals, "machine-23")
	c.Assert(docs[0]["m"], tc.Equals, "some.where")
	c.Assert(docs[0]["l"], tc.Equals, "foo.go:42")
	c.Assert(docs[0]["v"], tc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], tc.Equals, "all is well")

	c.Assert(docs[1]["t"], tc.Equals, t1.UnixNano())
	c.Assert(docs[1]["n"], tc.Equals, "machine-101")
	c.Assert(docs[1]["m"], tc.Equals, "else.where")
	c.Assert(docs[1]["l"], tc.Equals, "bar.go:99")
	c.Assert(docs[1]["v"], tc.Equals, int(loggo.ERROR))
	c.Assert(docs[1]["x"], tc.Equals, "oh noes")

	// Close connection.
	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that no error is logged when the connection is closed
	// normally.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.ShortWait,
		Delay: 2 * time.Millisecond,
	}
	for a := shortAttempt.Start(); a.Next(); {
		for _, log := range s.logs.Log() {
			c.Assert(log.Level, tc.LessThan, loggo.ERROR, tc.Commentf("log: %#v", log))
		}
	}
}

func (s *logtransferSuite) TestTracksLastSentLogTime(c *tc.C) {
	conn := s.dialWebsocket(c)

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	tracker := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "migration-logtransfer")
	defer tracker.Close()

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	})
	c.Assert(err, tc.ErrorIsNil)

	// First message time is tracked.
	assertTrackerTime(c, tracker, t0)

	// Doesn't track anything more until a log message 2 mins later.
	t1 := t0.Add(2*time.Minute - 1*time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t1,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "still good",
	})
	c.Assert(err, tc.ErrorIsNil)

	// No change
	assertTrackerTime(c, tracker, t0)

	t2 := t1.Add(1 * time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t2,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "nae bather",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Updated
	assertTrackerTime(c, tracker, t2)

	t3 := t2.Add(1 * time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t3,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "sweet as",
	})
	c.Assert(err, tc.ErrorIsNil)

	// No change,
	assertTrackerTime(c, tracker, t2)

	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Latest is saved when connection is closed.
	assertTrackerTime(c, tracker, t3)
}

func assertTrackerTime(c *tc.C, tracker *state.LastSentLogTracker, expected time.Time) {
	var timestamp int64
	var err error
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, timestamp, err = tracker.Get()
		if err != nil && errors.Cause(err) != state.ErrNeverForwarded {
			c.Assert(err, tc.ErrorIsNil)
		}
		if err == nil && timestamp == expected.UnixNano() {
			return
		}
	}
	c.Fatalf("tracker never set to %d - last seen was %d (err: %v)", expected.UnixNano(), timestamp, err)
}

func (s *logtransferSuite) setUserAccess(c *tc.C, level permission.Access) {
	_, err := s.State.SetUserAccess(s.userTag, s.State.ControllerTag(), level)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *logtransferSuite) setMigrationMode(c *tc.C, mode state.MigrationMode) {
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetMigrationMode(mode)
	c.Assert(err, tc.ErrorIsNil)
}
