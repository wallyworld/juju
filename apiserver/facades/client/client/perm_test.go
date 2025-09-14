// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/annotations"
	"github.com/juju/juju/api/client/application"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/modelconfig"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type permSuite struct {
	baseSuite
}

func TestPermSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &permSuite{})
}

// Most (if not all) of the permission tests below aim to test
// end-to-end operations execution through the API, but do not care
// about the results. They only test that a call is succeeds or fails
// (usually due to "permission denied"). There are separate test cases
// testing each individual API call data flow later on.

func allowed(allow []names.Tag) map[names.Tag]bool {
	p := make(map[names.Tag]bool)
	if allow != nil {
		for _, e := range allow {
			p[e] = true
		}
		return p
	}
	return p
}

func (s *permSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
	client.SkipReplicaCheck(s)
}

func (s *permSuite) TestOperationPermClientSetApplicationConstraints(c *tc.C) {
	s.testOperationPerm(c, opClientSetApplicationConstraints)
}

func (s *permSuite) TestOperationPermClientSetModelConstraints(c *tc.C) {
	s.testOperationPerm(c, opClientSetModelConstraints)
}

func (s *permSuite) TestOperationPermClientModelGet(c *tc.C) {
	s.testOperationPerm(c, opClientModelGet)
}

func (s *permSuite) TestOperationPermClientModelSet(c *tc.C) {
	s.testOperationPerm(c, opClientModelSet)
}

func (s *permSuite) TestOperationPermClientWatchAll(c *tc.C) {
	s.testOperationPerm(c, opClientWatchAll)
}

func (s *permSuite) TestOperationPermApplicationAddRelation(c *tc.C) {
	s.testOperationPerm(c, opClientAddRelation)
}

func (s *permSuite) TestOperationPermApplicationDestroyRelation(c *tc.C) {
	s.testOperationPerm(c, opClientDestroyRelation)
}

func (s *permSuite) TestOperationPermApplicationGetConstraints(c *tc.C) {
	s.testOperationPerm(c, opClientGetApplicationConstraints)
}

func (s *permSuite) TestOperationPermDestroyUnits(c *tc.C) {
	s.testOperationPerm(c, opClientDestroyUnit)
}

func (s *permSuite) TestOperationPermApplicationAddUnits(c *tc.C) {
	s.testOperationPerm(c, opClientAddApplicationUnits)
}

func (s *permSuite) TestOperationPermApplicationGet(c *tc.C) {
	s.testOperationPerm(c, opClientApplicationGet)
}

func (s *permSuite) TestOperationPermAnnotationsGetAnnotations(c *tc.C) {
	s.testOperationPerm(c, opClientGetAnnotations)
}

func (s *permSuite) TestOperationPermClientStatus(c *tc.C) {
	s.testOperationPerm(c, opClientStatus)
}

func (s *permSuite) TestOperationPermApplicationResolveUnitErrors(c *tc.C) {
	s.testOperationPerm(c, opClientResolved)
}

func (s *permSuite) TestOperationPermApplicationExpose(c *tc.C) {
	s.testOperationPerm(c, opClientApplicationExpose)
}

func (s *permSuite) TestOperationPermApplicationUnexpose(c *tc.C) {
	s.testOperationPerm(c, opClientApplicationUnexpose)
}

func (s *permSuite) TestOperationPermAnnotationsSetAnnotations(c *tc.C) {
	s.testOperationPerm(c, opClientSetAnnotations)
}

func (s *permSuite) TestOperationPermApplicationDestroyUnits(c *tc.C) {
	s.testOperationPerm(c, opClientDestroyApplicationUnits)
}

func (s *permSuite) TestOperationPermApplicationDestroy(c *tc.C) {
	s.testOperationPerm(c, opClientApplicationDestroy)
}

func (s *permSuite) TestOperationPermApplicationDestroyApplication(c *tc.C) {
	s.testOperationPerm(c, opClientDestroyApplication)
}

func (s *permSuite) TestOperationPermApplicationSetCharm(c *tc.C) {
	s.testOperationPerm(c, opClientApplicationSetCharm)
}

func (s *permSuite) testOperationPerm(
	c *tc.C,
	op func(c *tc.C, st api.Connection, mst *state.State) (reset func(), err error),
) {
	allow := allowed([]names.Tag{s.AdminUserTag(c), names.NewLocalUserTag("other")})
	for j, e := range s.setUpScenario(c) {
		c.Logf("\n------\ntest %d; entity %q", j, e)
		st := s.openAs(c, e)
		reset, err := op(c, st, s.State)
		if allow[e] {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
				Message: "permission denied",
				Code:    "unauthorized access",
			})
			c.Check(err, tc.Satisfies, params.IsCodeUnauthorized)
		}
		reset()
		_ = st.Close()
	}
}

func opClientAddRelation(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).AddRelation([]string{"nosuch1", "nosuch2"}, nil)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	err := application.NewClient(st).DestroyRelation((*bool)(nil), (*time.Duration)(nil), "nosuch1", "nosuch2")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientStatus(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	status, err := apiclient.NewClient(st, loggertesting.WrapCheckLog(c)).Status(nil)
	if err != nil {
		c.Check(status, tc.IsNil)
		return func() {}, err
	}
	clearSinceTimes(status)
	clearSinceTimes(scenarioStatus)
	clearContollerTimestamp(status)
	c.Assert(status, tc.DeepEquals, scenarioStatus)
	return func() {}, nil
}

func opClientApplicationGet(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).Get(model.GenerationMaster, "wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientApplicationExpose(c *tc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).Expose("wordpress", nil)
	if err != nil {
		return func() {}, err
	}
	return func() {
		svc, err := mst.Application("wordpress")
		c.Assert(err, tc.ErrorIsNil)
		err = svc.ClearExposed()
		c.Assert(err, tc.ErrorIsNil)
	}, nil
}

func opClientApplicationUnexpose(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	err := application.NewClient(st).Unexpose("wordpress", nil)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientResolved(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	err := application.NewClient(st).ResolveUnitErrors([]string{"wordpress/1"}, false, false)
	// There are several scenarios in which this test is called, one is
	// that the user is not authorized.  In that case we want to exit now,
	// letting the error percolate out so the caller knows that the
	// permission error was correctly generated.
	if err != nil && params.IsCodeUnauthorized(err) {
		return func() {}, err
	}
	// Otherwise, the user was authorized, but we expect an error anyway
	// because the unit is not in an error state when we tried to resolve
	// the error.  Therefore, since it is complaining it means that the
	// call to Resolved worked, so we're happy.
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `unit "wordpress/1" is not in an error state`)
	return func() {}, nil
}

func opClientGetAnnotations(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	ann, err := annotations.NewClient(st).Get([]string{"application-wordpress"})
	if err != nil {
		return func() {}, err
	}
	c.Assert(ann, tc.DeepEquals, []params.AnnotationsGetResult{{
		EntityTag:   "application-wordpress",
		Annotations: map[string]string{},
	}})
	return func() {}, nil
}

func opClientSetAnnotations(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	pairs := map[string]string{"key1": "value1", "key2": "value2"}
	setParams := map[string]map[string]string{
		"application-wordpress": pairs,
	}
	_, err := annotations.NewClient(st).Set(setParams)
	if err != nil {
		return func() {}, err
	}
	return func() {
		pairs := map[string]string{"key1": "", "key2": ""}
		setParams := map[string]map[string]string{
			"application-wordpress": pairs,
		}
		_, err := annotations.NewClient(st).Set(setParams)
		c.Assert(err, tc.ErrorIsNil)
	}, nil
}

func opClientApplicationSetCharm(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	cfg := application.SetCharmConfig{
		ApplicationName: "nosuch",
		CharmID: application.CharmID{
			URL:    "local:wordpress",
			Origin: apicharm.Origin{Source: "local"},
		},
	}
	err := application.NewClient(st).SetCharm(model.GenerationMaster, cfg)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientAddApplicationUnits(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).AddUnits(application.AddUnitsParams{
		ApplicationName: "nosuch",
		NumUnits:        1,
	})
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyApplicationUnits(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyUnits(
		application.DestroyUnitsParams{Units: []string{"wordpress/99"}})
	if err != nil && strings.HasPrefix(err.Error(), "no units were destroyed") {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyUnit(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"wordpress/99"},
	})
	return func() {}, err
}

func opClientApplicationDestroy(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyApplications(
		application.DestroyApplicationsParams{Applications: []string{"non-existent"}})
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyApplication(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"non-existent"},
	})
	return func() {}, err
}

func opClientGetApplicationConstraints(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := application.NewClient(st).GetConstraints("wordpress")
	return func() {}, err
}

func opClientSetApplicationConstraints(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := application.NewClient(st).SetConstraints("wordpress", nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientSetModelConstraints(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := modelconfig.NewClient(st).SetModelConstraints(nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientModelGet(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	_, err := modelconfig.NewClient(st).ModelGet()
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientModelSet(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	args := map[string]interface{}{"some-key": "some-value"}
	err := modelconfig.NewClient(st).ModelSet(args)
	if err != nil {
		return func() {}, err
	}
	return func() {
		args["some-key"] = nil
		modelconfig.NewClient(st).ModelSet(args)
	}, nil
}

func opClientWatchAll(c *tc.C, st api.Connection, _ *state.State) (func(), error) {
	watcher, err := apiclient.NewClient(st, loggertesting.WrapCheckLog(c)).WatchAll()
	if err == nil {
		watcher.Stop()
	}
	return func() {}, err
}
