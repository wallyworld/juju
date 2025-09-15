// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"io"
	"strings"
	tctesting "testing"

	"github.com/juju/blobstore/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
)

type tooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(v version.Binary) error
	Refresh() error
}

func testAgentTools(c *tc.C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(t, tc.IsNil)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = obj.SetAgentVersion(version.Binary{})
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("cannot set agent version for %s: empty series or arch", agent))

	v2 := version.MustParseBinary("7.8.9-ubuntu-amd64")
	err = obj.SetAgentVersion(v2)
	c.Assert(err, tc.ErrorIsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(t3.Version, tc.DeepEquals, v2)
	err = obj.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	t3, err = obj.AgentTools()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(t3.Version, tc.DeepEquals, v2)

	if le, ok := obj.(lifer); ok {
		testWhenDying(c, le, noErr, deadErr, func() error {
			return obj.SetAgentVersion(v2)
		})
	}
}

type binaryStorageSuite struct {
	ConnSuite

	controllerModelUUID string
	modelUUID           string
	st                  *state.State
}

func TestBinaryStorageSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &binaryStorageSuite{})
}

func (s *binaryStorageSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)

	s.controllerModelUUID = s.State.ControllerModelUUID()

	// Create a new model and store its UUID.
	s.modelUUID = utils.MustNewUUID().String()
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "new-model",
		"uuid": s.modelUUID,
	})
	var err error
	_, s.st, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   names.NewLocalUserTag("test-admin"),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) {
		s.st.Close()
	})
}

type storageOpener func() (binarystorage.StorageCloser, error)

func (s *binaryStorageSuite) TestToolsStorage(c *tc.C) {
	s.testStorage(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsControllerModel(c *tc.C) {
	s.testStorageParams(c, "toolsmetadata", []string{s.State.ModelUUID()}, s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsHostedModel(c *tc.C) {
	s.testStorageParams(c, "toolsmetadata", []string{s.modelUUID, s.State.ModelUUID()}, s.st.ToolsStorage)
}

func (s *binaryStorageSuite) testStorage(c *tc.C, collName string, openStorage storageOpener) {
	session := s.State.MongoSession()
	// if the collection didn't exist, we will create it on demand.
	err := session.DB("juju").C(collName).DropCollection()
	c.Assert(err, tc.ErrorIsNil)
	collectionNames, err := session.DB("juju").CollectionNames()
	c.Assert(err, tc.ErrorIsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), tc.IsFalse)

	storage, err := openStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := storage.Close()
		c.Assert(err, tc.ErrorIsNil)
	}()

	err = storage.Add(strings.NewReader(""), binarystorage.Metadata{})
	c.Assert(err, tc.ErrorIsNil)

	collectionNames, err = session.DB("juju").CollectionNames()
	c.Assert(err, tc.ErrorIsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), tc.IsTrue)
}

func (s *binaryStorageSuite) testStorageParams(c *tc.C, collName string, uuids []string, openStorage storageOpener) {
	var uuidArgs []string
	s.PatchValue(state.BinarystorageNew, func(
		modelUUID string,
		managedStorage blobstore.ManagedStorage,
		metadataCollection mongo.Collection,
		runner jujutxn.Runner,
	) binarystorage.Storage {
		uuidArgs = append(uuidArgs, modelUUID)
		c.Assert(managedStorage, tc.NotNil)
		c.Assert(metadataCollection.Name(), tc.Equals, collName)
		c.Assert(runner, tc.NotNil)
		return nil
	})

	storage, err := openStorage()
	c.Assert(err, tc.ErrorIsNil)
	storage.Close()
	c.Assert(uuidArgs, tc.DeepEquals, uuids)
}

func (s *binaryStorageSuite) TestToolsStorageLayered(c *tc.C) {
	modelTools, err := s.st.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer modelTools.Close()

	controllerTools, err := s.State.ToolsStorage()
	c.Assert(err, tc.ErrorIsNil)
	defer controllerTools.Close()

	err = modelTools.Add(strings.NewReader("abc"), binarystorage.Metadata{Version: "1.0", Size: 3})
	c.Assert(err, tc.ErrorIsNil)
	err = controllerTools.Add(strings.NewReader("defg"), binarystorage.Metadata{Version: "1.0", Size: 4})
	c.Assert(err, tc.ErrorIsNil)
	err = controllerTools.Add(strings.NewReader("def"), binarystorage.Metadata{Version: "2.0", Size: 3})
	c.Assert(err, tc.ErrorIsNil)

	all, err := modelTools.AllMetadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all, tc.DeepEquals, []binarystorage.Metadata{
		{Version: "1.0", Size: 3},
		{Version: "2.0", Size: 3},
	})

	assertContents := func(v, contents string) {
		_, rc, err := modelTools.Open(v)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rc, tc.NotNil)
		defer rc.Close()
		data, err := io.ReadAll(rc)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(data), tc.Equals, contents)
	}
	assertContents("1.0", "abc")
	assertContents("2.0", "def")
}
