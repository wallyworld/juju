// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	tctesting "testing"

	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/storage/provider"
)

type upgradesSuite struct {
	internalStateSuite
}

func TestUpgradesSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &upgradesSuite{})
}

type expectUpgradedData struct {
	coll     *mgo.Collection
	expected []bson.M
	filter   bson.D
}

func upgradedData(coll *mgo.Collection, expected []bson.M) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
	}
}

func (s *upgradesSuite) assertUpgradedData(c *tc.C, upgrade func(*StatePool) error, check tc.Checker, expect ...expectUpgradedData) {
	if check == nil {
		check = tc.DeepEquals
	}
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := upgrade(s.pool)
		c.Assert(err, tc.ErrorIsNil)

		for _, expect := range expect {
			var docs []bson.M
			err = expect.coll.Find(expect.filter).Sort("_id").All(&docs)
			c.Assert(err, tc.ErrorIsNil)
			for i, d := range docs {
				doc := d
				delete(doc, "txn-queue")
				delete(doc, "txn-revno")
				delete(doc, "version")
				docs[i] = doc
			}
			c.Assert(docs, check, expect.expected,
				tc.Commentf("differences: %s", pretty.Diff(docs, expect.expected)))
		}
	}
}

func (s *upgradesSuite) makeModel(c *tc.C, name string, attr coretesting.Attrs, modelArgs ModelArgs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, tc.ErrorIsNil)
	_, st, err := s.controller.NewModel(
		defaultModelArgs(&modelArgs, cfg, m.Owner()))
	c.Assert(err, tc.ErrorIsNil)
	return st
}

func defaultModelArgs(modelArgs *ModelArgs, cfg *config.Config, owner names.UserTag) ModelArgs {
	if modelArgs == nil {
		modelArgs = &ModelArgs{}
	}
	modelArgs.Config = cfg
	modelArgs.Owner = owner

	if modelArgs.Type == "" {
		modelArgs.Type = ModelTypeIAAS
	}
	if modelArgs.CloudName == "" {
		modelArgs.CloudName = "dummy"
	}
	if modelArgs.CloudRegion == "" {
		modelArgs.CloudRegion = "dummy-region"
	}
	if modelArgs.StorageProviderRegistry == nil {
		modelArgs.StorageProviderRegistry = provider.CommonStorageProviders()
	}

	return *modelArgs
}

// TestUpgradeAddVirtualHostKeys tests that after an upgrade,
// machines and CAAS units have a virtual host key.
func (s *upgradesSuite) TestUpgradeAddVirtualHostKeys(c *tc.C) {
	machineModel := s.makeModel(c, "model-1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	k8sModel := s.makeModel(c, "model-2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	defer func() {
		_ = machineModel.Close()
		_ = k8sModel.Close()
	}()

	machinesColl, machinesCloser := s.state.db().GetRawCollection(machinesC)
	defer machinesCloser()

	err := machinesColl.Insert(bson.M{
		"_id":        ensureModelUUID(machineModel.ModelUUID(), "1"),
		"machineid":  "1",
		"model-uuid": machineModel.ModelUUID(),
	})
	c.Assert(err, tc.ErrorIsNil)

	unitsColl, unitsCloser := s.state.db().GetRawCollection(unitsC)
	defer unitsCloser()

	// The first unit is on a machine model and the second on a k8s model.
	// The first unit is not expected to have a key while the second is.
	err = unitsColl.Insert(
		bson.M{
			"_id":        ensureModelUUID(machineModel.ModelUUID(), "machineunit/1"),
			"name":       "machineunit/1",
			"model-uuid": machineModel.ModelUUID(),
			"machineid":  "1",
		}, bson.M{
			"_id":        ensureModelUUID(k8sModel.ModelUUID(), "k8sunit/1"),
			"name":       "k8sunit/1",
			"model-uuid": k8sModel.ModelUUID(),
		})
	c.Assert(err, tc.ErrorIsNil)

	virtualHostKeysColl, vhkCloser := s.state.db().GetRawCollection(virtualHostKeysC)
	defer vhkCloser()

	// The hostkey values below are ignored by the checker but must still exist for deepEquals to work.
	expectedVirtualHostKeys := []bson.M{
		{
			"_id":     fmt.Sprintf("%s:machine-1-hostkey", machineModel.ModelUUID()),
			"hostkey": []byte("placeholder"),
		}, {
			"_id":     fmt.Sprintf("%s:unit-k8sunit/1-hostkey", k8sModel.ModelUUID()),
			"hostkey": []byte("placeholder"),
		}}

	// Sort the values since the model UUIDs are random and assertUpgradedData fetches
	// the actual data in sorted order.
	sort.Slice(expectedVirtualHostKeys, func(i, j int) bool {
		return expectedVirtualHostKeys[i]["_id"].(string) < expectedVirtualHostKeys[j]["_id"].(string)
	})

	// XXXX
	mc := tc.NewMultiChecker()
	//mc.AddExpr(`_[_]["hostkey"]`, testing.BytesToStringMatch, `-----BEGIN OPENSSH PRIVATE KEY-----.*`)
	s.assertUpgradedData(c, AddVirtualHostKeys, mc,
		upgradedData(virtualHostKeysColl, expectedVirtualHostKeys),
	)
}

func (s *upgradesSuite) TestSplitMigrationStatusMessages(c *tc.C) {
	model := s.makeModel(c, "m", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	defer func() { _ = model.Close() }()

	migStatus, closer := s.state.db().GetRawCollection(migrationsStatusC)
	defer closer()

	migStatusMessage, closer2 := s.state.db().GetRawCollection(migrationsStatusMessageC)
	defer closer2()

	err := migStatus.Insert(bson.M{
		"_id":                ensureModelUUID(model.ModelUUID(), "0"),
		"start-time":         "1742996705546941797",
		"success-time":       "1742996716038789910",
		"end-time":           "1742996722262468965",
		"phase":              "DONE",
		"phase-changed-time": "1742996722262468965",
		"status-message":     "successful, removing model from source controller",
	})
	c.Assert(err, tc.ErrorIsNil)

	expectedStatus := []bson.M{{
		"_id":                ensureModelUUID(model.ModelUUID(), "0"),
		"start-time":         "1742996705546941797",
		"success-time":       "1742996716038789910",
		"end-time":           "1742996722262468965",
		"phase":              "DONE",
		"phase-changed-time": "1742996722262468965",
	}}
	expectedStatusMessage := []bson.M{{
		"_id":            ensureModelUUID(model.ModelUUID(), "0"),
		"status-message": "successful, removing model from source controller",
	}}

	s.assertUpgradedData(c, SplitMigrationStatusMessages, nil,
		upgradedData(migStatus, expectedStatus),
		upgradedData(migStatusMessage, expectedStatusMessage),
	)
}
