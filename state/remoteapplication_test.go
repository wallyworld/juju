// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type remoteApplicationSuite struct {
	ConnSuite
	application            *state.RemoteApplication
	externalControllerUUID string
}

func TestRemoteApplicationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &remoteApplicationSuite{})
}

func (s *remoteApplicationSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.externalControllerUUID = utils.MustNewUUID().String()
	s.makeRemoteApplication(c, "mysql", "me/model.mysql")
	rc, err := state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc, tc.Equals, 1)
}

func (s *remoteApplicationSuite) makeRemoteApplication(c *tc.C, name, url string) {
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}

	spaces := []*environs.ProviderSpaceInfo{{
		CloudType: "ec2",
		ProviderAttributes: map[string]interface{}{
			"thing1":  23,
			"thing2":  "halberd",
			"network": "network-1",
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "public",
			ProviderId: "juju-space-public",
			Subnets: []network.SubnetInfo{{
				ProviderId:        "juju-subnet-12",
				CIDR:              "1.2.3.0/24",
				AvailabilityZones: []string{"az1", "az2"},
				ProviderSpaceId:   "juju-space-public",
				ProviderNetworkId: "network-1",
			}},
		},
	}, {
		CloudType: "ec2",
		ProviderAttributes: map[string]interface{}{
			"thing1":  24,
			"thing2":  "bardiche",
			"network": "network-1",
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "private",
			ProviderId: "juju-space-private",
			Subnets: []network.SubnetInfo{{
				ProviderId:        "juju-subnet-24",
				CIDR:              "1.2.4.0/24",
				AvailabilityZones: []string{"az1", "az2"},
				ProviderSpaceId:   "juju-space-private",
				ProviderNetworkId: "network-1",
			}},
		},
	}}
	bindings := map[string]string{
		"db":       "private",
		"db-admin": "private",
		"logging":  "public",
	}
	mac, err := newMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	s.application, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:                   name,
		URL:                    url,
		ExternalControllerUUID: s.externalControllerUUID,
		SourceModel:            s.Model.ModelTag(),
		Token:                  "app-token",
		Endpoints:              eps,
		Spaces:                 spaces,
		Bindings:               bindings,
		Macaroon:               mac,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationSuite) TestNoStatusForConsumerProxy(c *tc.C) {
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = application.Status()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestUseSuppliedVersionForConsumerProxy(c *tc.C) {
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application.ConsumeVersion(), tc.Equals, 666)
}

func (s *remoteApplicationSuite) TestConsumeVersion(c *tc.C) {
	c.Assert(s.application.ConsumeVersion(), tc.Equals, 1)
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "hosted-mysql",
		URL:         "me/model.mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application.ConsumeVersion(), tc.Equals, 2)
}

func (s *remoteApplicationSuite) TestInitialStatus(c *tc.C) {
	appStatus, err := s.application.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appStatus.Since, tc.NotNil)
	appStatus.Since = nil
	c.Assert(appStatus, tc.DeepEquals, status.StatusInfo{
		Status: status.Unknown,
		Data:   map[string]interface{}{},
	})
}

func (s *remoteApplicationSuite) TestStatus(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "busy",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	app, err := s.State.RemoteApplication("mysql")
	c.Assert(err, tc.ErrorIsNil)
	appStatus, err := app.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appStatus.Since, tc.NotNil)
	appStatus.Since = nil
	c.Assert(appStatus, tc.DeepEquals, status.StatusInfo{
		Status:  status.Maintenance,
		Message: "busy",
		Data:    map[string]interface{}{"foo": "bar"},
	})
}

func (s *remoteApplicationSuite) TestSetStatusSince(c *tc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	appStatus, err := s.application.Status()
	c.Assert(err, tc.ErrorIsNil)
	firstTime := appStatus.Since
	c.Assert(firstTime, tc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), tc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	err = s.application.SetStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
	appStatus, err = s.application.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *appStatus.Since), tc.IsTrue)
}

func (s *remoteApplicationSuite) TestGetSetStatusNotFound(c *tc.C) {
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Check(err, tc.ErrorIsNil)

	statusInfo, err := s.application.Status()
	c.Check(err, tc.ErrorMatches, `cannot get status: saas application "mysql" not found`)
	c.Check(statusInfo, tc.DeepEquals, status.StatusInfo{})
}

func (s *remoteApplicationSuite) TestTag(c *tc.C) {
	c.Assert(s.application.Tag().String(), tc.Equals, "application-mysql")
}

func (s *remoteApplicationSuite) TestURL(c *tc.C) {
	url, ok := s.application.URL()
	c.Assert(ok, tc.IsTrue)
	c.Assert(url, tc.Equals, "me/model.mysql")

	// Add another remote application without a URL.
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql1",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
	})
	c.Assert(err, tc.ErrorIsNil)
	url, ok = app.URL()
	c.Assert(ok, tc.IsFalse)
	c.Assert(url, tc.Equals, "")
}

func (s *remoteApplicationSuite) TestSpaces(c *tc.C) {
	spaces := s.application.Spaces()
	c.Assert(spaces, tc.DeepEquals, []state.RemoteSpace{{
		CloudType:  "ec2",
		Name:       "public",
		ProviderId: "juju-space-public",
		ProviderAttributes: map[string]interface{}{
			"thing1":  23,
			"thing2":  "halberd",
			"network": "network-1",
		},
		Subnets: []state.RemoteSubnet{{
			ProviderId:        "juju-subnet-12",
			CIDR:              "1.2.3.0/24",
			AvailabilityZones: []string{"az1", "az2"},
			ProviderSpaceId:   "juju-space-public",
			ProviderNetworkId: "network-1",
		}},
	}, {
		CloudType:  "ec2",
		Name:       "private",
		ProviderId: "juju-space-private",
		ProviderAttributes: map[string]interface{}{
			"thing1":  24,
			"thing2":  "bardiche",
			"network": "network-1",
		},
		Subnets: []state.RemoteSubnet{{
			ProviderId:        "juju-subnet-24",
			CIDR:              "1.2.4.0/24",
			AvailabilityZones: []string{"az1", "az2"},
			ProviderSpaceId:   "juju-space-private",
			ProviderNetworkId: "network-1",
		}},
	}})
}

func (s *remoteApplicationSuite) TestSpaceForEndpoint(c *tc.C) {
	space, ok := s.application.SpaceForEndpoint("db")
	c.Assert(ok, tc.IsTrue)
	c.Assert(space.Name, tc.Equals, "private")
	space, ok = s.application.SpaceForEndpoint("logging")
	c.Assert(ok, tc.IsTrue)
	c.Assert(space.Name, tc.Equals, "public")
	space, ok = s.application.SpaceForEndpoint("something else")
	c.Assert(ok, tc.IsFalse)
}

func (s *remoteApplicationSuite) TestBindings(c *tc.C) {
	c.Assert(s.application.Bindings(), tc.DeepEquals, map[string]string{
		"db":       "private",
		"db-admin": "private",
		"logging":  "public",
	})
}

func (s *remoteApplicationSuite) TestMysqlEndpoints(c *tc.C) {
	_, err := s.application.Endpoint("foo")
	c.Assert(err, tc.ErrorMatches, `saas application "mysql" has no "foo" relation`)

	serverEP, err := s.application.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverEP, tc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEp := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	loggingEp := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	eps, err := s.application.Endpoints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(eps, tc.DeepEquals, []state.Endpoint{serverEP, adminEp, loggingEp})
}

func (s *remoteApplicationSuite) TestMacaroon(c *tc.C) {
	mac, err := newMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	appMac, err := s.application.Macaroon()
	c.Assert(err, tc.ErrorIsNil)
	assertMacaroonEquals(c, appMac, mac)
}

func (s *remoteApplicationSuite) TestApplicationRefresh(c *tc.C) {
	s1, err := s.State.RemoteApplication(s.application.Name())
	c.Assert(err, tc.ErrorIsNil)

	err = s1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestAddRelationBothRemote(c *tc.C) {
	wpep := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "wordpress", Endpoints: wpep, SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": cannot add relation between saas applications "wordpress" and "mysql"`)
}

func (s *remoteApplicationSuite) TestInferEndpointsWrongScope(c *tc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
	_, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, tc.ErrorMatches, "no relations found")
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationErrors(c *tc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "haha/borken", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "haha/borken": name "haha/borken" not valid`)
	_, err = s.State.RemoteApplication("haha/borken")
	c.Assert(err, tc.ErrorMatches, `saas application name "haha/borken" not valid`)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "borken", URL: "haha/borken", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches,
		`cannot add saas application "borken": validating offer URL: `+
			`application offer URL is missing application`,
	)
	_, err = s.State.RemoteApplication("borken")
	c.Assert(err, tc.ErrorMatches, `saas application "borken" not found`)
}

func (s *remoteApplicationSuite) TestParamsValidateChecksBindings(c *tc.C) {
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}

	spaces := []*environs.ProviderSpaceInfo{{
		SpaceInfo: network.SpaceInfo{
			Name: "public",
		},
	}}
	bindings := map[string]string{
		"db": "private",
	}
	args := state.AddRemoteApplicationParams{
		Name:        "mysql",
		URL:         "me/model.mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
		Endpoints:   eps,
		Spaces:      spaces,
		Bindings:    bindings,
	}
	err := args.Validate()
	c.Assert(err, tc.ErrorMatches, `endpoint "db" bound to missing space "private" not valid`)
	bindings["db"] = "public"
	// Tolerates bindings for non-existent endpoints.
	bindings["gidget"] = "public"
	err = args.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationSuite) TestAddRemoteApplication(c *tc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", URL: "me/model.foo", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foo.Name(), tc.Equals, "foo")
	c.Assert(foo.IsConsumerProxy(), tc.IsFalse)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foo.Name(), tc.Equals, "foo")
	c.Assert(foo.OfferUUID(), tc.Equals, "offer-uuid")
	url, ok := foo.URL()
	c.Assert(ok, tc.IsTrue)
	c.Assert(url, tc.Equals, "me/model.foo")
	c.Assert(foo.IsConsumerProxy(), tc.IsFalse)
	c.Assert(foo.SourceModel().Id(), tc.Equals, s.Model.ModelTag().Id())
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationFromConsumer(c *tc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", SourceModel: s.Model.ModelTag(), IsConsumerProxy: true})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foo.IsConsumerProxy(), tc.IsTrue)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foo.Name(), tc.Equals, "foo")
	c.Assert(foo.IsConsumerProxy(), tc.IsTrue)
}

func (s *remoteApplicationSuite) TestSetSourceController(c *tc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = foo.SetSourceController("source-controller-uuid")
	c.Assert(err, tc.ErrorIsNil)

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		sourceCtrl := foo.SourceController()
		c.Assert(sourceCtrl, tc.Equals, "source-controller-uuid")

		err = foo.Refresh()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddEndpoints(c *tc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}

	err = foo.AddEndpoints(newEps)
	c.Assert(err, tc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range origEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(eps, tc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddEndpointsConflicting(c *tc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	newEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, tc.ErrorMatches, "endpoint ep1 already exists")
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentOneDeleted(c *tc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	reducedEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		// Destroy foo and recreate with fewer endpoints to simulate
		// endpoint removal.
		err := foo.Destroy()
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
			Endpoints: reducedEps,
		})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = foo.AddEndpoints(newEps)
	c.Assert(err, tc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range reducedEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(eps, tc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentConflictingOneAdded(c *tc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		newEps := []charm.Relation{
			{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		}
		app, err := s.State.RemoteApplication("foo")
		c.Assert(err, tc.ErrorIsNil)
		err = app.AddEndpoints(newEps)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, tc.ErrorMatches, "endpoint ep3 already exists")
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentDifferentOneAdded(c *tc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, tc.ErrorIsNil)

	concurrrentEps := []charm.Relation{
		{Name: "ep5", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		app, err := s.State.RemoteApplication("foo")
		c.Assert(err, tc.ErrorIsNil)
		err = app.AddEndpoints(concurrrentEps)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, tc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range origEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range concurrrentEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(eps, tc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddRemoteRelationWrongScope(c *tc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
	ep1 := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	ep2 := state.Endpoint{
		ApplicationName: "logging",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-client",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeContainer,
		},
	}
	_, err := s.State.AddRelation(ep1, ep2)
	c.Assert(err, tc.ErrorMatches, `cannot add relation "logging:logging-client mysql:logging": local endpoint must be globally scoped for remote relations`)
}

func (s *remoteApplicationSuite) TestAddRemoteRelationLocalFirst(c *tc.C) {
	s.assertAddRemoteRelation(c, "wordpress", "mysql")
}

func (s *remoteApplicationSuite) TestAddRemoteRelationRemoteFirst(c *tc.C) {
	s.assertAddRemoteRelation(c, "mysql", "wordpress")
}

func (s *remoteApplicationSuite) assertAddRemoteRelation(c *tc.C, application1, application2 string) {
	endpoints := map[string]state.Endpoint{
		"wordpress": {
			ApplicationName: "wordpress",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		},
		"mysql": {
			ApplicationName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints(application1, application2)
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.String(), tc.Equals, "wordpress:db mysql:db")
	c.Assert(rel.Endpoints(), tc.DeepEquals, []state.Endpoint{endpoints[application1], endpoints[application2]})
	remoteapp, err := s.State.RemoteApplication("mysql")
	c.Assert(err, tc.ErrorIsNil)
	relations, err := remoteapp.Relations()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.HasLen, 1)
	c.Assert(relations[0], tc.DeepEquals, rel)
}

func (s *remoteApplicationSuite) TestDestroySimple(c *tc.C) {
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Life(), tc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

}

func (s *remoteApplicationSuite) TestDestroyRemovesExternalController(c *tc.C) {
	ec := state.NewExternalControllers(s.State)
	_, err := ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(s.externalControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Life(), tc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = ec.Controller(s.externalControllerUUID)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyDoesNotRemoveExternalController(c *tc.C) {
	s.makeRemoteApplication(c, "mariadb", "user/model.mariadb")
	rc, err := state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc, tc.Equals, 2)

	ec := state.NewExternalControllers(s.State)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(s.externalControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Life(), tc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = ec.Controller(s.externalControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	rc, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc, tc.Equals, 1)
}

func (s *remoteApplicationSuite) TestDestroyWithRemovableRelation(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Refresh(), tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	// Destroy the remote application with no units in relation scope; check application and
	// unit removed.
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithRemoteTokens(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)

	// Add remote token so we can check it is cleaned up.
	re := s.State.RemoteEntities()
	relToken, err := re.ExportLocalEntity(rel.Tag())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.application.Refresh(), tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	_, err = re.GetToken(s.application.Tag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = re.GetToken(rel.Tag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = re.GetRemoteEntity("app-token")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = rel.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = re.GetRemoteEntity(relToken)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithOfferConnections(c *tc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Refresh(), tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	// Add a offer connection record so we can check it is cleaned up.
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: coretesting.ModelTag.Id(),
		RelationId:      rel.Id(),
		RelationKey:     rel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), tc.Equals, 1)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	rc, err = s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), tc.Equals, 0)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelation(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *tc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *remoteApplicationSuite) assertDestroyWithReferencedRelation(c *tc.C, refresh bool) {
	ch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingApplication(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	another := s.AddTestingApplication(c, "another", ch)
	eps, err = s.State.InferEndpoints("another", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(another.Refresh(), tc.ErrorIsNil)
	c.Assert(s.application.Refresh(), tc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.application.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel0.Life(), tc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are both removed.
	err = ru.LeaveScope()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyAlsoDeletesSecretConsumerInfo(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "another", ch)
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   app.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SaveSecretRemoteConsumer(uri, s.application.Tag(), &secrets.SecretConsumerMetadata{CurrentRevision: 666})
	c.Assert(err, tc.ErrorIsNil)

	unit := names.NewUnitTag(s.application.Name() + "/666")
	err = s.State.SaveSecretRemoteConsumer(uri, unit, &secrets.SecretConsumerMetadata{CurrentRevision: 667})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretRemoteConsumer(uri, s.application.Tag())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.GetSecretRemoteConsumer(uri, unit)
	c.Assert(err, tc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.GetSecretRemoteConsumer(uri, s.application.Tag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	_, err = s.State.GetSecretRemoteConsumer(uri, unit)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyAlsoDeletesSecretPermissions(c *tc.C) {
	wpEP := []charm.Relation{
		{Name: "db", Interface: "mysql", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal},
	}

	wp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "remote-wordpress", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints:       wpEP,
		IsConsumerProxy: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "mysqldb"})

	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, tc.ErrorIsNil)

	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(state.Endpoint{
		ApplicationName: "remote-wordpress",
		Relation:        wpEP[0],
	}, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       rel.Tag(),
		Subject:     wp.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, tc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, wp.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleView)

	_, err = wp.DestroyWithForce(true, time.Duration(0))
	c.Assert(err, tc.ErrorIsNil)
	access, err = s.State.SecretAccess(uri, wp.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, secrets.RoleNone)
}

func (s *remoteApplicationSuite) TestDestroyRemovesStatusHistory(c *tc.C) {
	err := s.application.SetStatus(status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, tc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 100}
	agentInfo, err := s.application.StatusHistory(filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(agentInfo), tc.Equals, 1)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	agentInfo, err = s.application.StatusHistory(filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agentInfo, tc.HasLen, 0)
}

func (s *remoteApplicationSuite) assertInScope(c *tc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.Equals, inScope)
}

func (s *remoteApplicationSuite) assertDestroyAppWithStatus(c *tc.C, appStatus *status.Status) {
	mysqlEP, err := s.application.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpUnit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wpEP, err := wordpress.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)

	rel, err := s.State.AddRelation(wpEP, mysqlEP)
	c.Assert(err, tc.ErrorIsNil)
	wpru, err := rel.Unit(wpUnit)
	c.Assert(err, tc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertInScope(c, wpru, true)

	mysqlru, err := rel.RemoteUnit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertInScope(c, mysqlru, true)

	c.Assert(s.application.Refresh(), tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	if appStatus != nil {
		err = s.application.SetStatus(status.StatusInfo{Status: *appStatus})
		c.Assert(err, tc.ErrorIsNil)
	}

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	if appStatus == nil || *appStatus != status.Terminated {
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(s.application.Life(), tc.Equals, state.Dying)
	} else {
		c.Assert(err, tc.Satisfies, errors.IsNotFound)
	}

	// If the remote app is terminated, any remote units are
	// forcibly removed from scope, but not local ones.
	s.assertInScope(c, mysqlru, appStatus == nil || *appStatus != status.Terminated)
	s.assertInScope(c, wpru, true)
}

func (s *remoteApplicationSuite) TestDestroyNoStatus(c *tc.C) {
	s.assertDestroyAppWithStatus(c, nil)
}

func (s *remoteApplicationSuite) TestDestroyNotTerminated(c *tc.C) {
	appStatus := status.Active
	s.assertDestroyAppWithStatus(c, &appStatus)
}

func (s *remoteApplicationSuite) TestDestroyTerminated(c *tc.C) {
	appStatus := status.Terminated
	s.assertDestroyAppWithStatus(c, &appStatus)
}

func (s *remoteApplicationSuite) TestDestroyTerminatedDead(c *tc.C) {
	err := s.application.SetStatus(status.StatusInfo{Status: status.Terminated})
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.SetDead()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestAllRemoteApplicationsNone(c *tc.C) {
	err := s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	applications, err := s.State.AllRemoteApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(applications), tc.Equals, 0)
}

func (s *remoteApplicationSuite) TestAllRemoteApplications(c *tc.C) {
	// There's initially the application created in test setup.
	applications, err := s.State.AllRemoteApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(applications), tc.Equals, 1)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "another", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	applications, err = s.State.AllRemoteApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(applications, tc.HasLen, 2)

	// Check the returned application, order is defined by sorted keys.
	names := make([]string, len(applications))
	for i, app := range applications {
		names[i] = app.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], tc.Equals, "another")
	c.Assert(names[1], tc.Equals, "mysql")
}

func (s *remoteApplicationSuite) TestAddApplicationModelDying(c *tc.C) {
	// Check that applications cannot be added if the model is initially Dying.
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": model is no longer alive`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameLocalExists(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationLocalAddedAfterInitial(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a local application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddApplication(state.AddApplicationArgs{
			Name: "s1", Charm: charm,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "20.04/stable",
			}},
		})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameRemoteExists(c *tc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": saas application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationRemoteAddedAfterInitial(c *tc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "s1", SourceModel: s.Model.ModelTag()})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": saas application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationModelDiesAfterInitial(c *tc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		model, err := s.State.Model()
		c.Assert(err, tc.ErrorIsNil)
		err = model.Destroy(state.DestroyModelParams{})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorMatches, `cannot add saas application "s1": model "testmodel" is dying`)
}

func (s *remoteApplicationSuite) TestWatchRemoteApplications(c *tc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql") // initial
	wc.AssertNoChange()

	db2, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "db2", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("db2")
	wc.AssertNoChange()

	err = db2.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = db2.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	wc.AssertChange("db2")
	wc.AssertNoChange()
}

func (s *remoteApplicationSuite) TestWatchRemoteApplicationsDying(c *tc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql") // initial
	wc.AssertNoChange()

	ch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingApplication(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.application.Refresh(), tc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), tc.ErrorIsNil)

	// Add a unit to the relation so the remote application is not
	// short-circuit removed.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, tc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertChange("mysql")
	wc.AssertNoChange()
}

func (s *remoteApplicationSuite) TestTerminateOperationLeavesScopes(c *tc.C) {
	ch := s.AddTestingCharm(c, "wordpress")

	_ = s.AddTestingApplication(c, "wp1", ch)
	eps1, err := s.State.InferEndpoints("wp1", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps1...)
	c.Assert(err, tc.ErrorIsNil)

	_ = s.AddTestingApplication(c, "wp2", ch)
	eps2, err := s.State.InferEndpoints("wp2", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel2, err := s.State.AddRelation(eps2...)
	c.Assert(err, tc.ErrorIsNil)

	ru1, err := rel1.RemoteUnit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	err = ru1.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	ru2, err := rel2.RemoteUnit("mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, tc.ErrorIsNil)

	op := s.application.TerminateOperation("do-do-do do-do-do do-do")
	err = s.State.ApplyOperation(op)
	c.Assert(err, tc.ErrorIsNil)

	err = s.application.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	appStatus, err := s.application.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appStatus.Status, tc.Equals, status.Terminated)
	c.Assert(appStatus.Message, tc.Equals, "do-do-do do-do-do do-do")
	c.Assert(s.application.Life(), tc.Equals, state.Dead)

	remoteRelUnits1, err := rel1.AllRemoteUnits("mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(remoteRelUnits1, tc.HasLen, 0)

	remoteRelUnits2, err := rel2.AllRemoteUnits("mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(remoteRelUnits2, tc.HasLen, 0)
}
