// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"errors"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujustorage "github.com/juju/juju/storage"
)

type ImportFilesystemSuite struct {
	SubStorageSuite
	importer mockStorageImporter
}

func TestImportFilesystemSuite(t *tctesting.T) {
	tc.Run(t, &ImportFilesystemSuite{})
}

func (s *ImportFilesystemSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)
	s.importer = mockStorageImporter{}
}

var initErrorTests = []struct {
	args        []string
	expectedErr string
}{{
	args:        []string{"foo", "bar"},
	expectedErr: "import-filesystem requires a storage provider, provider ID, and storage name",
}, {
	args:        []string{"123", "foo", "bar"},
	expectedErr: `pool name "123" not valid`,
}, {
	args:        []string{"foo", "abc123", "123"},
	expectedErr: `"123" is not a valid storage name`,
}}

func (s *ImportFilesystemSuite) TestInitErrors(c *tc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d for %q", i, t.args)
		_, err := s.run(c, t.args...)
		c.Assert(err, tc.ErrorMatches, t.expectedErr)
	}
}

func (s *ImportFilesystemSuite) TestImportSuccess(c *tc.C) {
	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
importing "bar" from storage pool "foo" as storage "baz"
imported storage baz/0
`[1:])

	s.importer.CheckCalls(c, []testhelpers.StubCall{
		{"ImportStorage", []interface{}{
			jujustorage.StorageKindFilesystem,
			"foo", "bar", "baz", false,
		}},
		{"Close", nil},
	})
}

func (s *ImportFilesystemSuite) TestImportError(c *tc.C) {
	s.importer.SetErrors(errors.New("nope"))

	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorMatches, "nope")

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `importing "bar" from storage pool "foo" as storage "baz"`+"\n")
}

func (s *ImportFilesystemSuite) TestImportSuccessCAAS(c *tc.C) {
	s.SetFeatureFlags(feature.K8SAttachStorage)

	store := jujuclienttesting.MinimalStore()
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	s.store = store

	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
importing "bar" from storage pool "foo" as storage "baz"
imported storage baz/0
`[1:])

	s.importer.CheckCalls(c, []testhelpers.StubCall{
		{"ImportStorage", []interface{}{
			jujustorage.StorageKindFilesystem,
			"foo", "bar", "baz", false,
		}},
		{"Close", nil},
	})
}

func (s *ImportFilesystemSuite) TestImportErrorCAASNotSupport(c *tc.C) {
	store := jujuclienttesting.MinimalStore()
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	s.store = store

	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorMatches, "Juju command \"import-filesystem\" not supported on container models")

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *ImportFilesystemSuite) TestImportWithForce(c *tc.C) {
	ctx, err := s.run(c, "--force", "foo", "bar", "baz")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
importing "bar" from storage pool "foo" as storage "baz"
imported storage baz/0
`[1:])

	s.importer.CheckCalls(c, []testhelpers.StubCall{
		{"ImportStorage", []interface{}{
			jujustorage.StorageKindFilesystem,
			"foo", "bar", "baz", true,
		}},
		{"Close", nil},
	})
}

func (s *ImportFilesystemSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewImportFilesystemCommand(
		func(*storage.StorageCommandBase) (storage.StorageImporter, error) {
			return &s.importer, nil
		},
		s.store,
	), args...)
}

type mockStorageImporter struct {
	testhelpers.Stub
}

func (m *mockStorageImporter) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

func (m *mockStorageImporter) ImportStorage(
	k jujustorage.StorageKind,
	pool, providerId, storageName string,
	force bool,
) (names.StorageTag, error) {
	m.MethodCall(m, "ImportStorage", k, pool, providerId, storageName, force)
	return names.NewStorageTag(storageName + "/0"), m.NextErr()
}
