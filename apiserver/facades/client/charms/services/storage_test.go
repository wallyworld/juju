// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services_test

import (
	"errors"
	"fmt"
	"strings"
	tctesting "testing"

	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/apiserver/facades/client/charms/services/mocks"
	"github.com/juju/juju/core/charm/downloader"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

func TestStorageTestSuite(t *tctesting.T) {
	tc.Run(t, &storageTestSuite{})
}

type storageTestSuite struct {
	testhelpers.IsolationSuite

	stateBackend   *mocks.MockStateBackend
	uploadedCharm  *mocks.MockUploadedCharm
	storageBackend *mocks.MockStorage
	storage        *services.CharmStorage
	uuid           utils.UUID
}

func (s *storageTestSuite) TestPrepareToStoreNotYetUploadedCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(false)

	err := s.storage.PrepareToStoreCharm(curl)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageTestSuite) TestPrepareToStoreAlreadyUploadedCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"

	s.stateBackend.EXPECT().PrepareCharmUpload(curl).Return(s.uploadedCharm, nil)
	s.uploadedCharm.EXPECT().IsUploaded().Return(true)

	err := s.storage.PrepareToStoreCharm(curl)

	expErr := downloader.NewCharmAlreadyStoredError(curl)
	c.Assert(err, tc.Equals, expErr)
}

func (s *storageTestSuite) TestStoreBlobFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData: strings.NewReader("the-blob"),
		Size:      7337,
	}

	s.stateBackend.EXPECT().ModelUUID().Return("the-model-uuid")
	s.storageBackend.EXPECT().Put(expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(errors.New("failed"))

	err := s.storage.Store(curl, dlCharm)
	c.Assert(err, tc.ErrorMatches, "cannot add charm to storage.*")
}

func (s *storageTestSuite) TestStoreBlobAlreadyStored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl := "ch:ubuntu-lite"
	expStoreCharmPath := fmt.Sprintf("charms/%s-%s", curl, s.uuid)
	dlCharm := downloader.DownloadedCharm{
		CharmData:    strings.NewReader("the-blob"),
		Size:         7337,
		SHA256:       "d357",
		CharmVersion: "the-version",
	}

	s.stateBackend.EXPECT().ModelUUID().Return("the-model-uuid")
	s.storageBackend.EXPECT().Put(expStoreCharmPath, gomock.AssignableToTypeOf(dlCharm.CharmData), int64(7337)).Return(nil)
	s.stateBackend.EXPECT().UpdateUploadedCharm(state.CharmInfo{
		StoragePath: expStoreCharmPath,
		ID:          curl,
		SHA256:      "d357",
		Version:     "the-version",
	}).Return(nil, stateerrors.NewErrCharmAlreadyUploaded(curl))

	// As the blob is already uploaded (to another path), we need to remove
	// the duplicate we just uploaded from the store.
	s.storageBackend.EXPECT().Remove(expStoreCharmPath).Return(nil)

	err := s.storage.Store(curl, dlCharm)
	c.Assert(err, tc.ErrorIsNil) // charm already uploaded by someone; no error
}

func (s *storageTestSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateBackend = mocks.NewMockStateBackend(ctrl)
	s.uploadedCharm = mocks.NewMockUploadedCharm(ctrl)
	s.storageBackend = mocks.NewMockStorage(ctrl)

	var err error
	s.uuid, err = utils.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.storage = services.NewCharmStorage(services.CharmStorageConfig{
		Logger:       loggo.GetLogger("test"),
		StateBackend: s.stateBackend,
		StorageFactory: func(_ string) services.Storage {
			return s.storageBackend
		},
	})
	s.storage.SetUUIDGenerator(func() (utils.UUID, error) {
		return s.uuid, nil
	})

	return ctrl
}
