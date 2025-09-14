// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"math/rand"
	"strconv"
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	corelogger "github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

type LogCollectionSuite struct {
	ConnSuite
}

func TestLogCollectionSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &LogCollectionSuite{})
}

func (s *LogCollectionSuite) TestCreateCollection(c *tc.C) {
	session := s.State.MongoSession()
	modelUUID := "00000000-0000-0000-0000-000000000001"

	coll := session.DB("logs").C("logs." + modelUUID)

	// Loop to test idempotency.
	for i := 0; i < 2; i++ {
		err := state.InitDbLogsForModel(s.Session, modelUUID, 1)
		c.Assert(err, tc.ErrorIsNil)
		capped, size, err := state.GetCollectionCappedInfo(coll)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(capped, tc.IsTrue)
		c.Assert(size, tc.Equals, 1)
	}
}

func (s *LogCollectionSuite) TestUpgradeCollection(c *tc.C) {
	session := s.State.MongoSession()
	modelUUID := "00000000-0000-0000-0000-000000000002"

	coll := session.DB("logs").C("logs." + modelUUID)
	// Create a non-capped collection.
	err := coll.Create(&mgo.CollectionInfo{})
	c.Assert(err, tc.ErrorIsNil)
	capped, size, err := state.GetCollectionCappedInfo(coll)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capped, tc.IsFalse)
	c.Assert(size, tc.Equals, 0)

	// Ensure collection is "upgraded" to a capped collection.
	err = state.InitDbLogsForModel(s.Session, modelUUID, 1)
	c.Assert(err, tc.ErrorIsNil)
	capped, size, err = state.GetCollectionCappedInfo(coll)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capped, tc.IsTrue)
	c.Assert(size, tc.Equals, 1)
}

func (s *LogCollectionSuite) TestResizeCollection(c *tc.C) {
	session := s.State.MongoSession()
	modelUUID := "00000000-0000-0000-0000-000000000003"

	coll := session.DB("logs").C("logs." + modelUUID)
	// Create a small collection.
	err := state.InitDbLogsForModel(s.Session, modelUUID, 2)
	c.Assert(err, tc.ErrorIsNil)
	capped, size, err := state.GetCollectionCappedInfo(coll)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capped, tc.IsTrue)
	c.Assert(size, tc.Equals, 2)

	// Make it bigger.
	err = state.InitDbLogsForModel(s.Session, modelUUID, 3)
	c.Assert(err, tc.ErrorIsNil)
	capped, size, err = state.GetCollectionCappedInfo(coll)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capped, tc.IsTrue)
	c.Assert(size, tc.Equals, 3)

	// Make it even smaller.
	err = state.InitDbLogsForModel(s.Session, modelUUID, 1)
	c.Assert(err, tc.ErrorIsNil)
	capped, size, err = state.GetCollectionCappedInfo(coll)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capped, tc.IsTrue)
	c.Assert(size, tc.Equals, 1)
}

type LogsSuite struct {
	ConnSuite
	logsColl *mgo.Collection

	logger loggo.Logger
}

func TestLogsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &LogsSuite{})
}

func (s *LogsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.logsColl = s.logCollFor(s.State)
	s.logger = loggo.GetLogger("test")
}

func (s *LogsSuite) logCollFor(st *state.State) *mgo.Collection {
	session := st.MongoSession()
	return session.DB("logs").C("logs." + st.ModelUUID())
}

func (s *LogsSuite) TestLastSentLogTrackerSetGet(c *tc.C) {
	tracker := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "test-sink")
	defer tracker.Close()

	err := tracker.Set(10, 100)
	c.Assert(err, tc.ErrorIsNil)
	id1, ts1, err := tracker.Get()
	c.Assert(err, tc.ErrorIsNil)
	err = tracker.Set(20, 200)
	c.Assert(err, tc.ErrorIsNil)
	id2, ts2, err := tracker.Get()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(id1, tc.Equals, int64(10))
	c.Check(ts1, tc.Equals, int64(100))
	c.Check(id2, tc.Equals, int64(20))
	c.Check(ts2, tc.Equals, int64(200))
}

func (s *LogsSuite) TestLastSentLogTrackerGetNeverSet(c *tc.C) {
	tracker := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "test")
	defer tracker.Close()

	_, _, err := tracker.Get()

	c.Check(err, tc.ErrorMatches, state.ErrNeverForwarded.Error())
}

func (s *LogsSuite) TestLastSentLogTrackerIndependentModels(c *tc.C) {
	tracker0 := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "test-sink")
	defer tracker0.Close()
	otherModel := s.NewStateForModelNamed(c, "test-model")
	defer otherModel.Close()
	tracker1 := state.NewLastSentLogTracker(otherModel, otherModel.ModelUUID(), "test-sink") // same sink
	defer tracker1.Close()
	err := tracker0.Set(10, 100)
	c.Assert(err, tc.ErrorIsNil)
	id0, ts0, err := tracker0.Get()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id0, tc.Equals, int64(10))
	c.Assert(ts0, tc.Equals, int64(100))

	_, _, errBefore := tracker1.Get()
	err = tracker1.Set(20, 200)
	c.Assert(err, tc.ErrorIsNil)
	id1, ts1, errAfter := tracker1.Get()
	c.Assert(errAfter, tc.ErrorIsNil)
	id0, ts0, err = tracker0.Get()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(errBefore, tc.ErrorMatches, state.ErrNeverForwarded.Error())
	c.Check(id1, tc.Equals, int64(20))
	c.Check(ts1, tc.Equals, int64(200))
	c.Check(id0, tc.Equals, int64(10))
	c.Check(ts0, tc.Equals, int64(100))
}

func (s *LogsSuite) TestLastSentLogTrackerIndependentSinks(c *tc.C) {
	tracker0 := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "test-sink0")
	defer tracker0.Close()
	tracker1 := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "test-sink1")
	defer tracker1.Close()
	err := tracker0.Set(10, 100)
	c.Assert(err, tc.ErrorIsNil)
	id0, ts0, err := tracker0.Get()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id0, tc.Equals, int64(10))
	c.Assert(ts0, tc.Equals, int64(100))

	_, _, errBefore := tracker1.Get()
	err = tracker1.Set(20, 200)
	c.Assert(err, tc.ErrorIsNil)
	id1, ts1, errAfter := tracker1.Get()
	c.Assert(errAfter, tc.ErrorIsNil)
	id0, ts0, err = tracker0.Get()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(errBefore, tc.ErrorMatches, state.ErrNeverForwarded.Error())
	c.Check(id1, tc.Equals, int64(20))
	c.Check(ts1, tc.Equals, int64(200))
	c.Check(id0, tc.Equals, int64(10))
	c.Check(ts0, tc.Equals, int64(100))
}

func (s *LogsSuite) TestIndexesCreated(c *tc.C) {
	// Indexes should be created on the logs collection when state is opened.
	indexes, err := s.logsColl.Indexes()
	c.Assert(err, tc.ErrorIsNil)
	var keys []string
	for _, index := range indexes {
		keys = append(keys, strings.Join(index.Key, "-"))
	}
	c.Assert(keys, tc.SameContents, []string{
		"_id",   // default index
		"t-_id", // timestamp and ID
		"n",     // entity
		"c",     // optional labels
	})
}

func (s *LogsSuite) TestDbLogger(c *tc.C) {
	logger := state.NewDbLogger(s.State)
	defer logger.Close()

	t0 := coretesting.ZeroTime().Truncate(time.Millisecond) // MongoDB only stores timestamps with ms precision.
	t1 := t0.Add(time.Second)
	err := logger.Log([]corelogger.LogRecord{{
		Time:     t0,
		Entity:   "machine-45",
		Module:   "some.where",
		Location: "foo.go:99",
		Level:    loggo.INFO,
		Message:  "all is well",
	}, {
		Time:     t1,
		Entity:   "machine-47",
		Module:   "else.where",
		Location: "bar.go:42",
		Level:    loggo.ERROR,
		Message:  "oh noes",
	}})
	c.Assert(err, tc.ErrorIsNil)

	var docs []bson.M
	err = s.logsColl.Find(nil).Sort("t").All(&docs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(docs, tc.HasLen, 2)

	c.Assert(docs[0]["t"], tc.Equals, t0.UnixNano())
	c.Assert(docs[0]["n"], tc.Equals, "machine-45")
	c.Assert(docs[0]["m"], tc.Equals, "some.where")
	c.Assert(docs[0]["l"], tc.Equals, "foo.go:99")
	c.Assert(docs[0]["v"], tc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], tc.Equals, "all is well")

	c.Assert(docs[1]["t"], tc.Equals, t1.UnixNano())
	c.Assert(docs[1]["n"], tc.Equals, "machine-47")
	c.Assert(docs[1]["m"], tc.Equals, "else.where")
	c.Assert(docs[1]["l"], tc.Equals, "bar.go:42")
	c.Assert(docs[1]["v"], tc.Equals, int(loggo.ERROR))
	c.Assert(docs[1]["x"], tc.Equals, "oh noes")
}

type LogTailerSuite struct {
	ConnWithWallClockSuite
	oplogColl            *mgo.Collection
	otherState           *state.State
	modelUUID, otherUUID string
}

func TestLogTailerSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &LogTailerSuite{})
}

func (s *LogTailerSuite) SetUpTest(c *tc.C) {
	s.ConnWithWallClockSuite.SetUpTest(c)

	session := s.State.MongoSession()
	// Create a fake oplog collection.
	s.oplogColl = session.DB("logs").C("oplog.fake")
	err := s.oplogColl.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 1024 * 1024,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { s.oplogColl.DropCollection() })

	s.otherState = s.NewStateForModelNamed(c, "test-model")
	c.Assert(s.otherState, tc.NotNil)
	s.AddCleanup(func(c *tc.C) {
		err := s.otherState.Close()
		c.Assert(err, tc.ErrorIsNil)
	})
	s.modelUUID = s.State.ModelUUID()
	s.otherUUID = s.otherState.ModelUUID()
}

func (s *LogTailerSuite) getCollection(modelUUID string) *mgo.Collection {
	return s.State.MongoSession().DB("logs").C("logs." + modelUUID)
}

func (s *LogTailerSuite) TestLogDeletionDuringTailing(c *tc.C) {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("test", &tw)
	c.Assert(err, tc.ErrorIsNil)
	defer loggo.RemoveWriter("test")

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{}, s.oplogColl)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()

	want := logTemplate{Message: "want"}

	s.writeLogs(c, s.otherUUID, 2, want)
	// Delete something.
	s.deleteLogOplogEntry(s.otherUUID, want)
	s.writeLogs(c, s.otherUUID, 2, want)

	s.assertTailer(c, tailer, 4, want)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{{
		Level:   loggo.WARNING,
		Message: `.*log deserialization failed.*`,
	}})
}

func (s *LogTailerSuite) TestTimeFiltering(c *tc.C) {
	// Add 10 logs that shouldn't be returned.
	threshT := coretesting.NonZeroTime()
	s.writeLogsT(c,
		s.otherUUID,
		threshT.Add(-5*time.Second), threshT.Add(-time.Millisecond), 5,
		logTemplate{Message: "dont want"},
	)

	// Add 5 logs that should be returned.
	want := logTemplate{Message: "want"}
	s.writeLogsT(c, s.otherUUID, threshT, threshT.Add(5*time.Second), 5, want)
	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{StartTime: threshT}, s.oplogColl)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()
	s.assertTailer(c, tailer, 5, want)

	// Write more logs. These will be read from the the oplog.
	want2 := logTemplate{Message: "want 2"}
	s.writeLogsT(c, s.otherUUID, threshT.Add(6*time.Second), threshT.Add(10*time.Second), 5, want2)
	s.assertTailer(c, tailer, 5, want2)

}

func (s *LogTailerSuite) TestOplogTransition(c *tc.C) {
	// Ensure that logs aren't repeated as the log tailer moves from
	// reading from the logs collection to tailing the oplog.
	//
	// All logs are written out with the same timestamp to create a
	// challenging scenario for the tailer.

	for i := 0; i < 5; i++ {
		s.writeLogs(c, s.otherUUID, 1, logTemplate{Message: strconv.Itoa(i)})
	}

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{}, s.oplogColl)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()
	for i := 0; i < 5; i++ {
		s.assertTailer(c, tailer, 1, logTemplate{Message: strconv.Itoa(i)})
	}

	// Write more logs. These will be read from the the oplog.
	for i := 5; i < 10; i++ {
		lt := logTemplate{Message: strconv.Itoa(i)}
		s.writeLogs(c, s.otherUUID, 2, lt)
		s.assertTailer(c, tailer, 2, lt)
	}
}

func (s *LogTailerSuite) TestModelFiltering(c *tc.C) {
	good := logTemplate{Message: "good"}
	writeLogs := func() {
		s.writeLogs(c, "someuuid0", 1, logTemplate{
			Message: "bad",
		})
		s.writeLogs(c, "someuuid1", 1, logTemplate{
			Message: "bad",
		})
		s.writeLogs(c, s.otherUUID, 1, good)
	}

	assert := func(tailer corelogger.LogTailer) {
		// Only the entries the s.State's UUID should be reported.
		s.assertTailer(c, tailer, 1, good)
	}

	s.checkLogTailerFiltering(c, s.otherState, corelogger.LogTailerParams{}, writeLogs, assert)
}

func (s *LogTailerSuite) TestTailingLogsOnlyForOneModel(c *tc.C) {
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, logTemplate{
			Message: "bad"},
		)
		s.writeLogs(c, s.modelUUID, 1, logTemplate{
			Message: "good1",
		})
		s.writeLogs(c, s.modelUUID, 1, logTemplate{
			Message: "good2",
		})
	}

	assert := func(tailer corelogger.LogTailer) {
		messages := map[string]bool{}
		defer func() {
			c.Assert(messages, tc.HasLen, 2)
			for m := range messages {
				if m != "good1" && m != "good2" {
					c.Fatalf("received message: %v", m)
				}
			}
		}()
		count := 0
		for {
			select {
			case log := <-tailer.Logs():
				c.Assert(log.ModelUUID, tc.Equals, s.State.ModelUUID())
				messages[log.Message] = true
				count++
				c.Logf("count %d", count)
				if count >= 2 {
					return
				}
			case <-time.After(coretesting.ShortWait):
				c.Fatalf("timeout waiting for logs %d", count)
			}
		}
	}
	s.checkLogTailerFiltering(c, s.State, corelogger.LogTailerParams{}, writeLogs, assert)
}

func (s *LogTailerSuite) TestLevelFiltering(c *tc.C) {
	info := logTemplate{Level: loggo.INFO}
	error := logTemplate{Level: loggo.ERROR}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, logTemplate{Level: loggo.DEBUG})
		s.writeLogs(c, s.otherUUID, 1, info)
		s.writeLogs(c, s.otherUUID, 1, error)
	}
	params := corelogger.LogTailerParams{
		MinLevel: loggo.INFO,
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 1, info)
		s.assertTailer(c, tailer, 1, error)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestInitialLines(c *tc.C) {
	expected := logTemplate{Message: "want"}
	s.writeLogs(c, s.otherUUID, 3, logTemplate{Message: "dont want"})
	s.writeLogs(c, s.otherUUID, 5, expected)

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{InitialLines: 5}, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()

	// Should see just the last 5 lines as requested.
	s.assertTailer(c, tailer, 5, expected)
}

func (s *LogTailerSuite) TestRecordsAddedOutOfTimeOrder(c *tc.C) {
	format := "2006-01-02 03:04"
	t1, err := time.Parse(format, "2016-11-25 09:10")
	c.Assert(err, tc.ErrorIsNil)
	t2, err := time.Parse(format, "2016-11-25 09:20")
	c.Assert(err, tc.ErrorIsNil)
	here := logTemplate{Message: "logged here"}
	s.writeLogsT(c, s.otherUUID, t2, t2, 1, here)
	migrated := logTemplate{Message: "transferred by migration"}
	s.writeLogsT(c, s.otherUUID, t1, t1, 1, migrated)

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{}, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()

	// They still come back in the right time order.
	s.assertTailer(c, tailer, 1, migrated)
	s.assertTailer(c, tailer, 1, here)
}

func (s *LogTailerSuite) TestInitialLinesWithNotEnoughLines(c *tc.C) {
	expected := logTemplate{Message: "want"}
	s.writeLogs(c, s.otherUUID, 2, expected)

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{InitialLines: 5}, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()

	// Should see just the 2 lines that existed, even though 5 were
	// asked for.
	s.assertTailer(c, tailer, 2, expected)
}

func (s *LogTailerSuite) TestNoTail(c *tc.C) {
	expected := logTemplate{Message: "want"}
	s.writeLogs(c, s.otherUUID, 2, expected)

	// Write a log entry that's only in the oplog.
	doc := s.logTemplateToDoc(logTemplate{Message: "dont want"}, coretesting.ZeroTime())
	err := s.writeLogToOplog(s.otherUUID, doc)
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := state.NewLogTailer(s.otherState, corelogger.LogTailerParams{NoTail: true}, nil)
	c.Assert(err, tc.ErrorIsNil)
	// Not strictly necessary, just in case NoTail doesn't work in the test.
	defer tailer.Stop()

	// Logs only in the oplog shouldn't be reported and the tailer
	// should stop itself once the log collection has been read.
	s.assertTailer(c, tailer, 2, expected)
	select {
	case _, ok := <-tailer.Logs():
		if ok {
			c.Fatal("shouldn't be any further logs")
		}
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for logs channel to close")
	}

	select {
	case <-tailer.Dying():
		// Success.
	case <-time.After(coretesting.LongWait):
		c.Fatal("tailer didn't stop itself")
	}
}

func (s *LogTailerSuite) TestIncludeEntity(c *tc.C) {
	machine0 := logTemplate{Entity: "machine-0"}
	foo0 := logTemplate{Entity: "unit-foo-0"}
	foo1 := logTemplate{Entity: "unit-foo-1"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 3, machine0)
		s.writeLogs(c, s.otherUUID, 2, foo0)
		s.writeLogs(c, s.otherUUID, 1, foo1)
		s.writeLogs(c, s.otherUUID, 3, machine0)
	}
	params := corelogger.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo-0",
			"unit-foo-1",
		},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 2, foo0)
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeEntityWildcard(c *tc.C) {
	machine0 := logTemplate{Entity: "machine-0"}
	foo0 := logTemplate{Entity: "unit-foo-0"}
	foo1 := logTemplate{Entity: "unit-foo-1"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 3, machine0)
		s.writeLogs(c, s.otherUUID, 2, foo0)
		s.writeLogs(c, s.otherUUID, 1, foo1)
		s.writeLogs(c, s.otherUUID, 3, machine0)
	}
	params := corelogger.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo*",
		},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 2, foo0)
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeEntity(c *tc.C) {
	machine0 := logTemplate{Entity: "machine-0"}
	foo0 := logTemplate{Entity: "unit-foo-0"}
	foo1 := logTemplate{Entity: "unit-foo-1"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 3, machine0)
		s.writeLogs(c, s.otherUUID, 2, foo0)
		s.writeLogs(c, s.otherUUID, 1, foo1)
		s.writeLogs(c, s.otherUUID, 3, machine0)
	}
	params := corelogger.LogTailerParams{
		ExcludeEntity: []string{
			"machine-0",
			"unit-foo-0",
		},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeEntityWildcard(c *tc.C) {
	machine0 := logTemplate{Entity: "machine-0"}
	foo0 := logTemplate{Entity: "unit-foo-0"}
	foo1 := logTemplate{Entity: "unit-foo-1"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 3, machine0)
		s.writeLogs(c, s.otherUUID, 2, foo0)
		s.writeLogs(c, s.otherUUID, 1, foo1)
		s.writeLogs(c, s.otherUUID, 3, machine0)
	}
	params := corelogger.LogTailerParams{
		ExcludeEntity: []string{
			"machine*",
			"unit-*-0",
		},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeModule(c *tc.C) {
	mod0 := logTemplate{Module: "foo.bar"}
	mod1 := logTemplate{Module: "juju.thing"}
	subMod1 := logTemplate{Module: "juju.thing.hai"}
	mod2 := logTemplate{Module: "elsewhere"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, subMod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod2)
	}
	params := corelogger.LogTailerParams{
		IncludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 1, mod1)
		s.assertTailer(c, tailer, 1, subMod1)
		s.assertTailer(c, tailer, 1, mod2)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeModule(c *tc.C) {
	mod0 := logTemplate{Module: "foo.bar"}
	mod1 := logTemplate{Module: "juju.thing"}
	subMod1 := logTemplate{Module: "juju.thing.hai"}
	mod2 := logTemplate{Module: "elsewhere"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, subMod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod2)
	}
	params := corelogger.LogTailerParams{
		ExcludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 2, mod0)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeExcludeModule(c *tc.C) {
	foo := logTemplate{Module: "foo"}
	bar := logTemplate{Module: "bar"}
	barSub := logTemplate{Module: "bar.thing"}
	baz := logTemplate{Module: "baz"}
	qux := logTemplate{Module: "qux"}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, foo)
		s.writeLogs(c, s.otherUUID, 1, bar)
		s.writeLogs(c, s.otherUUID, 1, barSub)
		s.writeLogs(c, s.otherUUID, 1, baz)
		s.writeLogs(c, s.otherUUID, 1, qux)
	}
	params := corelogger.LogTailerParams{
		IncludeModule: []string{"foo", "bar", "qux"},
		ExcludeModule: []string{"foo", "bar"},
	}
	assert := func(tailer corelogger.LogTailer) {
		// Except just "qux" because "foo" and "bar" were included and
		// then excluded.
		s.assertTailer(c, tailer, 1, qux)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeLabels(c *tc.C) {
	mod0 := logTemplate{Labels: []string{"foo_bar"}}
	mod1 := logTemplate{Labels: []string{"juju_thing"}}
	subMod1 := logTemplate{Labels: []string{"juju_thing_hai"}}
	mod2 := logTemplate{Labels: []string{"elsewhere"}}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, subMod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod2)
	}
	params := corelogger.LogTailerParams{
		IncludeLabel: []string{"juju_thing", "elsewhere"},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 1, mod1)
		s.assertTailer(c, tailer, 1, mod2)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeLabels(c *tc.C) {
	mod0 := logTemplate{Labels: []string{"foo_bar"}}
	mod1 := logTemplate{Labels: []string{"juju_thing"}}
	subMod1 := logTemplate{Labels: []string{"juju_thing_hai"}}
	mod2 := logTemplate{Labels: []string{"elsewhere"}}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, subMod1)
		s.writeLogs(c, s.otherUUID, 1, mod0)
		s.writeLogs(c, s.otherUUID, 1, mod2)
	}
	params := corelogger.LogTailerParams{
		ExcludeLabel: []string{"juju_thing", "juju_thing_hai", "elsewhere"},
	}
	assert := func(tailer corelogger.LogTailer) {
		s.assertTailer(c, tailer, 2, mod0)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeExcludeLabels(c *tc.C) {
	foo := logTemplate{Labels: []string{"foo"}}
	bar := logTemplate{Labels: []string{"bar"}}
	barSub := logTemplate{Labels: []string{"bar_thing"}}
	baz := logTemplate{Labels: []string{"baz"}}
	qux := logTemplate{Labels: []string{"qux"}}
	writeLogs := func() {
		s.writeLogs(c, s.otherUUID, 1, foo)
		s.writeLogs(c, s.otherUUID, 1, bar)
		s.writeLogs(c, s.otherUUID, 1, barSub)
		s.writeLogs(c, s.otherUUID, 1, baz)
		s.writeLogs(c, s.otherUUID, 1, qux)
	}
	params := corelogger.LogTailerParams{
		IncludeLabel: []string{"foo", "bar", "qux"},
		ExcludeLabel: []string{"foo", "bar"},
	}
	assert := func(tailer corelogger.LogTailer) {
		// Except just "qux" because "foo" and "bar" were included and
		// then excluded.
		s.assertTailer(c, tailer, 1, qux)
	}
	s.checkLogTailerFiltering(c, s.otherState, params, writeLogs, assert)
}

func (s *LogTailerSuite) checkLogTailerFiltering(
	c *tc.C,
	st *state.State,
	params corelogger.LogTailerParams,
	writeLogs func(),
	assertTailer func(corelogger.LogTailer),
) {
	// Check the tailer does the right thing when reading from the
	// logs collection.
	writeLogs()
	tailer, err := state.NewLogTailer(st, params, s.oplogColl)
	c.Assert(err, tc.ErrorIsNil)
	defer tailer.Stop()
	assertTailer(tailer)

	// Now write out logs and check the tailer again. These will be
	// read from the oplog.
	writeLogs()
	assertTailer(tailer)
}

type logTemplate struct {
	Entity   string
	Version  version.Number
	Module   string
	Location string
	Level    loggo.Level
	Message  string
	Labels   []string
}

// writeLogs creates count log messages at the current time using
// the supplied template. As well as writing to the logs collection,
// entries are also made into the fake oplog collection.
func (s *LogTailerSuite) writeLogs(c *tc.C, modelUUID string, count int, lt logTemplate) {
	t := coretesting.ZeroTime()
	s.writeLogsT(c, modelUUID, t, t, count, lt)
}

// writeLogsT creates count log messages between startTime and
// endTime using the supplied template. As well as writing to the logs
// collection, entries are also made into the fake oplog collection.
func (s *LogTailerSuite) writeLogsT(c *tc.C, modelUUID string, startTime, endTime time.Time, count int, lt logTemplate) {
	interval := endTime.Sub(startTime) / time.Duration(count)
	t := startTime
	for i := 0; i < count; i++ {
		doc := s.logTemplateToDoc(lt, t)
		err := s.writeLogToOplog(modelUUID, doc)
		c.Assert(err, tc.ErrorIsNil)
		err = s.getCollection(modelUUID).Insert(doc)
		c.Assert(err, tc.ErrorIsNil)
		t = t.Add(interval)
	}
}

// writeLogToOplog writes out a log record to the a (probably fake)
// oplog collection.
func (s *LogTailerSuite) writeLogToOplog(modelUUID string, doc interface{}) error {
	return s.oplogColl.Insert(bson.D{
		{"ts", bson.MongoTimestamp(coretesting.ZeroTime().Unix() << 32)}, // an approximation which will do
		{"h", rand.Int63()}, // again, a suitable fake
		{"op", "i"},         // this will always be an insert
		{"ns", "logs.logs." + modelUUID},
		{"o", doc},
	})
}

// deleteLogOplogEntry writes out a log record to the a (probably fake)
// oplog collection.
func (s *LogTailerSuite) deleteLogOplogEntry(modelUUID string, doc interface{}) error {
	return s.oplogColl.Insert(bson.D{
		{"ts", bson.MongoTimestamp(coretesting.ZeroTime().Unix() << 32)}, // an approximation which will do
		{"h", rand.Int63()}, // again, a suitable fake
		{"op", "d"},
		{"ns", "logs.logs." + modelUUID},
		{"o", doc},
	})
}

func (s *LogTailerSuite) normaliseLogTemplate(lt *logTemplate) {
	if lt.Entity == "" {
		lt.Entity = "not-a-tag"
	}
	if lt.Version == version.Zero {
		lt.Version = jujuversion.Current
	}
	if lt.Module == "" {
		lt.Module = "module"
	}
	if lt.Location == "" {
		lt.Location = "loc"
	}
	if lt.Level == loggo.UNSPECIFIED {
		lt.Level = loggo.INFO
	}
	if lt.Message == "" {
		lt.Message = "message"
	}
}

func (s *LogTailerSuite) logTemplateToDoc(lt logTemplate, t time.Time) interface{} {
	s.normaliseLogTemplate(&lt)
	return state.MakeLogDoc(
		lt.Entity,
		t,
		lt.Module,
		lt.Location,
		lt.Level,
		lt.Message,
		lt.Labels,
	)
}

func (s *LogTailerSuite) assertTailer(c *tc.C, tailer corelogger.LogTailer, expectedCount int, lt logTemplate) {
	s.normaliseLogTemplate(&lt)

	timeout := time.After(coretesting.LongWait)
	count := 0
	for {
		select {
		case log, ok := <-tailer.Logs():
			if !ok {
				c.Fatalf("tailer died unexpectedly: %v", tailer.Err())
			}

			c.Assert(log.Version, tc.Equals, lt.Version)
			c.Assert(log.Entity, tc.Equals, lt.Entity)
			c.Assert(log.Module, tc.Equals, lt.Module)
			c.Assert(log.Location, tc.Equals, lt.Location)
			c.Assert(log.Level, tc.Equals, lt.Level)
			c.Assert(log.Message, tc.Equals, lt.Message)
			c.Assert(log.Labels, tc.DeepEquals, lt.Labels)
			count++
			if count == expectedCount {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for logs (received %d)", count)
		}
	}
}

type DBLogSizeSuite struct {
	coretesting.BaseSuite
}

func TestDBLogSizeSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &DBLogSizeSuite{})
}

func (*DBLogSizeSuite) TestDBLogSizeIntSize(c *tc.C) {
	res, err := state.DBCollectionSizeToInt(bson.M{"size": int(12345)}, "coll-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.Equals, int(12345))
}

func (*DBLogSizeSuite) TestDBLogSizeNoSize(c *tc.C) {
	res, err := state.DBCollectionSizeToInt(bson.M{}, "coll-name")
	// Old code didn't treat this as an error, if we know it doesn't happen often, we could start changing it to be an error.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.Equals, int(0))
}

func (*DBLogSizeSuite) TestDBLogSizeInt64Size(c *tc.C) {
	// Production results have shown that sometimes collStats can return an int64.
	// See https://bugs.launchpad.net/juju/+bug/1790626 in case we ever figure out why
	res, err := state.DBCollectionSizeToInt(bson.M{"size": int64(12345)}, "coll-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.Equals, int(12345))
}

func (*DBLogSizeSuite) TestDBLogSizeInt64SizeOverflow(c *tc.C) {
	// Just in case, it is unlikely this ever actually happens
	res, err := state.DBCollectionSizeToInt(bson.M{"size": int64(12345678901)}, "coll-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.Equals, int((1<<31)-1))
}

func (*DBLogSizeSuite) TestDBLogSizeNegativeSize(c *tc.C) {
	_, err := state.DBCollectionSizeToInt(bson.M{"size": int(-10)}, "coll-name")
	c.Check(err, tc.ErrorMatches, `mongo collStats for "coll-name" returned a negative value: -10`)
	_, err = state.DBCollectionSizeToInt(bson.M{"size": int64(-10)}, "coll-name")
	c.Check(err, tc.ErrorMatches, `mongo collStats for "coll-name" returned a negative value: -10`)
}

func (*DBLogSizeSuite) TestDBLogSizeUnknownType(c *tc.C) {
	_, err := state.DBCollectionSizeToInt(bson.M{"size": float64(12345)}, "coll-name")
	c.Check(err, tc.ErrorMatches, `mongo collStats for "coll-name" did not return an int or int64 for size, returned float64: 12345`)
}
