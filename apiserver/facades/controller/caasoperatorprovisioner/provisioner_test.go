// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"crypto/x509"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

func TestCAASProvisionerSuite(t *tctesting.T) {
	tc.Run(t, &CAASProvisionerSuite{})
}

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	resources          *common.Resources
	authorizer         *apiservertesting.FakeAuthorizer
	api                *caasoperatorprovisioner.API
	st                 *mockState
	storagePoolManager *mockStoragePoolManager
	registry           *mockStorageRegistry
	broker             *mocks.MockBroker
}

func (s *CAASProvisionerSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.broker = mocks.NewMockBroker(ctrl)

	api, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(
		s.resources, s.authorizer, s.st, s.st, s.storagePoolManager, s.registry, s.broker)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *CAASProvisionerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })
	s.PatchValue(&jujuversion.OfficialBuild, 0)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	s.storagePoolManager = &mockStoragePoolManager{}
	s.registry = &mockStorageRegistry{}

}

func (s *CAASProvisionerSuite) TestPermission(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.broker = mocks.NewMockBroker(ctrl)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(
		s.resources, s.authorizer, s.st, s.st, s.storagePoolManager, s.registry, s.broker)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplications(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	applicationNames := []string{"db2", "hadoop"}
	s.st.applicationWatcher.changes <- applicationNames
	result, err := s.api.WatchApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "1")
	c.Assert(result.Changes, tc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, tc.NotNil)
	c.Assert(resource, tc.Implements, new(state.StringsWatcher))
}

func (s *CAASProvisionerSuite) TestSetPasswords(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
	}

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "application-app", Password: "xxx-12345678901234567890"},
			{Tag: "application-another", Password: "yyy-12345678901234567890"},
			{Tag: "machine-0", Password: "zzz-12345678901234567890"},
		},
	}
	results, err := s.api.SetPasswords(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "entity application-another not found", Code: "not found"}},
			{&params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
	c.Assert(s.st.app.password, tc.Equals, "xxx-12345678901234567890")
}

func (s *CAASProvisionerSuite) TestLife(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
	}
	results, err := s.api.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-app"},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{
			Life: life.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoDefault(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		charm: &mockCharm{meta: &charm.Meta{}},
	}
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return("ghcr.io/juju/jujud-operator:2.6-beta3.666", nil)

	result, err := s.api.OperatorProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OperatorProvisioningInfoResults{
		Results: []params.OperatorProvisioningInfo{{
			ImageDetails: params.DockerImageInfo{
				RegistryPath: "ghcr.io/juju/jujud-operator:2.6-beta3.666",
				Repository:   "ghcr.io/juju",
			},
			BaseImageDetails: params.DockerImageInfo{
				RegistryPath: "ghcr.io/juju/charm-base:ubuntu-20.04",
				Repository:   "ghcr.io/juju",
			},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
			CharmStorage: &params.KubernetesFilesystemParams{
				StorageName: "charm",
				Size:        uint64(1024),
				Provider:    "kubernetes",
				Attributes: map[string]interface{}{
					"storage-class": "k8s-storage",
					"foo":           "bar",
				},
				Tags: map[string]string{
					"juju-model-uuid":      coretesting.ModelTag.Id(),
					"juju-controller-uuid": coretesting.ControllerTag.Id()},
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.operatorRepo = "somerepo"
	s.st.app = &mockApplication{
		charm: &mockCharm{meta: &charm.Meta{}},
	}
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return(s.st.operatorRepo+"/jujud-operator:"+"2.6-beta3.666", nil)

	result, err := s.api.OperatorProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OperatorProvisioningInfoResults{
		Results: []params.OperatorProvisioningInfo{{
			ImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/jujud-operator:" + "2.6-beta3.666",
				Repository:   s.st.operatorRepo,
			},
			BaseImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/charm-base:ubuntu-20.04",
				Repository:   s.st.operatorRepo,
			},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
			CharmStorage: &params.KubernetesFilesystemParams{
				StorageName: "charm",
				Size:        uint64(1024),
				Provider:    "kubernetes",
				Attributes: map[string]interface{}{
					"storage-class": "k8s-storage",
					"foo":           "bar",
				},
				Tags: map[string]string{
					"juju-model-uuid":      coretesting.ModelTag.Id(),
					"juju-controller-uuid": coretesting.ControllerTag.Id()},
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoNoStorage(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.operatorRepo = "somerepo"
	minVers := version.MustParse("2.8.0")
	s.st.app = &mockApplication{
		charm: &mockCharm{meta: &charm.Meta{MinJujuVersion: minVers}},
	}
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return(s.st.operatorRepo+"/jujud-operator:"+"2.6-beta3.666", nil)

	result, err := s.api.OperatorProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OperatorProvisioningInfoResults{
		Results: []params.OperatorProvisioningInfo{{
			ImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/jujud-operator:" + "2.6-beta3.666",
				Repository:   s.st.operatorRepo,
			},
			BaseImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/charm-base:ubuntu-20.04",
				Repository:   s.st.operatorRepo,
			},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoSidecarNoStorage(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.operatorRepo = "somerepo"
	s.st.app = &mockApplication{
		charm: &mockCharm{
			meta:     &charm.Meta{},
			manifest: &charm.Manifest{Bases: []charm.Base{{}}}},
	}
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return(s.st.operatorRepo+"/jujud-operator:"+"2.6-beta3.666", nil)
	result, err := s.api.OperatorProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OperatorProvisioningInfoResults{
		Results: []params.OperatorProvisioningInfo{{
			ImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/jujud-operator:" + "2.6-beta3.666",
				Repository:   s.st.operatorRepo,
			},
			BaseImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/charm-base:ubuntu-20.04",
				Repository:   s.st.operatorRepo,
			},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoNoStoragePool(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.storagePoolManager.SetErrors(errors.NotFoundf("pool"))
	s.st.operatorRepo = "somerepo"
	minVers := version.MustParse("2.7.0")
	s.st.app = &mockApplication{
		charm: &mockCharm{meta: &charm.Meta{MinJujuVersion: minVers}},
	}
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return(s.st.operatorRepo+"/jujud-operator:"+"2.6-beta3.666", nil)

	result, err := s.api.OperatorProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OperatorProvisioningInfoResults{
		Results: []params.OperatorProvisioningInfo{{
			ImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/jujud-operator:" + "2.6-beta3.666",
				Repository:   s.st.operatorRepo,
			},
			BaseImageDetails: params.DockerImageInfo{
				RegistryPath: s.st.operatorRepo + "/charm-base:ubuntu-20.04",
				Repository:   s.st.operatorRepo,
			},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
			CharmStorage: &params.KubernetesFilesystemParams{
				StorageName: "charm",
				Size:        uint64(1024),
				Provider:    "kubernetes",
				Attributes: map[string]interface{}{
					"storage-class": "k8s-storage",
				},
				Tags: map[string]string{
					"juju-model-uuid":      coretesting.ModelTag.Id(),
					"juju-controller-uuid": coretesting.ControllerTag.Id()},
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestAddresses(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	_, err := s.api.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "APIHostPortsForAgents")
}

func (s *CAASProvisionerSuite) TestIssueOperatorCertificate(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	res, err := s.api.IssueOperatorCertificate(params.Entities{
		Entities: []params.Entity{{Tag: "application-appname"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StateServingInfo")
	c.Assert(res.Results, tc.HasLen, 1)
	certInfo := res.Results[0]
	c.Assert(certInfo.Error, tc.IsNil)

	certs, signers, err := pki.UnmarshalPemData(append([]byte(certInfo.Cert),
		[]byte(certInfo.PrivateKey)...))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(signers), tc.Equals, 1)
	c.Assert(len(certs), tc.Equals, 2)

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(certInfo.CACert))
	c.Assert(ok, tc.IsTrue)
	_, err = certs[0].Verify(x509.VerifyOptions{
		DNSName: "appname",
		Roots:   roots,
	})
	c.Assert(err, tc.ErrorIsNil)
}
