// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type unitSuite struct {
	coretesting.BaseSuite
}

func TestUnitSuite(t *tctesting.T) {
	tc.Run(t, &unitSuite{})
}

func (s *unitSuite) TestUnitAndUnitTag(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Refresh")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.UnitRefreshResults{})
		*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
			Results: []params.UnitRefreshResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)
	unit, err := client.Unit(tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Name(), tc.Equals, "mysql/0")
	c.Assert(unit.Tag(), tc.Equals, tag)
	c.Assert(unit.Life(), tc.Equals, life.Alive)
	c.Assert(unit.ApplicationName(), tc.Equals, "mysql")
	c.Assert(unit.ApplicationTag(), tc.Equals, names.NewApplicationTag("mysql"))
}

func (s *unitSuite) TestUnitAndUnitTagNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)
	_, err := client.Unit(tag)
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetAgentStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetAgentStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: "unit-mysql-0", Status: "idle", Info: "blah", Data: map[string]interface{}{"foo": "bar"}},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetAgentStatus(status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetAgentStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetAgentStatus(status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetUnitStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetUnitStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: "unit-mysql-0", Status: "idle", Info: "blah", Data: map[string]interface{}{"foo": "bar"}},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetUnitStatus(status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetUnitStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetUnitStatus(status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestUnitStatus(c *tc.C) {
	now := time.Now()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "UnitStatus")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StatusResults{})
		*(result.(*params.StatusResults)) = params.StatusResults{
			Results: []params.StatusResult{{
				Id:     "mysql/0",
				Life:   life.Alive,
				Status: "maintenance",
				Info:   "blah",
				Data:   map[string]interface{}{"foo": "bar"},
				Since:  &now,
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	statusInfo, err := unit.UnitStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusInfo, tc.DeepEquals, params.StatusResult{
		Id:     "mysql/0",
		Life:   life.Alive,
		Status: status.Maintenance.String(),
		Info:   "blah",
		Data:   map[string]interface{}{"foo": "bar"},
		Since:  &now,
	})
}

func (s *unitSuite) TestUnitStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.UnitStatus()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestEnsureDead(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "EnsureDead")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.EnsureDead()
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestEnsureDeadNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.EnsureDead()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestDestroy(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Destroy")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Destroy()
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestDestroyNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Destroy()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestDestroyAllSubordinates(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "DestroyAllSubordinates")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.DestroyAllSubordinates()
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestDestroyAllSubordinatesNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.DestroyAllSubordinates()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestRefresh(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Refresh")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.UnitRefreshResults{})
		*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
			Results: []params.UnitRefreshResult{{
				Life:       life.Dying,
				Resolved:   params.ResolvedRetryHooks,
				ProviderID: "666",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Life(), tc.Equals, life.Dying)
	c.Assert(unit.Resolved(), tc.Equals, params.ResolvedRetryHooks)
	c.Assert(unit.Life(), tc.Equals, life.Dying)
}

func (s *unitSuite) TestRefreshNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Refresh()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestClearResolved(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "ClearResolved")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.ClearResolved()
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestClearResolvedNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.ClearResolved()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatch(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Watch")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.Watch()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *unitSuite) TestWatchNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.Watch()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchRelations(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchUnitRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Changes:          []string{"666"},
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchRelations()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchRelationsNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchRelations()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestAssignedMachine(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "AssignedMachine")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "machine-666",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	tag, err := unit.AssignedMachine()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag, tc.Equals, names.NewMachineTag("666"))
}

func (s *unitSuite) TestAssignedMachineNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.AssignedMachine()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPrincipalName(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "GetPrincipal")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringBoolResults{})
		*(result.(*params.StringBoolResults)) = params.StringBoolResults{
			Results: []params.StringBoolResult{{
				Result: "unit-wordpress-0",
				Ok:     true,
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	name, ok, err := unit.PrincipalName()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "wordpress/0")
	c.Assert(ok, tc.IsTrue)
}

func (s *unitSuite) TestPrincipalNameNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, _, err := unit.PrincipalName()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestHasSubordinates(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "HasSubordinates")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	ok, err := unit.HasSubordinates()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

func (s *unitSuite) TestHasSubordinatesNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.HasSubordinates()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPublicAddress(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "PublicAddress")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "1.1.1.1",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.PublicAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "1.1.1.1")
}

func (s *unitSuite) TestPublicAddressNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.PublicAddress()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPrivateAddress(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "PrivateAddress")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "1.1.1.1",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.PrivateAddress()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "1.1.1.1")
}

func (s *unitSuite) TestPrivateAddressNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.PrivateAddress()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestAvailabilityZone(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "AvailabilityZone")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "a-zone",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.AvailabilityZone()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "a-zone")
}

func (s *unitSuite) TestAvailabilityZoneNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.AvailabilityZone()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestCharmURL(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "CharmURL")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringBoolResults{})
		*(result.(*params.StringBoolResults)) = params.StringBoolResults{
			Results: []params.StringBoolResult{{
				Result: "ch:mysql",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	curl, err := unit.CharmURL()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl, tc.Equals, "ch:mysql")
}

func (s *unitSuite) TestCharmURLNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.CharmURL()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetCharmURL(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetCharmURL")
		c.Assert(arg, tc.DeepEquals, params.EntitiesCharmURL{
			Entities: []params.EntityCharmURL{
				{Tag: "unit-mysql-0", CharmURL: "ch:mysql"},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetCharmURL("ch:mysql")
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetCharmURLNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetCharmURL("ch:mysql")
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestNetworkInfo(c *tc.C) {
	relId := 2
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "NetworkInfo")
		c.Check(arg, tc.DeepEquals, params.NetworkInfoParams{
			Unit:       "unit-mysql-0",
			Endpoints:  []string{"server"},
			RelationId: &relId,
		})
		c.Assert(result, tc.FitsTypeOf, &params.NetworkInfoResults{})
		*(result.(*params.NetworkInfoResults)) = params.NetworkInfoResults{
			Results: map[string]params.NetworkInfoResult{
				"db": {
					Error: &params.Error{Message: "FAIL"},
				}},
		}
		return nil
	})

	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	result, err := unit.NetworkInfo([]string{"server"}, &relId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result["db"].Error, tc.ErrorMatches, "FAIL")
}

func (s *unitSuite) TestNetworkInfoNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	relId := 2
	_, err := unit.NetworkInfo([]string{"server"}, &relId)
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestConfigSettings(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "ConfigSettings")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ConfigSettingsResults{})
		*(result.(*params.ConfigSettingsResults)) = params.ConfigSettingsResults{
			Results: []params.ConfigSettingsResult{{
				Settings: params.ConfigSettings{"foo": "bar"},
			}},
		}
		return nil
	})

	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	settings, err := unit.ConfigSettings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, charm.Settings{
		"foo": "bar",
	})
}

func (s *unitSuite) TestConfigSettingsNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.ConfigSettings()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchConfigSettingsHash(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchConfigSettingsHash")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Changes:          []string{"666"},
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchConfigSettingsHash()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchConfigSettingsHashNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchConfigSettingsHash()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchTrustConfigSettingsHash(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchTrustConfigSettingsHash")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Changes:          []string{"666"},
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchTrustConfigSettingsHash()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchTrustConfigSettingsHashNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchTrustConfigSettingsHash()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchAddressesHash(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchUnitAddressesHash")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Changes:          []string{"666"},
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchAddressesHash()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchAddressesHashNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchAddressesHash()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchUpgradeSeriesNotifications(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchUpgradeSeriesNotifications")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchUpgradeSeriesNotifications()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *unitSuite) TestWatchUpgradeSeriesNotificationsNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchUpgradeSeriesNotifications()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestUpgradeSeriesStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(arg, tc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity:  params.Entity{Tag: "unit-mysql-0"},
				Status:  "completed",
				Message: "done",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetUpgradeSeriesStatus(model.UpgradeSeriesCompleted, "done")
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestUpgradeSeriesStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	err := unit.SetUpgradeSeriesStatus(model.UpgradeSeriesCompleted, "done")
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetUpgradeSeriesStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.UpgradeSeriesStatusResults{})
		*(result.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{
				Status: "completed",
				Target: "focal",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	seriesStatus, target, err := unit.UpgradeSeriesStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seriesStatus, tc.Equals, model.UpgradeSeriesCompleted)
	c.Check(target, tc.Equals, "focal")
}

func (s *unitSuite) TestSetUpgradeSeriesStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, _, err := unit.UpgradeSeriesStatus()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestRelationStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "RelationsStatus")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.RelationUnitStatusResults{})
		*(result.(*params.RelationUnitStatusResults)) = params.RelationUnitStatusResults{
			Results: []params.RelationUnitStatusResult{{
				RelationResults: []params.RelationUnitStatus{{
					RelationTag: "relation-wordpress.server#mysql.db",
					Suspended:   true,
					InScope:     true,
				}},
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	relStatus, err := unit.RelationsStatus()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relStatus, tc.DeepEquals, []uniter.RelationStatus{{
		Tag:       names.NewRelationTag("wordpress:server mysql:db"),
		Suspended: true,
		InScope:   true,
	}})
}

func (s *unitSuite) TestRelationStatusNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.RelationsStatus()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestUnitState(c *tc.C) {
	unitState := params.UnitStateResult{
		StorageState:  "storage",
		SecretState:   "secret",
		UniterState:   "uniter",
		CharmState:    map[string]string{"foo": "bar"},
		RelationState: map[int]string{666: "666"},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "State")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.UnitStateResults{})
		*(result.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{unitState},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	result, err := unit.State()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, unitState)
}

func (s *unitSuite) TestUnitStateNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.State()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetState(c *tc.C) {
	unitState := params.SetUnitStateArg{
		Tag:           "unit-mysql-0",
		CharmState:    &map[string]string{"foo": "bar"},
		RelationState: &map[int]string{666: "666"},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetState")
		c.Assert(arg, tc.DeepEquals, params.SetUnitStateArgs{
			Args: []params.SetUnitStateArg{unitState},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetState(unitState)
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetStateNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	err := unit.SetState(params.SetUnitStateArg{})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchInstanceData(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchInstanceData")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchInstanceData()
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *unitSuite) TestWatchInstanceDataNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchInstanceData()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestLXDProfileName(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "LXDProfileName")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "juju-default-mysql-0",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	profile, err := unit.LXDProfileName()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(profile, tc.Equals, "juju-default-mysql-0")
}

func (s *unitSuite) TestLXDProfileNameNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.LXDProfileName()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestCanApplyLXDProfile(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "CanApplyLXDProfile")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	canApply, err := unit.CanApplyLXDProfile()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(canApply, tc.IsTrue)
}

func (s *unitSuite) TestCanApplyLXDProfileNotImplemented(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewState(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.CanApplyLXDProfile()
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}
