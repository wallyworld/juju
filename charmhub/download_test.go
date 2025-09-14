// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/testcharms/repo"
)

const defaultSeries = "bionic"
const localCharmRepo = "../testcharms/charm-repo"

type DownloadSuite struct {
	testhelpers.IsolationSuite
}

func TestDownloadSuite(t *tctesting.T) {
	tc.Run(t, &DownloadSuite{})
}

func (s *DownloadSuite) TestDownloadAndRead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		archiveBytes := s.createCharmArchieve(c)

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(archiveBytes)),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DownloadSuite) TestDownloadAndReadWithNotFoundStatusCode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": archive not found`)
}

func (s *DownloadSuite) TestDownloadAndReadWithFailedStatusCode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, err := os.CreateTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}()

	fileSystem := NewMockFileSystem(ctrl)
	fileSystem.EXPECT().Create(tmpFile.Name()).Return(tmpFile, nil)

	httpClient := NewMockHTTPClient(ctrl)
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(http.StatusInternalServerError),
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("")),
		}, nil
	})

	serverURL, err := url.Parse("http://meshuggah.rocks")
	c.Assert(err, tc.ErrorIsNil)

	client := newDownloadClient(httpClient, fileSystem, &FakeLogger{})
	_, err = client.DownloadAndRead(c.Context(), serverURL, tmpFile.Name())
	c.Assert(err, tc.ErrorMatches, `cannot retrieve "http://meshuggah.rocks": unable to locate archive \(store API responded with status: Internal Server Error\)`)
}

func (s *DownloadSuite) createCharmArchieve(c *tc.C) []byte {
	tmpDir, err := os.MkdirTemp("", "charm")
	c.Assert(err, tc.ErrorIsNil)

	repo := repo.NewRepo(localCharmRepo, defaultSeries)
	charmPath := repo.CharmArchivePath(tmpDir, "dummy")

	path, err := os.ReadFile(charmPath)
	c.Assert(err, tc.ErrorIsNil)
	return path
}
