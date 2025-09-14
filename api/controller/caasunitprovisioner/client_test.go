// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasunitprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

type unitprovisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestUnitprovisionerSuite(t *tctesting.T) {
	tc.Run(t, &unitprovisionerSuite{})
}

func newClient(f basetesting.APICallerFunc) *caasunitprovisioner.Client {
	return caasunitprovisioner.NewClient(basetesting.BestVersionCaller{f, 1})
}

func (s *unitprovisionerSuite) TestProvisioningInfo(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ProvisioningInfo")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.KubernetesProvisioningInfoResults{})
		*(result.(*params.KubernetesProvisioningInfoResults)) = params.KubernetesProvisioningInfoResults{
			Results: []params.KubernetesProvisioningInfoResult{{
				Result: &params.KubernetesProvisioningInfo{
					PodSpec:     "foo",
					Tags:        map[string]string{"foo": "bar"},
					Constraints: constraints.MustParse("mem=4G"),
					ImageRepo: params.DockerImageInfo{
						RegistryPath: "operator/image-path",
					},
					DeploymentInfo: &params.KubernetesDeploymentInfo{
						DeploymentType: "stateful",
						ServiceType:    "loadbalancer",
					},
					Filesystems: []params.KubernetesFilesystemParams{{
						StorageName: "database",
						Size:        uint64(100),
						Provider:    "k8s",
						Tags:        map[string]string{"tag": "resource"},
						Attributes:  map[string]interface{}{"key": "value"},
						Attachment: &params.KubernetesFilesystemAttachmentParams{
							Provider:   "k8s",
							MountPoint: "/path/to/here",
							ReadOnly:   true,
						}},
					},
					Devices: []params.KubernetesDeviceParams{
						{
							Type:       "nvidia.com/gpu",
							Count:      3,
							Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
						},
					},
				},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	info, err := client.ProvisioningInfo("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &caasunitprovisioner.ProvisioningInfo{
		PodSpec:     "foo",
		Tags:        map[string]string{"foo": "bar"},
		Constraints: constraints.MustParse("mem=4G"),
		ImageDetails: resources.DockerImageDetails{
			RegistryPath: "operator/image-path",
		},
		DeploymentInfo: caasunitprovisioner.DeploymentInfo{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName:  "database",
			Size:         uint64(100),
			Provider:     storage.ProviderType("k8s"),
			ResourceTags: map[string]string{"tag": "resource"},
			Attributes:   map[string]interface{}{"key": "value"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "/path/to/here",
				AttachmentParams: storage.AttachmentParams{
					Provider: storage.ProviderType("k8s"),
					ReadOnly: true,
				},
			},
		}},
		Devices: []devices.KubernetesDeviceParams{{
			Type:       devices.DeviceType("nvidia.com/gpu"),
			Count:      3,
			Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
		}},
	})
}

func (s *unitprovisionerSuite) TestProvisioningInfoError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.KubernetesProvisioningInfoResults)) = params.KubernetesProvisioningInfoResults{
			Results: []params.KubernetesProvisioningInfoResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	_, err := client.ProvisioningInfo("gitlab")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *unitprovisionerSuite) TestProvisioningInfoInvalidApplicationName(c *tc.C) {
	client := caasunitprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.ProvisioningInfo("gitlab/0")
	c.Assert(err, tc.ErrorMatches, `application name "gitlab/0" not valid`)
}

func (s *unitprovisionerSuite) TestLife(c *tc.C) {
	s.testLife(c, names.NewApplicationTag("gitlab"))
	s.testLife(c, names.NewUnitTag("gitlab/0"))
}

func (s *unitprovisionerSuite) testLife(c *tc.C, tag names.Tag) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
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

	client := caasunitprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *unitprovisionerSuite) TestLifeError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	_, err := client.Life("gitlab/0")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *unitprovisionerSuite) TestLifeInvalidEntityame(c *tc.C) {
	client := caasunitprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, tc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *unitprovisionerSuite) TestWatchApplications(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchApplications")
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplications()
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestWatchApplicationScale(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchApplicationsScale")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplicationScale("gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationScale(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ApplicationsScale")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.IntResults{})
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Result: 5,
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	scale, err := client.ApplicationScale("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scale, tc.Equals, 5)
}

func (s *unitprovisionerSuite) TestDeploymentMode(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "DeploymentMode")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "workload",
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	mode, err := client.DeploymentMode("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mode, tc.Equals, caas.ModeWorkload)
}

func (s *unitprovisionerSuite) TestWatchPodSpec(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchPodSpec")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchPodSpec("gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationConfig(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ApplicationsConfig")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ApplicationGetConfigResults{})
		*(result.(*params.ApplicationGetConfigResults)) = params.ApplicationGetConfigResults{
			Results: []params.ConfigResult{{
				Config: map[string]interface{}{"foo": "bar"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	cfg, err := client.ApplicationConfig("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, config.ConfigAttributes{"foo": "bar"})
}

func (s *unitprovisionerSuite) TestUpdateUnits(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "UpdateApplicationsUnits")
		c.Assert(a, tc.DeepEquals, params.UpdateApplicationUnitArgs{
			Args: []params.UpdateApplicationUnits{
				{
					ApplicationTag: "application-app",
					Units: []params.ApplicationUnitParams{
						{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
							Status: "active", Info: "message"},
					},
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
		*(result.(*params.UpdateApplicationUnitResults)) = params.UpdateApplicationUnitResults{
			Results: []params.UpdateApplicationUnitResult{{
				Info: &params.UpdateApplicationUnitsInfo{
					Units: []params.ApplicationUnitInfo{
						{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
					},
				},
			}},
		}
		return nil
	})
	info, err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
				Status: "active", Info: "message"},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(info, tc.DeepEquals, &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
		},
	})
}

func (s *unitprovisionerSuite) TestUpdateUnitsCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
		*(result.(*params.UpdateApplicationUnitResults)) = params.UpdateApplicationUnitResults{
			Results: []params.UpdateApplicationUnitResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	info, err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", Address: "address"},
		},
	})
	c.Check(err, tc.ErrorMatches, `expected 1 result\(s\), got 2`)
	c.Assert(info, tc.IsNil)
}

func (s *unitprovisionerSuite) TestUpdateApplicationService(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "UpdateApplicationsService")
		c.Assert(a, tc.DeepEquals, params.UpdateApplicationServiceArgs{
			Args: []params.UpdateApplicationServiceArg{
				{
					ApplicationTag: "application-app",
					ProviderId:     "id",
					Addresses:      []params.Address{{Value: "10.0.0.1"}},
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *unitprovisionerSuite) TestUpdateApplicationServiceCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, tc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *unitprovisionerSuite) TestSetOperatorStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetOperatorStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "error",
				Info:   "broken",
				Data:   map[string]interface{}{"foo": "bar"},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	err := client.SetOperatorStatus("gitlab", status.Error, "broken", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestClearApplicationResources(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ClearApplicationsResources")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	err := client.ClearApplicationResources("gitlab")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestWatchApplicationTrustHash(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchApplicationsTrustHash")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplicationTrustHash("gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationTrust(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASUnitProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ApplicationsTrust")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	trust, err := client.ApplicationTrust("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(trust, tc.IsTrue)
}
