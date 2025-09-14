// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/facades/client/resources/mocks"
	coreresources "github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type BaseSuite struct {
	testhelpers.IsolationSuite

	backend *mocks.MockBackend
	factory *mocks.MockNewCharmRepository
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *BaseSuite) setUpTest(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.factory = mocks.NewMockNewCharmRepository(ctrl)
	s.backend = mocks.NewMockBackend(ctrl)
	return ctrl
}

func (s *BaseSuite) newFacade(c *tc.C) *resources.API {
	factoryFunc := func(_ *charm.URL) (resources.NewCharmRepository, error) {
		return s.factory, nil
	}
	facade, err := resources.NewResourcesAPI(s.backend, factoryFunc)
	c.Assert(err, tc.ErrorIsNil)
	return facade
}

func newResource(c *tc.C, name, username, data string) (coreresources.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Username = username
	if username == "" {
		res.Timestamp = time.Time{}
	}

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        name,
			Description: name + " description",
			Type:        "file",
			Path:        res.Path,
			Origin:      "upload",
			Revision:    0,
			Fingerprint: res.Fingerprint.Bytes(),
			Size:        res.Size,
		},
		ID:            res.ID,
		ApplicationID: res.ApplicationID,
		Username:      username,
		Timestamp:     res.Timestamp,
	}

	return res, apiRes
}
