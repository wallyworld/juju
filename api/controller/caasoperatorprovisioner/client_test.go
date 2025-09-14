// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/version/v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasoperatorprovisioner"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

type provisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisionerSuite(t *tctesting.T) {
	tc.Run(t, &provisionerSuite{})
}

func newClient(f basetesting.APICallerFunc) *caasoperatorprovisioner.Client {
	return caasoperatorprovisioner.NewClient(basetesting.BestVersionCaller{f, 5})
}

func (s *provisionerSuite) TestWatchApplications(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "WatchApplications")
		c.Assert(a, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.WatchApplications()
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestSetPasswords(c *tc.C) {
	passwords := []caasoperatorprovisioner.ApplicationPassword{
		{Name: "app", Password: "secret"},
	}
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "SetPasswords")
		c.Assert(a, tc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "application-app", Password: "secret"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	result, err := client.SetPasswords(passwords)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Combine(), tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestSetPasswordsCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	passwords := []caasoperatorprovisioner.ApplicationPassword{
		{Name: "app", Password: "secret"},
	}
	_, err := client.SetPasswords(passwords)
	c.Check(err, tc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *provisionerSuite) TestLife(c *tc.C) {
	tag := names.NewApplicationTag("app")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: tag.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasoperatorprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasoperatorprovisioner.NewClient(apiCaller)
	_, err := client.Life("gitlab")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *provisionerSuite) TestLifeInvalidApplicationName(c *tc.C) {
	client := caasoperatorprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *provisionerSuite) TestLifeCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	_, err := client.Life("gitlab")
	c.Check(err, tc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *provisionerSuite) TestOperatorProvisioningInfo(c *tc.C) {
	vers := version.MustParse("2.99.0")
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "OperatorProvisioningInfo")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.OperatorProvisioningInfoResults{})
		*(result.(*params.OperatorProvisioningInfoResults)) = params.OperatorProvisioningInfoResults{
			Results: []params.OperatorProvisioningInfo{{
				ImageDetails: params.DockerImageInfo{RegistryPath: "juju-operator-image"},
				Version:      vers,
				APIAddresses: []string{"10.0.0.1:1"},
				Tags:         map[string]string{"foo": "bar"},
				CharmStorage: &params.KubernetesFilesystemParams{
					Size:        10,
					Provider:    "kubernetes",
					StorageName: "stor",
					Tags:        map[string]string{"model": "model-tag"},
					Attributes:  map[string]interface{}{"key": "value"},
				},
			}}}
		return nil
	})
	info, err := client.OperatorProvisioningInfo("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, caasoperatorprovisioner.OperatorProvisioningInfo{
		ImageDetails: resources.DockerImageDetails{RegistryPath: "juju-operator-image"},
		Version:      vers,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags:         map[string]string{"foo": "bar"},
		CharmStorage: &storage.KubernetesFilesystemParams{
			Size:         10,
			Provider:     "kubernetes",
			StorageName:  "stor",
			ResourceTags: map[string]string{"model": "model-tag"},
			Attributes:   map[string]interface{}{"key": "value"},
		},
	})
}

func (s *provisionerSuite) TestOperatorProvisioningInfoArity(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "OperatorProvisioningInfo")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.OperatorProvisioningInfoResults{})
		*(result.(*params.OperatorProvisioningInfoResults)) = params.OperatorProvisioningInfoResults{
			Results: []params.OperatorProvisioningInfo{{}, {}},
		}
		return nil
	})
	_, err := client.OperatorProvisioningInfo("gitlab")
	c.Assert(err, tc.ErrorMatches, "expected one result, got 2")
}

func (s *provisionerSuite) TestIssueOperatorCertificate(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "IssueOperatorCertificate")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-appymcappface"}}})
		c.Assert(result, tc.FitsTypeOf, &params.IssueOperatorCertificateResults{})
		*(result.(*params.IssueOperatorCertificateResults)) = params.IssueOperatorCertificateResults{
			Results: []params.IssueOperatorCertificateResult{{
				CACert:     "ca cert",
				Cert:       "cert",
				PrivateKey: "private key",
			}},
		}
		return nil
	})
	info, err := client.IssueOperatorCertificate("appymcappface")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, caasoperatorprovisioner.OperatorCertificate{
		CACert:     "ca cert",
		Cert:       "cert",
		PrivateKey: "private key",
	})
}

func (s *provisionerSuite) TestIssueOperatorCertificateArity(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASOperatorProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "IssueOperatorCertificate")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-appymcappface"}}})
		c.Assert(result, tc.FitsTypeOf, &params.IssueOperatorCertificateResults{})
		return nil
	})
	_, err := client.IssueOperatorCertificate("appymcappface")
	c.Assert(err, tc.ErrorMatches, "expected one result, got 0")
}
