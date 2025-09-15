// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"net/url"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/core/base"
)

type resolveSuite struct {
	charmsAPI      *mocks.MockCharmsAPI
	downloadClient *mocks.MockDownloadBundleClient
	bundle         *mocks.MockBundle
}

func TestResolveSuite(t *tctesting.T) {
	tc.Run(t, &resolveSuite{})
}

func (s *resolveSuite) TestResolveCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-3")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedOrigin, obtainedBases, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, tc.Equals, "edge")
	c.Assert(obtainedBases, tc.SameContents, []base.Base{
		base.MustParseBaseFromString("ubuntu@18.04"),
		base.MustParseBaseFromString("ubuntu@20.04"),
	})
	c.Assert(obtainedURL, tc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmWithAPIError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCallWithAPIError(curl, "edge", errors.New("bad"))

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, _, err = charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *tc.C) {
	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, tc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, tc.NotNil)
	c.Assert(obtainedOrigin.Risk, tc.Equals, "")
}

func (s *resolveSuite) TestResolveBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectBundleResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
		Type:   "bundle",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, tc.Equals, "edge")
	c.Assert(obtainedURL, tc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	curl.Series = "bionic"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) TestCharmHubGetBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-1")
	c.Assert(err, tc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Type:   "bundle",
		Risk:   "edge",
	}
	s.expectedCharmHubGetBundle(c, curl, origin)

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	bundle, err := charmAdapter.GetBundle(curl, origin, "/tmp/")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bundle, tc.DeepEquals, s.bundle)
}

func (s *resolveSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
	s.downloadClient = mocks.NewMockDownloadBundleClient(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	return ctrl
}

func (s *resolveSuite) expectBundleResolutionCall(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "bundle",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCall(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "charm",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCallWithAPIError(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "charm",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
		Error: err,
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, nil)
}

func (s *resolveSuite) expectedCharmHubGetBundle(c *tc.C, curl *charm.URL, origin commoncharm.Origin) {
	surl := "http://messhuggah.com"
	s.charmsAPI.EXPECT().GetDownloadInfo(curl, origin).Return(apicharm.DownloadInfo{
		URL: surl,
	}, nil)
	url, err := url.Parse(surl)
	c.Assert(err, tc.ErrorIsNil)
	s.downloadClient.EXPECT().DownloadAndReadBundle(gomock.Any(), url, "/tmp/", gomock.Any()).Return(s.bundle, nil)
}
