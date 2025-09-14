// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type firewallerBaseSuite struct {
	testhelpers.IsolationSuite

	newFunc func(caller base.APICaller) clientCommmon
	objType string
}

type clientCommmon interface {
	WatchApplications() (watcher.StringsWatcher, error)
	WatchApplication(string) (watcher.NotifyWatcher, error)
	IsExposed(string) (bool, error)
	ApplicationConfig(string) (config.ConfigAttributes, error)
	Life(string) (life.Value, error)
}

type firewallerLegacySuite struct {
	firewallerBaseSuite
}

func TestFirewallerLegacySuite(t *tctesting.T) {
	tc.Run(t, &firewallerLegacySuite{
		firewallerBaseSuite{
			objType: "CAASFirewaller",
			newFunc: func(caller base.APICaller) clientCommmon {
				return caasfirewaller.NewClientLegacy(caller)
			},
		},
	})
}

type firewallerSidecarSuite struct {
	firewallerBaseSuite
}

func TestFirewallerSidecarSuite(t *tctesting.T) {
	tc.Run(t, &firewallerSidecarSuite{
		firewallerBaseSuite{
			objType: "CAASFirewallerSidecar",
			newFunc: func(caller base.APICaller) clientCommmon {
				return caasfirewaller.NewClientSidecar(caller)
			},
		},
	})
}

func (s *firewallerSidecarSuite) TestWatchOpenedPorts(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchOpenedPorts")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
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

	client := caasfirewaller.NewClientSidecar(apiCaller)
	watcher, err := client.WatchOpenedPorts()
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *firewallerSidecarSuite) TestGetOpenedPorts(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetOpenedPorts")
		c.Check(arg, tc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, tc.FitsTypeOf, &params.ApplicationOpenedPortsResults{})
		*(result.(*params.ApplicationOpenedPortsResults)) = params.ApplicationOpenedPortsResults{
			Results: []params.ApplicationOpenedPortsResult{{
				ApplicationPortRanges: []params.ApplicationOpenedPorts{
					{
						PortRanges: []params.PortRange{
							{
								FromPort: 80,
								ToPort:   8080,
								Protocol: "tcp",
							},
						},
					},
				},
			}},
		}
		return nil
	})

	client := caasfirewaller.NewClientSidecar(apiCaller)
	result, err := client.GetOpenedPorts("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, network.GroupedPortRanges{
		"": []network.PortRange{
			{
				FromPort: 80,
				ToPort:   8080,
				Protocol: "tcp",
			},
		},
	})
}

func (s *firewallerBaseSuite) TestIsExposed(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "IsExposed")
		c.Check(arg, tc.DeepEquals, params.Entities{
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

	client := s.newFunc(apiCaller)
	exposed, err := client.IsExposed("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exposed, tc.IsTrue)
}

func (s *firewallerBaseSuite) TestIsExposedError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	_, err := client.IsExposed("gitlab")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *firewallerBaseSuite) TestIsExposedInvalidEntityame(c *tc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.IsExposed("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *firewallerBaseSuite) TestLife(c *tc.C) {
	tag := names.NewApplicationTag("gitlab")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *firewallerBaseSuite) TestLifeError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	_, err := client.Life("gitlab")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *firewallerBaseSuite) TestLifeInvalidEntityame(c *tc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, tc.ErrorMatches, `application name "" not valid`)
}

func (s *firewallerBaseSuite) TestWatchApplications(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchApplications")
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	watcher, err := client.WatchApplications()
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *firewallerBaseSuite) TestWatchApplication(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Watch")
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

	client := s.newFunc(apiCaller)
	watcher, err := client.WatchApplication("gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *firewallerBaseSuite) TestApplicationConfig(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	cfg, err := client.ApplicationConfig("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, config.ConfigAttributes{"foo": "bar"})
}
