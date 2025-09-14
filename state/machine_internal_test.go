// Copyright Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
)

type MachineInternalSuite struct {
	testhelpers.IsolationSuite
}

func (s *MachineInternalSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func TestMachineInternalSuite(t *tctesting.T) {
	tc.Run(t, &MachineInternalSuite{})
}

func (s *MachineInternalSuite) TestCreateUpgradeLockTxnAssertsMachineAlive(c *tc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLockDoc{}
	var found bool
	for _, op := range createUpgradeSeriesLockTxnOps(arbitraryId, arbitraryData) {
		assertVal, ok := op.Assert.(bson.D)
		if op.C == machinesC && ok && assertVal.Map()["life"] == Alive {
			found = true
			break
		}
	}
	c.Assert(found, tc.IsTrue, tc.Commentf("Transaction does not assert that machines are of status Alive"))
}

func (s *MachineInternalSuite) TestCreateUpgradeLockTxnAssertsDocDoesNOTExist(c *tc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLockDoc{}
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocMissing,
		Insert: arbitraryData,
	}
	assertContainsOP(c, expectedOp, createUpgradeSeriesLockTxnOps(arbitraryId, arbitraryData))
}

func (s *MachineInternalSuite) TestRemoveUpgradeLockTxnAssertsDocExists(c *tc.C) {
	arbitraryId := "1"
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocExists,
		Remove: true,
	}
	assertContainsOP(c, expectedOp, removeUpgradeSeriesLockTxnOps(arbitraryId))
}

func (s *MachineInternalSuite) TestSetUpgradeSeriesTxnOpsBuildsCorrectUnitTransaction(c *tc.C) {
	arbitraryMachineID := "id"
	arbitraryUnitName := "application/0"
	arbitraryStatus := model.UpgradeSeriesPrepareStarted
	arbitraryUpdateTime := bson.Now()
	arbitraryMessage := "some message"
	arbitraryUpgradeSeriesMessage := newUpgradeSeriesMessage(arbitraryUnitName, arbitraryMessage, arbitraryUpdateTime)
	expectedOp := txn.Op{
		C:  machineUpgradeSeriesLocksC,
		Id: arbitraryMachineID,
		Assert: bson.D{{"$and", []bson.D{
			{{"unit-statuses", bson.D{{"$exists", true}}}},
			{{"unit-statuses.application/0.status", bson.D{{"$ne", arbitraryStatus}}}}}}},
		Update: bson.D{
			{"$set", bson.D{
				{"unit-statuses.application/0.status", arbitraryStatus},
				{"timestamp", arbitraryUpdateTime},
				{"unit-statuses.application/0.timestamp", arbitraryUpdateTime}}},
			{"$push", bson.D{{"messages", arbitraryUpgradeSeriesMessage}}},
		},
	}
	actualOps, err := setUpgradeSeriesTxnOps(arbitraryMachineID, arbitraryUnitName, arbitraryStatus, arbitraryUpdateTime, arbitraryUpgradeSeriesMessage)
	c.Assert(err, tc.ErrorIsNil)
	expectedOpSt := fmt.Sprint(expectedOp.Update)
	actualOpSt := fmt.Sprint(actualOps[1].Update)
	c.Assert(actualOpSt, tc.Equals, expectedOpSt)
}

func (s *MachineInternalSuite) TestSetUpgradeSeriesTxnOpsShouldAssertAssignedMachineIsAlive(c *tc.C) {
	arbitraryMachineID := "id"
	arbitraryStatus := model.UpgradeSeriesPrepareStarted
	arbitraryUnitName := "application/0"
	arbitraryUpdateTime := bson.Now()
	arbitraryMessage := "message"
	arbitraryUpgradeSeriesMessage := newUpgradeSeriesMessage(arbitraryUnitName, arbitraryMessage, arbitraryUpdateTime)
	expectedOp := txn.Op{
		C:      machinesC,
		Id:     arbitraryMachineID,
		Assert: isAliveDoc,
	}
	actualOps, err := setUpgradeSeriesTxnOps(arbitraryMachineID, arbitraryUnitName, arbitraryStatus, arbitraryUpdateTime, arbitraryUpgradeSeriesMessage)
	c.Assert(err, tc.ErrorIsNil)
	expectedOpSt := fmt.Sprint(expectedOp)
	actualOpSt := fmt.Sprint(actualOps[0])
	c.Assert(actualOpSt, tc.Equals, expectedOpSt)
}

func (s *MachineInternalSuite) TestStartUpgradeSeriesUnitCompletionTxnOps(c *tc.C) {
	arbitraryMachineID := "id"
	arbitraryTimestamp := bson.Now()
	arbitraryLock := &upgradeSeriesLockDoc{TimeStamp: arbitraryTimestamp}
	expectedOps := []txn.Op{
		{
			C:      machinesC,
			Id:     arbitraryMachineID,
			Assert: isAliveDoc,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     arbitraryMachineID,
			Assert: bson.D{{"machine-status", model.UpgradeSeriesCompleteStarted}},
			Update: bson.D{{"$set", bson.D{
				{"unit-statuses", map[string]UpgradeSeriesUnitStatus{}},
				{"timestamp", arbitraryTimestamp},
				{"messages", []UpgradeSeriesMessage{}}}}},
		},
	}
	actualOps := startUpgradeSeriesUnitCompletionTxnOps(arbitraryMachineID, arbitraryLock)
	expectedOpsSt := fmt.Sprint(expectedOps)
	actualOpsSt := fmt.Sprint(actualOps)
	c.Assert(actualOpsSt, tc.Equals, expectedOpsSt)
}

func (s *MachineInternalSuite) TestSetUpgradeSeriesMessageTxnOps(c *tc.C) {
	arbitraryMachineID := "id"
	arbitraryTimestamp := bson.Now()
	arbitraryMessages := []UpgradeSeriesMessage{
		{
			Message:   "arbitraryMessages0",
			Timestamp: arbitraryTimestamp,
			Seen:      false,
		},
		{
			Message:   "arbitraryMessages1",
			Timestamp: arbitraryTimestamp,
			Seen:      false,
		},
	}
	expectedOps := []txn.Op{
		{
			C:      machinesC,
			Id:     arbitraryMachineID,
			Assert: isAliveDoc,
		},
		{
			C:  machineUpgradeSeriesLocksC,
			Id: arbitraryMachineID,
			Update: bson.D{{"$set", bson.D{
				{"messages.0.seen", true},
				{"messages.1.seen", true},
			}}},
		},
	}
	actualOps := setUpgradeSeriesMessageTxnOps(arbitraryMachineID, arbitraryMessages, true)
	expectedOpsSt := fmt.Sprint(expectedOps)
	actualOpsSt := fmt.Sprint(actualOps)
	c.Assert(actualOpsSt, tc.Equals, expectedOpsSt)
}

func assertContainsOP(c *tc.C, expectedOp txn.Op, actualOps []txn.Op) {
	var found bool
	for _, actualOp := range actualOps {
		if actualOp == expectedOp {
			found = true
			break
		}
	}
	c.Assert(found, tc.IsTrue, tc.Commentf("expected %#v to contain %#v", actualOps, expectedOp))
}
