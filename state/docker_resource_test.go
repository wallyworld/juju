// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/state"
)

type dockerMetadataStorageSuite struct {
	ConnSuite
	metadataStorage state.DockerMetadataStorage
}

func TestDockerMetadataStorageSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &dockerMetadataStorageSuite{})
}

func (s *dockerMetadataStorageSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.metadataStorage = state.NewDockerMetadataStorage(s.State)
}

func (s *dockerMetadataStorageSuite) Test(c *tc.C) {}

func (s *dockerMetadataStorageSuite) TestSaveNewResource(c *tc.C) {
	id := "test-123"
	registryPath := "url@sha256:abc123"
	resource := resources.DockerImageDetails{
		RegistryPath: registryPath,
	}
	err := s.metadataStorage.Save(id, resource)

	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource)
}

func (s *dockerMetadataStorageSuite) TestSaveUpdatesExistingResource(c *tc.C) {
	id := "test-123"
	resource1 := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource1)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource1)

	resource2 := resources.DockerImageDetails{
		RegistryPath: "url@sha256:deadbeef",
	}
	err = s.metadataStorage.Save(id, resource2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource2)
}

func (s *dockerMetadataStorageSuite) TestSaveIdempotent(c *tc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, tc.ErrorIsNil)
	err = s.metadataStorage.Save(id, resource)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource)
}

func (s *dockerMetadataStorageSuite) assertSavedDockerResource(c *tc.C, resourceID string, registryInfo resources.DockerImageDetails) {
	coll, closer := state.GetCollection(s.State, "dockerResources")
	defer closer()

	var raw bson.M
	err := coll.FindId(resourceID).One(&raw)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(raw["_id"], tc.Equals, fmt.Sprintf("%s:%s", s.State.ModelUUID(), resourceID))
	c.Assert(raw["registry-path"], tc.Equals, registryInfo.RegistryPath)
	c.Assert(raw["password"], tc.Equals, registryInfo.Password)
	c.Assert(raw["username"], tc.Equals, registryInfo.Username)
}

func (s *dockerMetadataStorageSuite) TestGet(c *tc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "testuser",
				Password: "hunter2",
			},
		},
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, tc.ErrorIsNil)

	retrieved, num, err := s.metadataStorage.Get(id)
	c.Assert(err, tc.ErrorIsNil)
	retrievedInfo := readerToDockerDetails(c, retrieved)
	c.Assert(num, tc.Equals, int64(76))
	c.Assert(retrievedInfo.RegistryPath, tc.Equals, "url@sha256:abc123")
	c.Assert(retrievedInfo.Username, tc.Equals, "testuser")
	c.Assert(retrievedInfo.Password, tc.Equals, "hunter2")

}

func (s *dockerMetadataStorageSuite) TestRemove(c *tc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, tc.ErrorIsNil)

	err = s.metadataStorage.Remove(id)
	c.Assert(err, tc.ErrorIsNil)
	_, _, err = s.metadataStorage.Get(id)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func readerToDockerDetails(c *tc.C, r io.ReadCloser) *resources.DockerImageDetails {
	var info resources.DockerImageDetails
	respBuf := new(bytes.Buffer)
	_, err := respBuf.ReadFrom(r)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal(respBuf.Bytes(), &info)
	c.Assert(err, tc.ErrorIsNil)
	return &info
}
