// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	tctesting "testing"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	mgotxn "github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	jujuversion "github.com/juju/juju/version"
)

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

// preventUnitDestroyRemove sets a non-allocating status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *tc.C, u *state.Unit) {
	// To have a non-allocating status, a unit needs to
	// be assigned to a machine.
	_, err := u.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		err = u.AssignToNewMachine()
	}
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, tc.ErrorIsNil)
}

type StateSuite struct {
	ConnSuite
}

func TestStateSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &StateSuite{})
}

func (s *StateSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *StateSuite) TestOpenController(c *tc.C) {
	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller.Close(), tc.IsNil)
}

func (s *StateSuite) TestOpenControllerTwice(c *tc.C) {
	for i := 0; i < 2; i++ {
		controller, err := state.OpenController(s.testOpenParams())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(controller.Close(), tc.IsNil)
	}
}

func (s *StateSuite) TestIsController(c *tc.C) {
	c.Assert(s.State.IsController(), tc.IsTrue)
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(st2.IsController(), tc.IsFalse)
}

func (s *StateSuite) TestControllerOwner(c *tc.C) {
	owner, err := s.State.ControllerOwner()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner, tc.Equals, s.Owner)

	// Check that other models return the same controller owner.
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	owner2, err := otherSt.ControllerOwner()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(owner2, tc.Equals, s.Owner)
}

func (s *StateSuite) TestUserModelNameIndex(c *tc.C) {
	index := state.UserModelNameIndex("BoB", "testing")
	c.Assert(index, tc.Equals, "bob:testing")
}

func (s *StateSuite) TestDocID(c *tc.C) {
	id := "wordpress"
	docID := state.DocID(s.State, id)
	c.Assert(docID, tc.Equals, s.State.ModelUUID()+":"+id)

	// Ensure that the prefix isn't added if it's already there.
	docID2 := state.DocID(s.State, docID)
	c.Assert(docID2, tc.Equals, docID)
}

func (s *StateSuite) TestLocalID(c *tc.C) {
	id := s.State.ModelUUID() + ":wordpress"
	localID := state.LocalID(s.State, id)
	c.Assert(localID, tc.Equals, "wordpress")
}

func (s *StateSuite) TestIDHelpersAreReversible(c *tc.C) {
	id := "wordpress"
	docID := state.DocID(s.State, id)
	localID := state.LocalID(s.State, docID)
	c.Assert(localID, tc.Equals, id)
}

func (s *StateSuite) TestStrictLocalID(c *tc.C) {
	id := state.DocID(s.State, "wordpress")
	localID, err := state.StrictLocalID(s.State, id)
	c.Assert(localID, tc.Equals, "wordpress")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestParseIDToTag(c *tc.C) {
	model := "42c4f770-86ed-4fcc-8e39-697063d082bc:e"
	machine := "42c4f770-86ed-4fcc-8e39-697063d082bc:m#0"
	application := "c9741ea1-0c2a-444d-82f5-787583a48557:a#mysql"
	unit := "c9741ea1-0c2a-444d-82f5-787583a48557:u#mysql/0"
	moTag := state.TagFromDocID(model)
	maTag := state.TagFromDocID(machine)
	unTag := state.TagFromDocID(unit)
	apTag := state.TagFromDocID(application)

	tag, err := names.ParseTag(moTag.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.String(), tc.Equals, moTag.String())

	tag, err = names.ParseTag(maTag.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.String(), tc.Equals, maTag.String())

	tag, err = names.ParseTag(unTag.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.String(), tc.Equals, unTag.String())

	tag, err = names.ParseTag(apTag.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.String(), tc.Equals, apTag.String())

	c.Assert(moTag.String(), tc.Equals, "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e")
	c.Assert(maTag.String(), tc.Equals, "machine-0")
	c.Assert(unTag.String(), tc.Equals, "unit-mysql-0")
	c.Assert(apTag.String(), tc.Equals, "application-mysql")
}

func (s *StateSuite) TestStrictLocalIDWithWrongPrefix(c *tc.C) {
	localID, err := state.StrictLocalID(s.State, "foo:wordpress")
	c.Assert(localID, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `unexpected id: "foo:wordpress"`)
}

func (s *StateSuite) TestStrictLocalIDWithNoPrefix(c *tc.C) {
	localID, err := state.StrictLocalID(s.State, "wordpress")
	c.Assert(localID, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `unexpected id: "wordpress"`)
}

func (s *StateSuite) TestOpenControllerRequiresExtantModelTag(c *tc.C) {
	uuid := utils.MustNewUUID()
	params := s.testOpenParams()
	params.ControllerModelTag = names.NewModelTag(uuid.String())
	controller, err := state.OpenController(params)
	if !c.Check(controller, tc.IsNil) {
		c.Check(controller.Close(), tc.ErrorIsNil)
	}
	expect := fmt.Sprintf("cannot read model %s: model %q not found", uuid, uuid)
	c.Check(err, tc.ErrorMatches, expect)
}

func (s *StateSuite) TestOpenControllerSetsModelTag(c *tc.C) {
	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, tc.ErrorIsNil)
	defer controller.Close()

	sysState, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := sysState.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(m.ModelTag(), tc.Equals, s.modelTag)
}

func (s *StateSuite) TestModelUUID(c *tc.C) {
	c.Assert(s.State.ModelUUID(), tc.Equals, s.modelTag.Id())
}

func (s *StateSuite) TestNoModelDocs(c *tc.C) {
	// For example:
	// found documents for model with uuid 7bfe98b6-7282-48d4-8e37-9b90fb3da4f1: 1 constraints doc, 1 modelusers doc, 1 settings doc, 1 statuses doc
	c.Assert(s.State.EnsureModelRemoved(), tc.ErrorMatches,
		fmt.Sprintf(`found documents for model with uuid %s: (\d+ [a-z]+ doc, )*\d+ [a-z]+ doc`, s.State.ModelUUID()))
}

func (s *StateSuite) TestMongoSession(c *tc.C) {
	session := s.State.MongoSession()
	c.Assert(session.Ping(), tc.IsNil)
}

type MultiModelStateSuite struct {
	ConnSuite
	OtherState *state.State
	OtherModel *state.Model
}

func (s *MultiModelStateSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.OtherState = s.Factory.MakeModel(c, nil)
	m, err := s.OtherState.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.OtherModel = m
}

func (s *MultiModelStateSuite) TearDownTest(c *tc.C) {
	if s.OtherState != nil {
		s.OtherState.Close()
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *MultiModelStateSuite) Reset(c *tc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func TestMultiModelStateSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &MultiModelStateSuite{})
}

func (s *MultiModelStateSuite) TestWatchTwoModels(c *tc.C) {
	for i, test := range []struct {
		about        string
		getWatcher   func(*state.State) interface{}
		setUpState   func(*state.State) (assertChanges bool)
		triggerEvent func(*state.State)
	}{
		{
			about: "machines",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchModelMachines()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), tc.Equals, "0")
			},
		},
		{
			about: "containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), tc.Equals, "0")
				return m.WatchAllContainers()
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				_, err = st.AddMachineInsideMachine(
					state.MachineTemplate{
						Base: state.UbuntuBase("22.04"),
						Jobs: []state.MachineJob{state.JobHostUnits},
					},
					m.Id(),
					instance.KVM,
				)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "lxd only containers",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), tc.Equals, "0")
				return m.WatchContainers(instance.LXD)
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				_, err = st.AddMachineInsideMachine(
					state.MachineTemplate{
						Base: state.UbuntuBase("22.04"),
						Jobs: []state.MachineJob{state.JobHostUnits},
					},
					m.Id(),
					instance.LXD,
				)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "units",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, nil)
				c.Assert(m.Id(), tc.Equals, "0")
				return m.WatchUnits()
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				f.MakeUnit(c, &factory.UnitParams{Machine: m})
			},
		}, {
			about: "applications",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchApplications()
			},
			triggerEvent: func(st *state.State) {
				f := factory.NewFactory(st, s.StatePool)
				f.MakeApplication(c, nil)
			},
		}, {
			about: "remote applications",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchRemoteApplications()
			},
			triggerEvent: func(st *state.State) {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "db2", SourceModel: s.Model.ModelTag()})
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "relations",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				wordpress := f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return wordpress.WatchRelations()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, tc.ErrorIsNil)
				_, err = st.AddRelation(eps...)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "remote relations",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchRemoteRelations()
			},
			setUpState: func(st *state.State) bool {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, tc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, tc.ErrorIsNil)
				_, err = st.AddRelation(eps...)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "relation ingress networks",
			getWatcher: func(st *state.State) interface{} {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, tc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, tc.ErrorIsNil)
				rel, err := st.AddRelation(eps...)
				c.Assert(err, tc.ErrorIsNil)
				return rel.WatchRelationIngressNetworks()
			},
			triggerEvent: func(st *state.State) {
				relIngress := state.NewRelationIngressNetworks(st)
				_, err := relIngress.Save("wordpress:db mysql:database", false, []string{"1.2.3.4/32", "4.3.2.1/16"})
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "relation egress networks",
			getWatcher: func(st *state.State) interface{} {
				_, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
					Name: "mysql", SourceModel: s.OtherModel.ModelTag(),
					Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
				})
				c.Assert(err, tc.ErrorIsNil)
				f := factory.NewFactory(st, s.StatePool)
				wpCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, tc.ErrorIsNil)
				rel, err := st.AddRelation(eps...)
				c.Assert(err, tc.ErrorIsNil)
				return rel.WatchRelationEgressNetworks()
			},
			triggerEvent: func(st *state.State) {
				relIngress := state.NewRelationEgressNetworks(st)
				_, err := relIngress.Save("wordpress:db mysql:database", false, []string{"1.2.3.4/32", "4.3.2.1/16"})
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "open ports",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchOpenedPorts()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				mysql := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql"})
				f.MakeUnit(c, &factory.UnitParams{Application: mysql})
				return false
			},
			triggerEvent: func(st *state.State) {
				u, err := st.Unit("mysql/0")
				c.Assert(err, tc.ErrorIsNil)

				unitPortRange, err := u.OpenedPortRanges()
				c.Assert(err, tc.ErrorIsNil)
				unitPortRange.Open(allEndpoints, network.MustParsePortRange("100-200/tcp"))
				c.Assert(st.ApplyOperation(unitPortRange.Changes()), tc.ErrorIsNil)
			},
		}, {
			about: "cleanups",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchCleanups()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})

				// add and destroy a relation, so there is something to cleanup.
				eps, err := st.InferEndpoints("wordpress", "mysql")
				c.Assert(err, tc.ErrorIsNil)
				r := f.MakeRelation(c, &factory.RelationParams{Endpoints: eps})
				loggo.GetLogger("juju.state").SetLogLevel(loggo.TRACE)
				err = r.Destroy()
				c.Assert(err, tc.ErrorIsNil)
				loggo.GetLogger("juju.state").SetLogLevel(loggo.DEBUG)
				return true
			},
			triggerEvent: func(st *state.State) {
				loggo.GetLogger("juju.state").SetLogLevel(loggo.TRACE)
				err := st.Cleanup(fakeSecretDeleter)
				c.Assert(err, tc.ErrorIsNil)
				loggo.GetLogger("juju.state").SetLogLevel(loggo.DEBUG)
			},
		}, {
			about: "reboots",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), tc.Equals, "0")
				w := m.WatchForRebootEvent()
				return w
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				err = m.SetRebootFlag(true)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "block devices",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				m := f.MakeMachine(c, &factory.MachineParams{})
				c.Assert(m.Id(), tc.Equals, "0")
				sb, err := state.NewStorageBackend(st)
				c.Assert(err, tc.ErrorIsNil)
				return sb.WatchBlockDevices(m.MachineTag())
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
				err = m.SetMachineBlockDevices(sdb)
				c.Assert(err, tc.ErrorIsNil)
				return true
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				sdb := state.BlockDeviceInfo{DeviceName: "sdb", Label: "fatty"}
				err = m.SetMachineBlockDevices(sdb)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "statuses",
			getWatcher: func(st *state.State) interface{} {
				m, err := st.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(m.Id(), tc.Equals, "0")
				// Ensure that all the creation events have flowed through the system.
				s.WaitForModelWatchersIdle(c, st.ModelUUID())
				return m.Watch()
			},
			setUpState: func(st *state.State) bool {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)
				m.SetProvisioned("inst-id", "", "fake_nonce", nil)
				return false
			},
			triggerEvent: func(st *state.State) {
				m, err := st.Machine("0")
				c.Assert(err, tc.ErrorIsNil)

				now := time.Now()
				sInfo := status.StatusInfo{
					Status:  status.Error,
					Message: "some status",
					Since:   &now,
				}
				err = m.SetStatus(sInfo)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "settings",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchApplications()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				app, err := st.Application("wordpress")
				c.Assert(err, tc.ErrorIsNil)

				err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "awesome"})
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "action status",
			getWatcher: func(st *state.State) interface{} {
				f := factory.NewFactory(st, s.StatePool)
				dummyCharm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
				application := f.MakeApplication(c, &factory.ApplicationParams{Name: "dummy", Charm: dummyCharm})

				unit, err := application.AddUnit(state.AddUnitParams{})
				c.Assert(err, tc.ErrorIsNil)
				return unit.WatchPendingActionNotifications()
			},
			triggerEvent: func(st *state.State) {
				unit, err := st.Unit("dummy/0")
				c.Assert(err, tc.ErrorIsNil)
				m, err := st.Model()
				c.Assert(err, tc.ErrorIsNil)
				operationID, err := m.EnqueueOperation("a test", 1)
				c.Assert(err, tc.ErrorIsNil)
				_, err = m.AddAction(unit, operationID, "snapshot", nil, nil, nil)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "min units",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchMinUnits()
			},
			setUpState: func(st *state.State) bool {
				f := factory.NewFactory(st, s.StatePool)
				wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
				_ = f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})
				return false
			},
			triggerEvent: func(st *state.State) {
				wordpress, err := st.Application("wordpress")
				c.Assert(err, tc.ErrorIsNil)
				err = wordpress.SetMinUnits(2)
				c.Assert(err, tc.ErrorIsNil)
			},
		}, {
			about: "subnets",
			getWatcher: func(st *state.State) interface{} {
				return st.WatchSubnets(nil)
			},
			triggerEvent: func(st *state.State) {
				_, err := st.AddSubnet(network.SubnetInfo{
					CIDR: "10.0.0.0/24",
				})
				c.Assert(err, tc.ErrorIsNil)
			},
		},
	} {
		c.Logf("Test %d: %s", i, test.about)
		func() {
			getTestWatcher := func(st *state.State) TestWatcherC {
				var wc interface{}
				switch w := test.getWatcher(st).(type) {
				case statetesting.StringsWatcher:
					wc = statetesting.NewStringsWatcherC(c, w)
					swc := wc.(statetesting.StringsWatcherC)
					// consume initial event
					swc.AssertChange()
					swc.AssertNoChange()
				case statetesting.NotifyWatcher:
					wc = statetesting.NewNotifyWatcherC(c, w)
					nwc := wc.(statetesting.NotifyWatcherC)
					// consume initial event
					nwc.AssertOneChange()
					nwc.AssertNoChange()
				default:
					c.Fatalf("unknown watcher type %T", w)
				}
				return TestWatcherC{
					c:       c,
					State:   st,
					Watcher: wc,
				}
			}

			checkIsolationForModel := func(w1, w2 TestWatcherC) {
				c.Logf("Making changes to model %s", w1.State.ModelUUID())
				// switch on type of watcher here
				if test.setUpState != nil {

					assertChanges := test.setUpState(w1.State)
					if assertChanges {
						// Consume events from setup.
						w1.AssertChanges()
						w1.AssertNoChange()
						w2.AssertNoChange()
					}
				}
				c.Logf("triggering event")
				test.triggerEvent(w1.State)
				w1.AssertChanges()
				w1.AssertNoChange()
				w2.AssertNoChange()
			}

			wc1 := getTestWatcher(s.State)
			defer wc1.Stop()
			wc2 := getTestWatcher(s.OtherState)
			defer wc2.Stop()
			wc2.AssertNoChange()
			wc1.AssertNoChange()
			checkIsolationForModel(wc1, wc2)
			checkIsolationForModel(wc2, wc1)
		}()
		s.Reset(c)
	}
}

type TestWatcherC struct {
	c       *tc.C
	State   *state.State
	Watcher interface{}
}

func (tw *TestWatcherC) AssertChanges() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		wc.AssertChanges()
	case statetesting.NotifyWatcherC:
		wc.AssertOneChange()
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (tw *TestWatcherC) AssertNoChange() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		wc.AssertNoChange()
	case statetesting.NotifyWatcherC:
		wc.AssertNoChange()
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (tw *TestWatcherC) Stop() {
	switch wc := tw.Watcher.(type) {
	case statetesting.StringsWatcherC:
		statetesting.AssertStop(tw.c, wc.Watcher)
	case statetesting.NotifyWatcherC:
		statetesting.AssertStop(tw.c, wc.Watcher)
	default:
		tw.c.Fatalf("unknown watcher type %T", wc)
	}
}

func (s *StateSuite) TestAddresses(c *tc.C) {
	var err error
	machines := make([]*state.Machine, 4)
	machines[0], err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	machines[1], err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	node, err := s.State.ControllerNode(machines[0].Id())
	c.Assert(err, tc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, tc.ErrorIsNil)

	changes, err := s.State.EnableHA(3, constraints.Value{}, state.UbuntuBase("12.10"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(changes.Added, tc.DeepEquals, []string{"2", "3"})
	c.Assert(changes.Maintained, tc.DeepEquals, []string{machines[0].Id()})

	machines[2], err = s.State.Machine("2")
	c.Assert(err, tc.ErrorIsNil)
	machines[3], err = s.State.Machine("3")
	c.Assert(err, tc.ErrorIsNil)

	for i, m := range machines {
		err := m.SetProviderAddresses(
			network.NewSpaceAddress(fmt.Sprintf("10.0.0.%d", i), network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("::1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			network.NewSpaceAddress("5.4.3.2", network.WithScope(network.ScopePublic)),
		)
		c.Assert(err, tc.ErrorIsNil)
	}
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := s.State.Addresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 3)
	c.Assert(addrs, tc.SameContents, []string{
		fmt.Sprintf("10.0.0.0:%d", cfg.StatePort()),
		fmt.Sprintf("10.0.0.2:%d", cfg.StatePort()),
		fmt.Sprintf("10.0.0.3:%d", cfg.StatePort()),
	})
}

func (s *StateSuite) TestPing(c *tc.C) {
	c.Assert(s.State.Ping(), tc.IsNil)
	mgotesting.MgoServer.Restart()
	c.Assert(s.State.Ping(), tc.NotNil)
}

func (s *StateSuite) TestIsNotFound(c *tc.C) {
	err1 := fmt.Errorf("unrelated error")
	err2 := errors.NotFoundf("foo")
	c.Assert(err1, tc.Not(tc.Satisfies), errors.IsNotFound)
	c.Assert(err2, tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) AssertMachineCount(c *tc.C, expect int) {
	ms, err := s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(ms), tc.Equals, expect)
}

var jobStringTests = []struct {
	job state.MachineJob
	s   string
}{
	{state.JobHostUnits, "JobHostUnits"},
	{state.JobManageModel, "JobManageModel"},
	{0, "<unknown job 0>"},
	{5, "<unknown job 5>"},
}

func (s *StateSuite) TestJobString(c *tc.C) {
	for _, t := range jobStringTests {
		c.Check(t.job.String(), tc.Equals, t.s)
	}
}

func (s *StateSuite) TestAddMachineErrors(c *tc.C) {
	_, err := s.State.AddMachine(state.Base{})
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: no base specified")
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"))
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: no jobs specified")
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits, state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: duplicate job: .*")
}

func (s *StateSuite) TestAddMachine(c *tc.C) {
	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageModel,
	}
	m0, err := s.State.AddMachine(state.UbuntuBase("12.10"), allJobs...)
	c.Assert(err, tc.ErrorIsNil)
	check := func(m *state.Machine, id string, base state.Base, jobs []state.MachineJob) {
		c.Assert(m.Id(), tc.Equals, id)
		c.Assert(m.Base().String(), tc.Equals, base.String())
		c.Assert(m.Jobs(), tc.DeepEquals, jobs)
		s.assertMachineContainers(c, m, nil)
	}
	check(m0, "0", state.UbuntuBase("12.10"), allJobs)
	m0, err = s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	check(m0, "0", state.UbuntuBase("12.10"), allJobs)

	oneJob := []state.MachineJob{state.JobHostUnits}
	m1, err := s.State.AddMachine(state.UbuntuBase("22.04"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	check(m1, "1", state.UbuntuBase("22.04"), oneJob)

	m1, err = s.State.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	check(m1, "1", state.UbuntuBase("22.04"), oneJob)

	m, err := s.State.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.HasLen, 2)
	check(m[0], "0", state.UbuntuBase("12.10"), allJobs)
	check(m[1], "1", state.UbuntuBase("22.04"), oneJob)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	_, err = st2.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: controller jobs specified but not allowed")
}

func (s *StateSuite) TestAddMachines(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")
	machineTemplate := state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		DisplayName:             "test-display-name",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), tc.IsTrue)
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, cons)
	mhc, err := m.HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*mhc, tc.DeepEquals, hc)
	instId, instDN, err := m.InstanceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(instId), tc.Equals, "inst-id")
	c.Assert(instDN, tc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachinesModelDying(c *tc.C) {
	err := s.Model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, `cannot add a new machine: model "testmodel" is dying`)
}

func (s *StateSuite) TestAddMachinesModelDyingAfterInitial(c *tc.C) {
	// Check that machines cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Life(), tc.Equals, state.Alive)
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), tc.IsNil)
	}).Check()
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, `cannot add a new machine: model "testmodel" is dying`)
}

func (s *StateSuite) TestAddMachinesModelMigrating(c *tc.C) {
	err := s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)
	// Check that machines cannot be added if the model is initially Dying.
	_, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, `cannot add a new machine: model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *tc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, tc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cores=4")
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		DisplayName: "test-display-name",
		Base:        state.UbuntuBase("12.10"),
		Constraints: extraCons,
		Jobs:        oneJob,
		Nonce:       "nonce",
		InstanceId:  "inst-id",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, expectedCons)
	m, err = s.State.Machine(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), tc.IsTrue)
	_, instDN, err := m.InstanceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instDN, tc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachinePlacementIgnoresModelConstraints(c *tc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=4G tags=foo"))
	c.Assert(err, tc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		DisplayName: "test-display-name",
		Base:        state.UbuntuBase("12.10"),
		Jobs:        oneJob,
		Placement:   "theplacement",
		Nonce:       "nonce",
		InstanceId:  "inst-id",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.Placement(), tc.Equals, "theplacement")
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("")
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mcons, tc.DeepEquals, expectedCons)
	m, err = s.State.Machine(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.CheckProvisioned("nonce"), tc.IsTrue)
	_, instDN, err := m.InstanceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instDN, tc.Equals, "test-display-name")
}

func (s *StateSuite) TestAddMachineWithVolumes(c *tc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, tc.ErrorIsNil)

	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")
	hc := instance.MustParseHardware("mem=2G")

	volume0 := state.VolumeParams{
		Pool: "loop-pool",
		Size: 123,
	}
	volume1 := state.VolumeParams{
		Pool: "", // use default
		Size: 456,
	}
	volumeAttachment0 := state.VolumeAttachmentParams{}
	volumeAttachment1 := state.VolumeAttachmentParams{
		ReadOnly: true,
	}

	machineTemplate := state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Constraints:             cons,
		HardwareCharacteristics: hc,
		InstanceId:              "inst-id",
		DisplayName:             "test-display-name",
		Nonce:                   "nonce",
		Jobs:                    oneJob,
		Volumes: []state.HostVolumeParams{{
			volume0, volumeAttachment0,
		}, {
			volume1, volumeAttachment1,
		}},
	}
	machines, err := s.State.AddMachines(machineTemplate)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	m, err := s.State.Machine(machines[0].Id())
	c.Assert(err, tc.ErrorIsNil)

	// When adding the machine, the default pool should
	// have been set on the volume params.
	machineTemplate.Volumes[1].Volume.Pool = "loop"

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	volumeAttachments, err := sb.MachineVolumeAttachments(m.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeAttachments, tc.HasLen, 2)
	if volumeAttachments[0].Volume() == names.NewVolumeTag(m.Id()+"/1") {
		va := volumeAttachments
		va[0], va[1] = va[1], va[0]
	}
	for i, att := range volumeAttachments {
		_, err = att.Info()
		c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
		attachmentParams, ok := att.Params()
		c.Assert(ok, tc.IsTrue)
		c.Check(attachmentParams, tc.Equals, machineTemplate.Volumes[i].Attachment)
		volume, err := sb.Volume(att.Volume())
		c.Assert(err, tc.ErrorIsNil)
		_, err = volume.Info()
		c.Assert(err, tc.Satisfies, errors.IsNotProvisioned)
		volumeParams, ok := volume.Params()
		c.Assert(ok, tc.IsTrue)
		c.Check(volumeParams, tc.Equals, machineTemplate.Volumes[i].Volume)
	}
	instId, instDN, err := m.InstanceNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(instId), tc.Equals, "inst-id")
	c.Assert(instDN, tc.Equals, "test-display-name")
}

func (s *StateSuite) assertMachineContainers(c *tc.C, m *state.Machine, containers []string) {
	mc, err := m.Containers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mc, tc.DeepEquals, containers)
}

func (s *StateSuite) TestAddContainerToNewMachine(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: oneJob,
	}
	parentTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("20.04"),
		Jobs: oneJob,
	}
	m, err := s.State.AddMachineInsideNewMachine(template, parentTemplate, instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0/lxd/0")
	c.Assert(m.Base().DisplayString(), tc.Equals, "ubuntu@12.10")
	c.Assert(m.ContainerType(), tc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&mcons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineContainers(c, m, []string{"0/lxd/0"})
	c.Assert(m.Base().DisplayString(), tc.Equals, "ubuntu@20.04")

	m, err = s.State.Machine("0/lxd/0")
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
}

func (s *StateSuite) TestAddContainerToExistingMachine(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	m1, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)

	// Add first container.
	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "1/lxd/0")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), tc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&mcons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0"})

	s.assertMachineContainers(c, m0, nil)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0"})
	m, err = s.State.Machine("1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)
	s.assertMachineContainers(c, m, nil)

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "1", instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "1/lxd/1")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), tc.Equals, instance.LXD)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxd/0", "1/lxd/1"})
}

func (s *StateSuite) TestAddContainerToMachineWithKnownSupportedContainers(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, tc.ErrorIsNil)

	m, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.KVM)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0/kvm/0")
	s.assertMachineContainers(c, host, []string{"0/kvm/0"})
}

func (s *StateSuite) TestAddInvalidContainerToMachineWithKnownSupportedContainers(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	err = host.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxd containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestAddContainerToMachineSupportingNoContainers(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	err = host.SupportsNoContainers()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: machine 0 cannot host lxd containers")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestAddContainerToMachineLockedForSeriesUpgrade(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	host, err := s.State.AddMachine(state.UbuntuBase("12.10"), oneJob...)
	c.Assert(err, tc.ErrorIsNil)
	err = host.CreateUpgradeSeriesLock(nil, state.UbuntuBase("18.04"))
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}, "0", instance.LXD)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: machine 0 is locked for series upgrade")
	s.assertMachineContainers(c, host, nil)
}

func (s *StateSuite) TestInvalidAddMachineParams(c *tc.C) {
	instIdTemplate := state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "i-foo",
	}
	normalTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(instIdTemplate, "0", instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(instIdTemplate, normalTemplate, instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, instIdTemplate, instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: cannot specify instance id for a new container")

	_, err = s.State.AddOneMachine(instIdTemplate)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")

	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
		InstanceId: "i-foo",
		Nonce:      "nonce",
	})
	c.Check(err, tc.ErrorMatches, fmt.Sprintf("cannot add a new machine: duplicate job: %s", state.JobHostUnits))

	noSeriesTemplate := state.MachineTemplate{
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobHostUnits},
	}
	_, err = s.State.AddOneMachine(noSeriesTemplate)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideNewMachine(noSeriesTemplate, normalTemplate, instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideNewMachine(normalTemplate, noSeriesTemplate, instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: no base specified")

	_, err = s.State.AddMachineInsideMachine(noSeriesTemplate, "0", instance.LXD)
	c.Check(err, tc.ErrorMatches, "cannot add a new machine: no base specified")
}

func (s *StateSuite) TestAddContainerErrors(c *tc.C) {
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideMachine(template, "10", instance.LXD)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: machine 10 not found")
	_, err = s.State.AddMachineInsideMachine(template, "10", "")
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: no container type specified")
}

func (s *StateSuite) TestInjectMachineErrors(c *tc.C) {
	injectMachine := func(base state.Base, instanceId instance.Id, nonce string, jobs ...state.MachineJob) error {
		_, err := s.State.AddOneMachine(state.MachineTemplate{
			Base:       base,
			Jobs:       jobs,
			InstanceId: instanceId,
			Nonce:      nonce,
		})
		return err
	}
	err := injectMachine(state.Base{}, "i-minvalid", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: no base specified")
	err = injectMachine(state.UbuntuBase("12.10"), "", agent.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: cannot specify a nonce without an instance id")
	err = injectMachine(state.UbuntuBase("12.10"), "i-minvalid", "", state.JobHostUnits)
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
	err = injectMachine(state.UbuntuBase("12.10"), agent.BootstrapNonce, "i-mlazy")
	c.Assert(err, tc.ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *tc.C) {
	cons := constraints.MustParse("mem=4G")
	arch := arch.DefaultArchitecture
	mem := uint64(1024)
	disk := uint64(1024)
	source := "loveshack"
	tags := []string{"foo", "bar"}
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits, state.JobManageModel},
		Constraints: cons,
		InstanceId:  "i-mindustrious",
		Nonce:       agent.BootstrapNonce,
		HardwareCharacteristics: instance.HardwareCharacteristics{
			Arch:           &arch,
			Mem:            &mem,
			RootDisk:       &disk,
			RootDiskSource: &source,
			Tags:           &tags,
		},
	}
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Jobs(), tc.DeepEquals, template.Jobs)
	instanceId, err := m.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instanceId, tc.Equals, template.InstanceId)
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, mcons)
	characteristics, err := m.HardwareCharacteristics()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*characteristics, tc.DeepEquals, template.HardwareCharacteristics)

	// Make sure the bootstrap nonce value is set.
	c.Assert(m.CheckProvisioned(template.Nonce), tc.IsTrue)
}

func (s *StateSuite) TestAddContainerToInjectedMachine(c *tc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	template := state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		InstanceId: "i-mindustrious",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	}
	m0, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)

	// Add first container.
	template = state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0/lxd/0")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), tc.Equals, instance.LXD)
	mcons, err := m.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&mcons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxd/0"})

	// Add second container.
	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0/lxd/1")
	c.Assert(m.Base().String(), tc.Equals, "ubuntu@12.10/stable")
	c.Assert(m.ContainerType(), tc.Equals, instance.LXD)
	c.Assert(m.Jobs(), tc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxd/0", "0/lxd/1"})
}

func (s *StateSuite) TestAddMachineCanOnlyAddControllerForMachine0(c *tc.C) {
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobManageModel},
	}
	// Check that we can add the bootstrap machine.
	m, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "0")
	node, err := s.State.ControllerNode(m.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.HasVote(), tc.IsFalse)
	c.Assert(m.Jobs(), tc.DeepEquals, []state.MachineJob{state.JobManageModel})

	// Check that the controller information is correct.
	controllerIds, err := s.State.ControllerIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerIds, tc.DeepEquals, []string{"0"})

	const errCannotAdd = "cannot add a new machine: controller jobs specified but not allowed"
	m, err = s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideMachine(template, "0", instance.LXD)
	c.Assert(err, tc.ErrorMatches, errCannotAdd)

	m, err = s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, tc.ErrorMatches, errCannotAdd)
}

func (s *StateSuite) TestReadMachine(c *tc.C) {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Id(), tc.Equals, expectedId)
}

func (s *StateSuite) TestMachineNotFound(c *tc.C) {
	_, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorMatches, "machine 0 not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestMachineIdLessThan(c *tc.C) {
	c.Assert(state.MachineIdLessThan("0", "0"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("0", "1"), tc.IsTrue)
	c.Assert(state.MachineIdLessThan("1", "0"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("10", "2"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("0", "0/lxd/0"), tc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxd/0", "0"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("1", "0/lxd/0"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/lxd/0", "1"), tc.IsTrue)
	c.Assert(state.MachineIdLessThan("0/lxd/0/lxd/1", "0/lxd/0"), tc.IsFalse)
	c.Assert(state.MachineIdLessThan("0/kvm/0", "0/lxd/0"), tc.IsTrue)
}

func (s *StateSuite) TestAllMachines(c *tc.C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, tc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "", "fake_nonce", nil)
		c.Assert(err, tc.ErrorIsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Assert(err, tc.ErrorIsNil)
		err = m.Destroy()
		c.Assert(err, tc.ErrorIsNil)
	}
	s.AssertMachineCount(c, numInserts)
	ms, _ := s.State.AllMachines()
	for i, m := range ms {
		c.Assert(m.Id(), tc.Equals, strconv.Itoa(i))
		instId, err := m.InstanceId()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(instId), tc.Equals, fmt.Sprintf("foo-%d", i))
		tools, err := m.AgentTools()
		c.Check(err, tc.ErrorIsNil)
		c.Check(tools.Version, tc.DeepEquals, version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Assert(m.Life(), tc.Equals, state.Dying)
	}
}

func (s *StateSuite) TestMachineCountForBase(c *tc.C) {
	add_machine := func(base state.Base) {
		m, err := s.State.AddMachine(base, state.JobHostUnits)
		c.Check(err, tc.ErrorIsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%s", base.String())), "", "fake_nonce", nil)
		c.Check(err, tc.ErrorIsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-ubuntu-amd64"))
		c.Check(err, tc.ErrorIsNil)
		err = m.Destroy()
		c.Check(err, tc.ErrorIsNil)
	}

	var windowsSeries = []string{
		"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2",
		"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
	}
	windowsBases := make([]state.Base, len(windowsSeries))
	for i, s := range windowsSeries {
		windowsBases[i] = state.Base{OS: "windows", Channel: s}
	}
	expectedWinResult := map[string]int{}
	for _, winBase := range windowsBases {
		add_machine(winBase)
		expectedWinResult[winBase.String()] = 1
	}
	add_machine(state.UbuntuBase("12.10"))
	s.AssertMachineCount(c, len(windowsSeries)+1)

	result, err := s.State.MachineCountForBase(windowsBases...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expectedWinResult)

	result, err = s.State.MachineCountForBase(
		state.UbuntuBase("12.10"), // count 1
		state.UbuntuBase("16.04"), // count 0
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]int{"ubuntu@12.10": 1})
}

func (s *StateSuite) TestInferActiveRelations(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("wp", "ms:prod")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	relation, err := s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relation, tc.Matches, "wp:db ms:prod")

	relation, err = s.State.InferActiveRelation("wp:db", "ms:prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relation, tc.Matches, "wp:db ms:prod")

	_, err = s.State.InferActiveRelation("wp", "ms:dev")
	c.Assert(err, tc.ErrorMatches, `relation matching "wp ms:dev" not found`)
}

func (s *StateSuite) TestInferActiveRelationsNoRelations(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, tc.ErrorMatches, `relation matching "wp ms" not found`)

	_, err = s.State.InferActiveRelation("wp:db", "ms:prod")
	c.Assert(err, tc.ErrorMatches, `relation matching "wp:db ms:prod" not found`)
}

func (s *StateSuite) TestInferActiveRelationsAmbiguous(c *tc.C) {
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	wp := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress-nolimit"))
	_, err = wp.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	ms := s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	_, err = ms.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	eps1, err := s.State.InferEndpoints("wp", "ms:prod")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps1...)
	c.Assert(err, tc.ErrorIsNil)

	eps2, err := s.State.InferEndpoints("wp", "ms:dev")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps2...)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.InferActiveRelation("wp", "ms")
	c.Assert(err, tc.ErrorMatches, `ambiguous relation: "wp ms" could refer to "wp:db ms:prod"; "wp:db ms:dev"`)

	relation, err := s.State.InferActiveRelation("wp", "ms:prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relation, tc.Matches, "wp:db ms:prod")
}

func (s *StateSuite) TestAllRelations(c *tc.C) {
	const numRelations = 32
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	_, err = mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	for i := 0; i < numRelations; i++ {
		applicationname := fmt.Sprintf("wordpress%d", i)
		wordpress := s.AddTestingApplication(c, applicationname, wordpressCharm)
		_, err = wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		eps, err := s.State.InferEndpoints(applicationname, "mysql")
		c.Assert(err, tc.ErrorIsNil)
		_, err = s.State.AddRelation(eps...)
		c.Assert(err, tc.ErrorIsNil)
	}

	relations, _ := s.State.AllRelations()

	c.Assert(len(relations), tc.Equals, numRelations)
	for i, relation := range relations {
		c.Assert(relation.Id(), tc.Equals, i)
		c.Assert(relation, tc.Matches, fmt.Sprintf("wordpress%d:.+ mysql:.+", i))
	}
}

func (s *StateSuite) TestAliveRelationKeys(c *tc.C) {
	const numRelations = 12
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	_, err = mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wordpressCharm := s.AddTestingCharm(c, "wordpress")
	for i := 0; i < numRelations; i++ {
		applicationname := fmt.Sprintf("wordpress%d", i)
		wordpress := s.AddTestingApplication(c, applicationname, wordpressCharm)
		_, err = wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, tc.ErrorIsNil)
		eps, err := s.State.InferEndpoints(applicationname, "mysql")
		c.Assert(err, tc.ErrorIsNil)
		r, err := s.State.AddRelation(eps...)
		c.Assert(err, tc.ErrorIsNil)
		// Destroy half the relations, to check we only get the ones Alive
		if i%2 == 0 {
			_ = r.Destroy()
		}
	}

	relationKeys := s.State.AliveRelationKeys()

	c.Assert(len(relationKeys), tc.Equals, numRelations/2)
	num := 1
	for _, relation := range relationKeys {
		c.Assert(relation, tc.Matches, fmt.Sprintf("wordpress%d:.+ mysql:.+", num))
		num += 2
	}
}

func (s *StateSuite) TestSaveCloudService(c *tc.C) {
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewSpaceAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svc.Refresh(), tc.ErrorIsNil)
	c.Assert(svc.Id(), tc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), tc.Equals, "provider-id")
	c.Assert(svc.Addresses(), tc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))

	getResult, err := s.State.CloudService("cloud-svc-ID")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(getResult.Id(), tc.Equals, "a#cloud-svc-ID")
	c.Assert(getResult.ProviderId(), tc.Equals, "provider-id")
	c.Assert(getResult.Addresses(), tc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))
}

func (s *StateSuite) TestSaveCloudServiceChangeAddressesAllGood(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id",
				Addresses:  network.NewSpaceAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id",
			Addresses:  network.NewSpaceAddresses("2.2.2.2"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svc.Refresh(), tc.ErrorIsNil)
	c.Assert(svc.Id(), tc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), tc.Equals, "provider-id")
	c.Assert(svc.Addresses(), tc.DeepEquals, network.NewSpaceAddresses("2.2.2.2"))
}

func (s *StateSuite) TestSaveCloudServiceChangeProviderId(c *tc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.SaveCloudService(
			state.SaveCloudServiceArgs{
				Id:         "cloud-svc-ID",
				ProviderId: "provider-id-existing",
				Addresses:  network.NewSpaceAddresses("1.1.1.1"),
			},
		)
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	svc, err := s.State.SaveCloudService(
		state.SaveCloudServiceArgs{
			Id:         "cloud-svc-ID",
			ProviderId: "provider-id-new", // ProviderId is immutable, changing this will get assert error.
			Addresses:  network.NewSpaceAddresses("1.1.1.1"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(svc.Refresh(), tc.ErrorIsNil)
	c.Assert(svc.Id(), tc.Equals, "a#cloud-svc-ID")
	c.Assert(svc.ProviderId(), tc.Equals, "provider-id-new")
	c.Assert(svc.Addresses(), tc.DeepEquals, network.NewSpaceAddresses("1.1.1.1"))
}

func (s *StateSuite) TestAddApplication(c *tc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "haha/borken", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "haha/borken": invalid name`)
	_, err = s.State.Application("haha/borken")
	c.Assert(err, tc.ErrorMatches, `"haha/borken" is not a valid application name`)

	// set that a nil charm is handled correctly
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "umadbro",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "umadbro": charm is nil`)

	// set that a nil charm origin is handled correctly
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  "umadbro",
		Charm: ch,
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "umadbro": charm origin is nil`)

	insettings := charm.Settings{"tuning": "optimized"}
	inconfig, err := coreconfig.NewConfig(coreconfig.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, tc.ErrorIsNil)

	wordpress, err := s.State.AddApplication(
		state.AddApplicationArgs{
			Name:              "wordpress",
			Charm:             ch,
			CharmConfig:       insettings,
			ApplicationConfig: inconfig,
			CharmOrigin: &state.CharmOrigin{
				ID:   "charmID",
				Hash: "testing-hash",
				Platform: &state.Platform{
					OS:      "ubuntu",
					Channel: "22.04/stable",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wordpress.Name(), tc.Equals, "wordpress")
	c.Assert(state.GetApplicationHasResources(wordpress), tc.IsFalse)
	outsettings, err := wordpress.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, tc.DeepEquals, expected)
	outconfig, err := wordpress.ApplicationConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(outconfig, tc.DeepEquals, inconfig.Attributes())
	cons, err := wordpress.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	a := arch.DefaultArchitecture
	c.Assert(cons, tc.DeepEquals, constraints.Value{
		Arch: &a,
	})

	mysqlArch := arch.ARM64
	mysql, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "mysql", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		Constraints: constraints.Value{Arch: &mysqlArch}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mysql.Name(), tc.Equals, "mysql")
	sInfo, err := mysql.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sInfo.Status, tc.Equals, status.Unset)
	c.Assert(sInfo.Message, tc.Equals, "")
	cons, err = mysql.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, constraints.Value{
		Arch: &mysqlArch,
	})

	// Check that retrieving the new created applications works correctly.
	wordpress, err = s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wordpress.Name(), tc.Equals, "wordpress")
	ch, _, err = wordpress.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, ch.URL())
	mysql, err = s.State.Application("mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mysql.Name(), tc.Equals, "mysql")
	ch, _, err = mysql.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, ch.URL())
}

func (s *StateSuite) TestAddApplicationFailCharmOriginIDOnly(c *tc.C) {
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        "testme",
		Charm:       &state.Charm{},
		CharmOrigin: &state.CharmOrigin{ID: "testing", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}},
	})
	c.Assert(err, tc.Satisfies, errors.IsBadRequest)
}

func (s *StateSuite) TestAddApplicationFailCharmOriginHashOnly(c *tc.C) {
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        "testme",
		Charm:       &state.Charm{},
		CharmOrigin: &state.CharmOrigin{Hash: "testing", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}},
	})
	c.Assert(err, tc.Satisfies, errors.IsBadRequest)
}

func (s *StateSuite) TestAddCAASApplication(c *tc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { _ = st.Close() }()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})

	insettings := charm.Settings{"tuning": "optimized"}
	inconfig, err := coreconfig.NewConfig(coreconfig.ConfigAttributes{"outlook": "good"}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, tc.ErrorIsNil)

	gitlab, err := st.AddApplication(
		state.AddApplicationArgs{
			Name: "gitlab", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			CharmConfig: insettings, ApplicationConfig: inconfig, NumUnits: 1,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gitlab.Name(), tc.Equals, "gitlab")
	c.Assert(gitlab.GetScale(), tc.Equals, 1)
	c.Assert(state.GetApplicationHasResources(gitlab), tc.IsTrue)
	outsettings, err := gitlab.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, tc.DeepEquals, expected)
	outconfig, err := gitlab.ApplicationConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(outconfig, tc.DeepEquals, inconfig.Attributes())

	cons, err := gitlab.Constraints()
	c.Assert(err, tc.ErrorIsNil)
	a := arch.DefaultArchitecture
	c.Assert(cons, tc.DeepEquals, constraints.Value{
		Arch: &a,
	})

	sInfo, err := gitlab.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sInfo.Status, tc.Equals, status.Unset)
	c.Assert(sInfo.Message, tc.Equals, "")

	// Check that retrieving the newly created application works correctly.
	gitlab, err = st.Application("gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gitlab.Name(), tc.Equals, "gitlab")
	ch, _, err = gitlab.Charm()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, ch.URL())
	units, err := gitlab.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 0)
}

func (s *StateSuite) TestAddApplicationKubernetesFormatV2(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer func() { _ = st.Close() }()
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	cockroach, err := st.AddApplication(state.AddApplicationArgs{
		Name: "mysql", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := cockroach.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 0)
}

func (s *StateSuite) TestAddApplicationKubernetesFormatV2SecondDeployUnitNumberStartFrom0(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer func() { _ = st.Close() }()
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	cockroach, err := st.AddApplication(state.AddApplicationArgs{
		Name: "cockroach", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err := cockroach.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(unitAssignments), tc.Equals, 0)

	err = cockroach.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = cockroach.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	assertCleanupCount(c, st, 2)

	ch = state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	cockroach, err = st.AddApplication(state.AddApplicationArgs{
		Name: "cockroach", Charm: ch, NumUnits: 1,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	units, err = cockroach.AllUnits()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(units), tc.Equals, 1)
	c.Assert(units[0].Name(), tc.Equals, `cockroach/0`)
}

func (s *StateSuite) TestAddCAASApplicationPlacementNotAllowed(c *tc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})

	placement := []*instance.Placement{instance.MustParsePlacement("#:2")}
	_, err := st.AddApplication(
		state.AddApplicationArgs{
			Name: "gitlab", Charm: ch,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			Placement: placement,
		})
	c.Assert(err, tc.ErrorMatches, ".*"+regexp.QuoteMeta(`cannot add application "gitlab": placement directives on k8s models not valid`))
}

func (s *StateSuite) TestAddApplicationWithNilCharmConfigValues(c *tc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	insettings := charm.Settings{"tuning": nil}

	wordpress, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		CharmConfig: insettings},
	)
	c.Assert(err, tc.ErrorIsNil)
	outsettings, err := wordpress.CharmConfig(model.GenerationMaster)
	c.Assert(err, tc.ErrorIsNil)
	expected := ch.Config().DefaultSettings()
	for name, value := range insettings {
		expected[name] = value
	}
	c.Assert(outsettings, tc.DeepEquals, expected)

	// Ensure that during creation, application settings with nil config values
	// were stripped and not written into database.
	dbSettings := state.GetApplicationCharmConfig(s.State, wordpress)
	_, dbFound := dbSettings.Get("tuning")
	c.Assert(dbFound, tc.IsFalse)
}

func (s *StateSuite) TestAddApplicationModelDying(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that applications cannot be added if the model is initially Dying.
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": model "testmodel" is dying`)
}

func (s *StateSuite) TestAddApplicationModelMigrating(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that applications cannot be added if the model is initially Dying.
	err := s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": model "testmodel" is being migrated`)
}

func (s *StateSuite) TestAddApplicationSameRemoteExists(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": saas application with same name already exists`)
}

func (s *StateSuite) TestAddApplicationRemoteAddedAfterInitial(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "s1", SourceModel: s.Model.ModelTag()})
		c.Assert(err, tc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": saas application with same name already exists`)
}

func (s *StateSuite) TestAddApplicationSameLocalExists(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s0", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s0": application already exists`)
}

func (s *StateSuite) TestAddApplicationLocalAddedAfterInitial(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a local application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.AddTestingApplication(c, "s1", charm)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": application already exists`)
}

func (s *StateSuite) TestAddApplicationModelDyingAfterInitial(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	s.AddTestingApplication(c, "s0", charm)
	// Check that applications cannot be added if the model is initially
	// Alive but set to Dying immediately before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Life(), tc.Equals, state.Alive)
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), tc.IsNil)
	}).Check()
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "s1": model "testmodel" is dying`)
}

func (s *StateSuite) TestApplicationNotFound(c *tc.C) {
	_, err := s.State.Application("bummer")
	c.Assert(err, tc.ErrorMatches, `application "bummer" not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestAddApplicationWithDefaultBindings(c *tc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := app.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings.Map(), tc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
	})

	// Removing the application also removes its bindings.
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, app)
}

func (s *StateSuite) TestAddApplicationWithSpecifiedBindings(c *tc.C) {
	// Add extra spaces to use in bindings.
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	clientSpace, err := s.State.AddSpace("client", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	// Specify some bindings, but not all when adding the application.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 43)
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
		EndpointBindings: map[string]string{
			"client":  clientSpace.Id(),
			"cluster": dbSpace.Id(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Read them back to verify defaults and given bindings got merged as
	// expected.
	bindings, err := app.EndpointBindings()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bindings.Map(), tc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId, // inherited from defaults.
		"client":  clientSpace.Id(),
		"cluster": dbSpace.Id(),
	})
}

func (s *StateSuite) TestAddApplicationWithInvalidBindings(c *tc.C) {
	charm := s.AddMetaCharm(c, "mysql", metaBase, 44)
	// Add extra spaces to use in bindings.
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)
	clientSpace, err := s.State.AddSpace("client", "", nil, true)
	c.Assert(err, tc.ErrorIsNil)

	for i, test := range []struct {
		about         string
		bindings      map[string]string
		expectedError string
		errorType     func(error) bool
	}{{ // 0
		about:         "extra endpoint bound to unknown space",
		bindings:      map[string]string{"extra": "4"},
		expectedError: `space not found`,
		errorType:     errors.IsNotFound,
	}, { // 1
		about:         "extra endpoint not bound to a space",
		bindings:      map[string]string{"extra": ""},
		expectedError: `unknown endpoint "extra" not valid`,
		errorType:     errors.IsNotValid,
	}, { // 2
		about:         "two extra endpoints, both bound to known spaces",
		bindings:      map[string]string{"ex1": dbSpace.Id(), "ex2": clientSpace.Id()},
		expectedError: `unknown endpoint "ex(1|2)" not valid`,
		errorType:     errors.IsNotValid,
	}, { // 3
		about:         "empty endpoint bound to unknown space",
		bindings:      map[string]string{"": "anything"},
		expectedError: `space not found`,
		errorType:     errors.IsNotFound,
	}, { // 4
		about:         "known endpoint bound to unknown space",
		bindings:      map[string]string{"server": "invalid"},
		expectedError: `space not found`,
		errorType:     errors.IsNotFound,
	}, { // 5
		about:         "known endpoint bound correctly and an extra endpoint",
		bindings:      map[string]string{"server": dbSpace.Id(), "foo": "public"},
		expectedError: `space not found`,
		errorType:     errors.IsNotFound,
	}} {
		c.Logf("test #%d: %s", i, test.about)

		_, err := s.State.AddApplication(state.AddApplicationArgs{
			Name:  "yoursql",
			Charm: charm,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			}},
			EndpointBindings: test.bindings,
		})
		c.Check(err, tc.ErrorMatches, `cannot add application "yoursql": `+test.expectedError)
		c.Check(err, tc.Satisfies, test.errorType)
	}
}

func (s *StateSuite) TestAddApplicationMachinePlacementInvalidSeries(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	charm := s.AddTestingCharm(c, "dummy")
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.04/stable",
		}},
		Placement: []*instance.Placement{
			{instance.MachineScope, m.Id()},
		},
	})
	c.Assert(err, tc.ErrorMatches, "cannot add application \"wordpress\": cannot deploy to machine .*: base does not match.*")
}

func (s *StateSuite) TestAddApplicationIncompatibleOSWithSeriesInURL(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "centos",
			Channel: "7/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "wordpress": OS "centos" not supported by charm "dummy", supported OSes are: ubuntu`)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithSeriesInURL(c *tc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	// A charm with a series in its URL is implicitly supported by that
	// series only.
	base, err := corebase.GetBaseFromSeries(charm.MustParseURL(ch.URL()).Series)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      base.OS,
			Channel: base.Channel.String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationCompatibleOSWithNoExplicitSupportedSeries(c *tc.C) {
	// If a charm doesn't declare any series, we can add it with any series we choose.
	charm := s.AddSeriesCharm(c, "dummy", "bionic")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestAddApplicationOSIncompatibleWithSupportedSeries(c *tc.C) {
	charm := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	// A charm with supported series can only be force-deployed to series
	// of the same operating systems as the supported series.
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "centos",
			Channel: "7/stable",
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot add application "wordpress": OS "centos" not supported by charm "multi-series", supported OSes are: ubuntu`)
}

func (s *StateSuite) TestAllApplications(c *tc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applications, err := s.State.AllApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(applications), tc.Equals, 0)

	// Check that after adding applications the result is ok.
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	applications, err = s.State.AllApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(applications), tc.Equals, 1)

	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name: "mysql", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	applications, err = s.State.AllApplications()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(applications, tc.HasLen, 2)

	// Check the returned application, order is defined by sorted keys.
	names := make([]string, len(applications))
	for i, app := range applications {
		names[i] = app.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], tc.Equals, "mysql")
	c.Assert(names[1], tc.Equals, "wordpress")
}

var inferEndpointsTests = []struct {
	summary string
	inputs  [][]string
	eps     []state.Endpoint
	err     string
}{
	{
		summary: "insane args",
		inputs:  [][]string{nil},
		err:     `cannot relate 0 endpoints`,
	}, {
		summary: "insane args",
		inputs:  [][]string{{"blah", "blur", "bleurgh"}},
		err:     `cannot relate 3 endpoints`,
	}, {
		summary: "invalid args",
		inputs: [][]string{
			{"ping:"},
			{":pong"},
			{":"},
		},
		err: `invalid endpoint ".*"`,
	}, {
		summary: "unknown application",
		inputs:  [][]string{{"wooble"}},
		err:     `application "wooble" not found`,
	}, {
		summary: "invalid relations",
		inputs: [][]string{
			{"ms", "ms"},
			{"wp", "wp"},
			{"rk1", "rk1"},
			{"rk1", "rk2"},
		},
		err: `no relations found`,
	}, {
		summary: "container scoped relation not possible when there's no subordinate",
		inputs: [][]string{
			{"lg-p", "wp"},
		},
		err: `no relations found`,
	}, {
		summary: "container scoped relations between 2 subordinates is ok",
		inputs:  [][]string{{"lg:logging-directory", "lg2:logging-client"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Name:      "logging-directory",
				Role:      "requirer",
				Interface: "logging",
				Scope:     charm.ScopeContainer,
			}}, {
			ApplicationName: "lg2",
			Relation: charm.Relation{
				Name:      "logging-client",
				Role:      "provider",
				Interface: "logging",
				Scope:     charm.ScopeGlobal,
			}},
		},
	},
	{
		summary: "valid peer relation",
		inputs: [][]string{
			{"rk1"},
			{"rk1:ring"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "rk1",
			Relation: charm.Relation{
				Name:      "ring",
				Interface: "riak",
				Role:      charm.RolePeer,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "ambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms", "wp"},
			{"ms", "wp:db"},
		},
		err: `ambiguous relation: ".*" could refer to "wp:db ms:dev"; "wp:db ms:prod"; "wp:db ms:test"`,
	}, {
		summary: "unambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms:dev", "wp"},
			{"ms:dev", "wp:db"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "ms",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "dev",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
				Limit:     2,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		}},
	}, {
		summary: "explicit logging relation is preferred over implicit juju-info",
		inputs:  [][]string{{"lg", "wp"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-directory",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-dir",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeContainer,
			},
		}},
	}, {
		summary: "implicit relations can be chosen explicitly",
		inputs: [][]string{
			{"lg:info", "wp"},
			{"lg", "wp:juju-info"},
			{"lg:info", "wp:juju-info"},
		},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
			},
		}, {
			ApplicationName: "wp",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "implicit relations will be chosen if there are no other options",
		inputs:  [][]string{{"lg", "ms"}},
		eps: []state.Endpoint{{
			ApplicationName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
			},
		}, {
			ApplicationName: "ms",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	},
}

func (s *StateSuite) TestInferEndpoints(c *tc.C) {
	s.AddTestingApplication(c, "ms", s.AddTestingCharm(c, "mysql-alternative"))
	s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	loggingCh := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "lg", loggingCh)
	s.AddTestingApplication(c, "lg2", loggingCh)
	riak := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "rk1", riak)
	s.AddTestingApplication(c, "rk2", riak)
	s.AddTestingApplication(c, "lg-p", s.AddTestingCharm(c, "logging-principal"))

	for i, t := range inferEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)
		for j, input := range t.inputs {
			c.Logf("  input %d: %+v", j, input)
			eps, err := s.State.InferEndpoints(input...)
			if t.err == "" {
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(eps, tc.DeepEquals, t.eps)
			} else {
				c.Assert(err, tc.ErrorMatches, t.err)
			}
		}
	}
}

func (s *StateSuite) TestModelConstraints(c *tc.C) {
	// Environ constraints start out empty (for now).
	cons, err := s.State.ModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(&cons, tc.Satisfies, constraints.IsEmpty)

	// Environ constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(1024)}
	err = s.State.SetModelConstraints(cons2)
	c.Assert(err, tc.ErrorIsNil)
	cons3, err := s.State.ModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons3, tc.DeepEquals, cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(250)}
	err = s.State.SetModelConstraints(cons4)
	c.Assert(err, tc.ErrorIsNil)
	cons5, err := s.State.ModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons5, tc.DeepEquals, cons4)
}

func (s *StateSuite) TestSetInvalidConstraints(c *tc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.State.SetModelConstraints(cons)
	c.Assert(err, tc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *StateSuite) TestSetUnsupportedConstraintsWarning(c *tc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("constraints-tester", tw), tc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.State.SetModelConstraints(cons)
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Assert(tw.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{{
		Level:   loggo.WARNING,
		Message: `setting model constraints: unsupported constraints: cpu-power`,
	}})
	econs, err := s.State.ModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(econs, tc.DeepEquals, cons)
}

func (s *StateSuite) TestWatchModelsBulkEvents(c *tc.C) {
	// Alive model...
	alive := s.Model

	// Dying model...
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	// Add a application so Destroy doesn't advance to Dead.
	app := factory.NewFactory(st1, s.StatePool).MakeApplication(c, nil)
	dying, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = dying.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Add an empty model, destroy and remove it; we should
	// never see it reported.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model2.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	err = st2.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	// All except the removed model are reported in initial event.
	w := s.State.WatchModels()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.UUID(), dying.UUID())

	// Progress dying to dead, alive to dying; and see changes reported.
	err = app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(alive.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(alive.Refresh(), tc.ErrorIsNil)
	c.Assert(alive.Life(), tc.Equals, state.Dying)
	c.Assert(dying.Refresh(), tc.Satisfies, errors.IsNotFound)
	wc.AssertChange(alive.UUID())
}

func (s *StateSuite) TestWatchModelsLifecycle(c *tc.C) {
	// Initial event reports the controller model.
	w := s.State.WatchModelLives()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(s.State.ModelUUID())
	wc.AssertNoChange()

	// Add a non-empty model: reported.
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	app := factory.NewFactory(st1, s.StatePool).MakeApplication(c, nil)
	model, err := st1.Model()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()

	// Remove the model: reported.
	c.Assert(app.Destroy(), tc.ErrorIsNil)
	c.Assert(st1.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(st1.RemoveDyingModel(), tc.ErrorIsNil)
	wc.AssertChange(model.UUID())
	wc.AssertNoChange()
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestWatchApplicationsBulkEvents(c *tc.C) {
	// Alive application...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive := s.AddTestingApplication(c, "application0", dummyCharm)

	// Dying application...
	dying := s.AddTestingApplication(c, "application1", dummyCharm)
	keepDying, err := dying.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// Dead application (actually, gone, Dead == removed in this case).
	gone := s.AddTestingApplication(c, "application2", dummyCharm)
	err = gone.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// All except gone are reported in initial event.
	w := s.State.WatchApplications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = keepDying.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchApplicationsLifecycle(c *tc.C) {
	// Initial event is empty when no applications.
	w := s.State.WatchApplications()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a application: reported.
	application := s.AddTestingApplication(c, "application", s.AddTestingCharm(c, "dummy"))
	wc.AssertChange("application")
	wc.AssertNoChange()

	// Change the application: not reported.
	keepDying, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	c.Assert(application.Destroy(), tc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()

	c.Assert(application.Refresh(), tc.ErrorIsNil)
	c.Check(application.Life(), tc.Equals, state.Dying)

	// Make it Dead(/removed): reported.
	c.Assert(keepDying.Destroy(), tc.ErrorIsNil)
	needs, err := s.State.NeedsCleanup()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needs, tc.IsTrue)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), tc.ErrorIsNil)
	wc.AssertChange("application")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchApplicationsDiesOnStateClose(c *tc.C) {
	// This test is testing logic in watcher.lifecycleWatcher,
	// which is also used by:
	//     State.WatchModels
	//     Application.WatchUnits
	//     Application.WatchRelations
	//     Machine.WatchContainers
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		w := st.WatchApplications()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchMachinesBulkEvents(c *tc.C) {
	// Alive machine...
	alive, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	// Dying machine...
	dying, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = dying.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = dying.Destroy()
	c.Assert(err, tc.ErrorIsNil)

	// Dead machine...
	dead, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)

	// Gone machine.
	gone, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = gone.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = gone.Remove()
	c.Assert(err, tc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	// All except gone machine are reported in initial event.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Id(), dying.Id(), dead.Id())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = dying.Remove()
	c.Assert(err, tc.ErrorIsNil)
	err = dead.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(alive.Id(), dying.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesLifecycle(c *tc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = machine.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Make it Dead: reported.
	err = machine.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Remove it: not reported.
	err = machine.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIncludesOldMachines(c *tc.C) {
	// Older versions of juju do not write the "containertype" field.
	// This has caused machines to not be detected in the initial event.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$unset", bson.D{{"containertype", 1}}}},
	)
	c.Assert(err, tc.ErrorIsNil)

	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(machine.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIgnoresContainers(c *tc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchModelMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	machines, err := s.State.AddMachines(template)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	machine := machines[0]
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Add a container: not reported.
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the container Dying: not reported.
	err = m.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the container Dead: not reported.
	err = m.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchContainerLifecycle(c *tc.C) {
	// Add a host machine.
	template := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)

	otherMachine, err := s.State.AddOneMachine(template)
	c.Assert(err, tc.ErrorIsNil)

	// Initial event is empty when no containers.
	w := machine.WatchContainers(instance.LXD)
	defer statetesting.AssertStop(c, w)
	wAll := machine.WatchAllContainers()
	defer statetesting.AssertStop(c, wAll)

	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	wcAll := statetesting.NewStringsWatcherC(c, wAll)
	wcAll.AssertChange()
	wcAll.AssertNoChange()

	// Add a container of the required type: reported.
	m, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Add a container of a different type: not reported.
	m1, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Add a nested container of the right type: not reported.
	mchild, err := s.State.AddMachineInsideMachine(template, m.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()

	// Add a container of a different machine: not reported.
	m2, err := s.State.AddMachineInsideMachine(template, otherMachine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	statetesting.AssertStop(c, w)
	wcAll.AssertNoChange()
	statetesting.AssertStop(c, wAll)

	w = machine.WatchContainers(instance.LXD)
	defer statetesting.AssertStop(c, w)
	wc = statetesting.NewStringsWatcherC(c, w)
	wAll = machine.WatchAllContainers()
	defer statetesting.AssertStop(c, wAll)
	wcAll = statetesting.NewStringsWatcherC(c, wAll)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/kvm/0", "0/lxd/0")
	wcAll.AssertNoChange()

	// Make the container Dying: cannot because of nested container.
	err = m.Destroy()
	c.Assert(err, tc.ErrorMatches, `machine .* is hosting containers? ".*"`)

	err = mchild.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = mchild.Remove()
	c.Assert(err, tc.ErrorIsNil)

	// Make the container Dying: reported.
	err = m.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the other containers Dying: not reported.
	err = m1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	err = m2.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Make the container Dead: reported.
	err = m.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("0/lxd/0")
	wc.AssertNoChange()
	wcAll.AssertChange("0/lxd/0")
	wcAll.AssertNoChange()

	// Make the other containers Dead: not reported.
	err = m1.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	err = m2.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	// But reported by the all watcher.
	wcAll.AssertChange("0/kvm/0")
	wcAll.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
	wcAll.AssertNoChange()
}

func (s *StateSuite) TestWatchMachineHardwareCharacteristics(c *tc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := machine.WatchInstanceData()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	vers := version.MustParseBinary("1.2.3-ubuntu-ppc")
	err = machine.SetAgentVersion(vers)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchControllerConfig(c *tc.C) {
	w := s.State.WatchControllerConfig()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	cfg, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	expectedCfg := testing.FakeControllerConfig()
	c.Assert(cfg, tc.DeepEquals, expectedCfg)

	settings := state.GetControllerSettings(s.State)
	settings.Set("model-logs-size", "5M")
	_, err = settings.Write()
	c.Assert(err, tc.ErrorIsNil)

	wc.AssertOneChange()

	cfg, err = s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	expectedCfg["model-logs-size"] = "5M"
	c.Assert(cfg, tc.DeepEquals, expectedCfg)
}

func (s *StateSuite) insertFakeModelDocs(c *tc.C, st *state.State) string {
	// insert one doc for each multiModelCollection
	var ops []mgotxn.Op
	modelUUID := st.ModelUUID()
	for _, collName := range state.MultiModelCollections() {
		// skip adding constraints, modelUser and settings as they were added when the
		// model was created
		if collName == "constraints" || collName == "modelusers" || collName == "settings" {
			continue
		}
		if state.HasRawAccess(collName) {
			coll, closer := state.GetRawCollection(st, collName)
			defer closer()

			err := coll.Insert(bson.M{
				"_id":        state.DocID(st, "arbitraryid"),
				"model-uuid": modelUUID,
			})
			c.Assert(err, tc.ErrorIsNil)
		} else {
			doc := bson.M{
				"model-uuid": modelUUID,
			}
			id := "arbitraryid"
			// We need a "real" application and offer.
			if collName == "applicationOffers" {
				doc["application-name"] = "foo"
			} else if collName == "applications" {
				doc["name"] = "foo"
				id = "foo"
			}
			ops = append(ops, mgotxn.Op{
				C:      collName,
				Id:     state.DocID(st, id),
				Insert: doc,
			})
		}
	}

	state.RunTransaction(c, st, ops)

	// test that we can find each doc in state
	for _, collName := range state.MultiModelCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"model-uuid", st.ModelUUID()}}).Count()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Not(tc.Equals), 0)
	}

	// Add a model user whose permissions should get removed
	// when the model is.
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("amelia@external"),
			CreatedBy: s.Owner,
			Access:    permission.ReadAccess,
		})
	c.Assert(err, tc.ErrorIsNil)

	return state.UserModelNameIndex(s.Model.Owner().Id(), s.Model.Name())
}

type checkUserModelNameArgs struct {
	st     *state.State
	id     string
	exists bool
}

func (s *StateSuite) checkUserModelNameExists(c *tc.C, args checkUserModelNameArgs) {
	indexColl, closer := state.GetCollection(args.st, "usermodelname")
	defer closer()
	n, err := indexColl.FindId(args.id).Count()
	c.Assert(err, tc.ErrorIsNil)
	if args.exists {
		c.Assert(n, tc.Equals, 1)
	} else {
		c.Assert(n, tc.Equals, 0)
	}
}

func (s *StateSuite) AssertModelDeleted(c *tc.C, st *state.State) {
	// check to see if the model itself is gone
	_, err := st.Model()
	c.Assert(err, tc.ErrorMatches, `model "`+st.ModelUUID()+`" not found`)

	// ensure all docs for all MultiModelCollections are removed
	for _, collName := range state.MultiModelCollections() {
		coll, closer := state.GetRawCollection(st, collName)
		defer closer()
		n, err := coll.Find(bson.D{{"model-uuid", st.ModelUUID()}}).Count()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Equals, 0)
	}

	// ensure user permissions for the model are removed
	permPattern := fmt.Sprintf("^%s#%s#", state.ModelGlobalKey, st.ModelUUID())
	permissions, closer := state.GetCollection(st, "permissions")
	defer closer()
	permCount, err := permissions.Find(bson.M{"_id": bson.M{"$regex": permPattern}}).Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(permCount, tc.Equals, 0)
}

func (s *StateSuite) TestRemoveModel(c *tc.C) {
	st := s.State

	userModelKey := s.insertFakeModelDocs(c, st)
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: true})

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetDead()
	c.Assert(err, tc.ErrorIsNil)

	cloud, err := s.State.Cloud(model.CloudName())
	c.Assert(err, tc.ErrorIsNil)
	refCount, err := state.CloudModelRefCount(st, cloud.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 1)

	err = st.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	cloud, err = s.State.Cloud(model.CloudName())
	c.Assert(err, tc.ErrorIsNil)
	_, err = state.CloudModelRefCount(st, cloud.Name)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
}

func (s *StateSuite) TestRemoveDyingModelAliveModelFails(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	err := st.RemoveDyingModel()
	c.Assert(errors.Cause(err), tc.ErrorMatches, "can't remove model: model still alive")
}

func (s *StateSuite) TestRemoveDyingModelForDyingModel(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(st.SetDyingModelToDead(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.DeepEquals, state.Dead)

	c.Assert(st.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestRemoveDyingModelForDeadModel(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.DeepEquals, state.Dying)

	c.Assert(st.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *StateSuite) TestSetDyingModelToDeadRequiresDyingModel(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetDyingModelToDead()
	c.Assert(errors.Cause(err), tc.Equals, state.ErrModelNotDying)

	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.DeepEquals, state.Dying)
	c.Assert(st.SetDyingModelToDead(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.DeepEquals, state.Dead)

	err = st.SetDyingModelToDead()
	c.Assert(errors.Cause(err), tc.Equals, state.ErrModelNotDying)
}

func (s *StateSuite) TestRemoveImportingModelDocsFailsActive(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := st.RemoveImportingModelDocs()
	c.Assert(err, tc.ErrorMatches, "can't remove model: model not being imported for migration")
}

func (s *StateSuite) TestRemoveImportingModelDocsFailsExporting(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, tc.ErrorMatches, "can't remove model: model not being imported for migration")
}

func (s *StateSuite) TestRemoveImportingModelDocsImporting(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	userModelKey := s.insertFakeModelDocs(c, st)
	c.Assert(state.HostedModelCount(c, st), tc.Equals, 1)

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = m.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	// remove suite state
	err = s.State.RemoveImportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, st), tc.Equals, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsFailsActive(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := st.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorMatches, "can't remove model: model not being exported for migration")
}

func (s *StateSuite) TestRemoveExportingModelDocsFailsImporting(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorMatches, "can't remove model: model not being exported for migration")
}

func (s *StateSuite) TestRemoveExportingModelDocsExporting(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	userModelKey := s.insertFakeModelDocs(c, st)
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 1)

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	err = s.Model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	// test that we can not find the user:envName unique index
	s.checkUserModelNameExists(c, checkUserModelNameArgs{st: st, id: userModelKey, exists: false})
	s.AssertModelDeleted(c, st)
	c.Assert(state.HostedModelCount(c, s.State), tc.Equals, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesOfferPermissions(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	s.createOffer(c)

	coll, closer := state.GetRawCollection(s.State, "permissions")
	defer closer()
	cnt, err := coll.Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cnt, tc.Equals, 8)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	cnt, err = coll.Count()
	c.Assert(err, tc.ErrorIsNil)
	// 2 model permissions deleted.
	// 2 offer permissions deleted.
	c.Assert(cnt, tc.Equals, 4)
}

func (s *StateSuite) createOffer(c *tc.C) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps := map[string]string{"db": "server", "db-admin": "server-admin"}
	sd := state.NewApplicationOffers(s.State)
	owner := s.Factory.MakeUser(c, nil)
	offerArgs := crossmodel.AddApplicationOfferArgs{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql is a db server",
		Endpoints:              eps,
		Owner:                  owner.Name(),
		HasRead:                []string{"everyone@external"},
	}
	_, err := sd.AddOffer(offerArgs)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesLogs(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveImportingModelDocsRemovesLogs(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, tc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveImportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveModelRemovesLogs(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = model.SetDead()
	c.Assert(err, tc.ErrorIsNil)

	writeLogs(c, st, 5)
	writeLogs(c, s.State, 5)

	err = st.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)

	assertLogCount(c, s.State, 5)
	assertLogCount(c, st, 0)
}

func (s *StateSuite) TestRemoveExportingModelDocsRemovesLogTrackers(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, tc.ErrorIsNil)

	t1 := state.NewLastSentLogTracker(st, model.UUID(), "go-away")
	defer t1.Close()
	t2 := state.NewLastSentLogTracker(st, s.State.ModelUUID(), "stay")
	defer t2.Close()

	c.Assert(t1.Set(100, 100), tc.ErrorIsNil)
	c.Assert(t2.Set(100, 100), tc.ErrorIsNil)

	err = st.RemoveExportingModelDocs()
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = t1.Get()
	c.Check(errors.Cause(err), tc.Equals, state.ErrNeverForwarded)

	id, count, err := t2.Get()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, int64(100))
	c.Check(count, tc.Equals, int64(100))
}

func writeLogs(c *tc.C, st *state.State, n int) {
	dbLogger := state.NewDbLogger(st)
	defer dbLogger.Close()
	for i := 0; i < n; i++ {
		err := dbLogger.Log([]corelogger.LogRecord{{
			Time:     time.Now(),
			Entity:   "application-van-occupanther",
			Module:   "chasing after deer",
			Location: "in a log house",
			Level:    loggo.INFO,
			Message:  "why are your fingers like that of a hedge in winter?",
		}})
		c.Assert(err, tc.ErrorIsNil)
	}
}

func assertLogCount(c *tc.C, st *state.State, expected int) {
	logColl := st.MongoSession().DB("logs").C("logs." + st.ModelUUID())
	actual, err := logColl.Count()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actual, tc.Equals, expected)
}

func (s *StateSuite) TestWatchForModelConfigChanges(c *tc.C) {
	cur := jujuversion.Current
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.Model.WatchForModelConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	// Multiple changes will only result in a single change notification
	newVersion := cur
	newVersion.Minor++
	err = statetesting.SetAgentVersion(s.State, newVersion)
	c.Assert(err, tc.ErrorIsNil)

	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()

	newerVersion := newVersion
	newerVersion.Minor++
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Setting it to the same value does not trigger a change notification
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchForModelConfigControllerChanges(c *tc.C) {
	w := s.Model.WatchForModelConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
}

func (s *StateSuite) TestWatchCloudSpecChanges(c *tc.C) {
	w := s.Model.WatchCloudSpecChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	cloud, err := s.State.Cloud(s.Model.CloudName())
	c.Assert(err, tc.ErrorIsNil)

	// Multiple changes will only result in a single change notification
	cloud.StorageEndpoint = "https://storage"
	err = s.State.UpdateCloud(cloud)
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	cloud.StorageEndpoint = "https://storage1"
	err = s.State.UpdateCloud(cloud)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *StateSuite) TestAddAndGetEquivalence(c *tc.C) {
	// The equivalence tested here isn't necessarily correct, and
	// comparing private details is discouraged in the project.
	// The implementation might choose to cache information, or
	// to have different logic when adding or removing, and the
	// comparison might fail despite it being correct.
	// That said, we've had bugs with txn-revno being incorrect
	// before, so this testing at least ensures we're conscious
	// about such changes.

	m1, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	m2, err := s.State.Machine(m1.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m1, tc.DeepEquals, m2)

	charm1 := s.AddTestingCharm(c, "wordpress")
	charm2, err := s.State.Charm(charm1.URL())
	c.Assert(err, tc.ErrorIsNil)
	// Refresh is required to set the charmURL, so the test will succeed.
	err = charm2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charm1, tc.DeepEquals, charm2)

	wordpress1 := s.AddTestingApplication(c, "wordpress", charm1)
	wordpress2, err := s.State.Application("wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(wordpress1, tc.DeepEquals, wordpress2)

	unit1, err := wordpress1.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	unit2, err := s.State.Unit("wordpress/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit1, tc.DeepEquals, unit2)

	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, tc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	relation1, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	relation2, err := s.State.EndpointsRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relation1, tc.DeepEquals, relation2)
	relation3, err := s.State.Relation(relation1.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relation1, tc.DeepEquals, relation3)
}

func tryOpenState(modelTag names.ModelTag, controllerTag names.ControllerTag, info *mongo.MongoInfo) error {
	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	if err != nil {
		return err
	}
	defer session.Close()
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
	if err == nil {
		err = pool.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *tc.C) {
	info := statetesting.NewMongoInfo()
	info.Tag, info.Password = names.NewUserTag("arble"), "bar"
	err := tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsUnauthorized)
	c.Check(err, tc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = names.NewUserTag("arble"), ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsUnauthorized)
	c.Check(err, tc.ErrorMatches, `cannot log in to admin database as "user-arble": unauthorized mongo access: .*`)

	info.Tag, info.Password = nil, ""
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(err, tc.ErrorIsNil)
}

func testSetPassword(c *tc.C, getEntity func() (state.Authenticator, error)) {
	e, err := getEntity()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(e.PasswordValid(goodPassword), tc.IsFalse)
	err = e.SetPassword(goodPassword)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(e.PasswordValid(goodPassword), tc.IsTrue)

	// Check a newly-fetched entity has the same password.
	e2, err := getEntity()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(e2.PasswordValid(goodPassword), tc.IsTrue)

	err = e.SetPassword(alternatePassword)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(e.PasswordValid(goodPassword), tc.IsFalse)
	c.Assert(e.PasswordValid(alternatePassword), tc.IsTrue)

	// Check that refreshing fetches the new password
	err = e2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(e2.PasswordValid(alternatePassword), tc.IsTrue)

	if le, ok := e.(lifer); ok {
		testWhenDying(c, le, noErr, deadErr, func() error {
			return e.SetPassword("arble-farble-dying-yarble")
		})
	}
}

type findEntityTest struct {
	tag names.Tag
	err string
}

var findEntityTests = []findEntityTest{{
	tag: names.NewRelationTag("app1:rel1 app2:rel2"),
	err: `relation "app1:rel1 app2:rel2" not found`,
}, {
	tag: names.NewModelTag("9f484882-2f18-4fd2-967d-db9663db7bea"),
	err: `model "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
}, {
	tag: names.NewMachineTag("0"),
}, {
	tag: names.NewControllerAgentTag("0"),
}, {
	tag: names.NewApplicationTag("ser-vice2"),
}, {
	tag: names.NewRelationTag("wordpress:db ser-vice2:server"),
}, {
	tag: names.NewUnitTag("ser-vice2/0"),
}, {
	tag: names.NewUserTag("arble"),
}, {
	tag: names.NewActionTag("fedcba98-7654-4321-ba98-76543210beef"),
	err: `action "fedcba98-7654-4321-ba98-76543210beef" not found`,
}, {
	tag: names.NewOperationTag("666"),
	err: `operation "666" not found`,
}, {
	tag: names.NewUserTag("eric"),
}, {
	tag: names.NewUserTag("eric@local"),
}, {
	tag: names.NewUserTag("eric@remote"),
	err: `user "eric@remote" not found`,
}}

var entityTypes = map[string]interface{}{
	names.UserTagKind:            (*state.User)(nil),
	names.ModelTagKind:           (*state.Model)(nil),
	names.ApplicationTagKind:     (*state.Application)(nil),
	names.UnitTagKind:            (*state.Unit)(nil),
	names.MachineTagKind:         (*state.Machine)(nil),
	names.ControllerAgentTagKind: (*state.ControllerNodeInstance)(nil),
	names.RelationTagKind:        (*state.Relation)(nil),
	names.ActionTagKind:          (state.Action)(nil),
	names.OperationTagKind:       (state.Operation)(nil),
}

func (s *StateSuite) TestFindEntity(c *tc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "eric"})
	_, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddControllerNode()
	c.Assert(err, tc.ErrorIsNil)
	app := s.AddTestingApplication(c, "ser-vice2", s.AddTestingCharm(c, "mysql"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	operationID, err := s.Model.EnqueueOperation("something", 1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.Model.AddAction(unit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.Factory.MakeUser(c, &factory.UserParams{Name: "arble"})
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "ser-vice2")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.String(), tc.Equals, "wordpress:db ser-vice2:server")

	findEntityTests = append([]findEntityTest{}, findEntityTests...)
	findEntityTests = append(findEntityTests, findEntityTest{
		tag: names.NewModelTag(s.Model.UUID()),
	})

	for i, test := range findEntityTests {
		c.Logf("test %d: %q", i, test.tag)
		e, err := s.State.FindEntity(test.tag)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			kind := test.tag.Kind()
			c.Assert(e, tc.FitsTypeOf, entityTypes[kind])
			if kind == names.ModelTagKind {
				// TODO(axw) 2013-12-04 #1257587
				// We *should* only be able to get the entity with its tag, but
				// for backwards-compatibility we accept any non-UUID tag.
				c.Assert(e.Tag(), tc.Equals, s.Model.Tag())
			} else if kind == names.UserTagKind {
				// Test the fully qualified username rather than the tag structure itself.
				expected := test.tag.(names.UserTag).Id()
				c.Assert(e.Tag().(names.UserTag).Id(), tc.Equals, expected)
			} else {
				c.Assert(e.Tag(), tc.Equals, test.tag)
			}
		}
	}
}

func (s *StateSuite) TestParseNilTagReturnsAnError(c *tc.C) {
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, nil)
	c.Assert(err, tc.ErrorMatches, "tag is nil")
	c.Assert(coll, tc.Equals, "")
	c.Assert(id, tc.IsNil)
}

func (s *StateSuite) TestParseMachineTag(c *tc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, m.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "machines")
	c.Assert(id, tc.Equals, state.DocID(s.State, m.Id()))
}

func (s *StateSuite) TestParseApplicationTag(c *tc.C) {
	app := s.AddTestingApplication(c, "ser-vice2", s.AddTestingCharm(c, "dummy"))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, app.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "applications")
	c.Assert(id, tc.Equals, state.DocID(s.State, app.Name()))
}

func (s *StateSuite) TestParseUnitTag(c *tc.C) {
	app := s.AddTestingApplication(c, "application2", s.AddTestingCharm(c, "dummy"))
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, u.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "units")
	c.Assert(id, tc.Equals, state.DocID(s.State, u.Name()))
}

func (s *StateSuite) TestParseActionTag(c *tc.C) {
	app := s.AddTestingApplication(c, "application2", s.AddTestingCharm(c, "dummy"))
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, tc.ErrorIsNil)
	f, err := s.Model.AddAction(u, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	action, err := s.Model.Action(f.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(action.Tag(), tc.Equals, names.NewActionTag(action.Id()))
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, action.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "actions")
	c.Assert(id, tc.Equals, action.Id())
}

func (s *StateSuite) TestParseUserTag(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, user.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "users")
	c.Assert(id, tc.Equals, user.Name())
}

func (s *StateSuite) TestParseModelTag(c *tc.C) {
	coll, id, err := state.ConvertTagToCollectionNameAndId(s.State, s.Model.Tag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(coll, tc.Equals, "models")
	c.Assert(id, tc.Equals, s.Model.UUID())
}

func (s *StateSuite) TestWatchCleanups(c *tc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Set up two relations for later use, check no events.
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	s.AddTestingApplication(c, "varnish", s.AddTestingCharm(c, "varnish"))
	c.Assert(err, tc.ErrorIsNil)
	eps, err = s.State.InferEndpoints("wordpress", "varnish")
	c.Assert(err, tc.ErrorIsNil)
	relV, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy one relation, check one change.
	err = relM.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Handle that cleanup doc and create another, check one change.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = relV.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean up final doc, check change.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchCleanupsDiesOnStateClose(c *tc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		w := st.WatchCleanups()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchCleanupsBulk(c *tc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Create two peer relations by creating their applications.
	riak := s.AddTestingApplication(c, "riak", s.AddTestingCharm(c, "riak"))
	_, err := riak.Endpoint("ring")
	c.Assert(err, tc.ErrorIsNil)
	allHooks := s.AddTestingApplication(c, "all-hooks", s.AddTestingCharm(c, "all-hooks"))
	_, err = allHooks.Endpoint("self")
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy them both, check one change.
	err = riak.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	// TODO(quiescence): reimplement some quiescence on the cleanup watcher
	wc.AssertOneChange()
	err = allHooks.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Clean them both up, check one change.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertAtleastOneChange()
}

func (s *StateSuite) TestWatchMinUnits(c *tc.C) {
	// Check initial event.
	w := s.State.WatchMinUnits()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Set up applications for later use.
	wordpress := s.AddTestingApplication(c,
		"wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpressName := wordpress.Name()

	// Add application units for later use.
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	wordpress1, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	// No events should occur.
	wc.AssertNoChange()

	// Add minimum units to a application; a single change should occur.
	err = wordpress.SetMinUnits(2)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Decrease minimum units for a application; expect no changes.
	err = wordpress.SetMinUnits(1)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Increase minimum units for two applications; a single change should occur.
	err = mysql.SetMinUnits(1)
	c.Assert(err, tc.ErrorIsNil)
	err = wordpress.SetMinUnits(3)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(mysql.Name(), wordpressName)
	wc.AssertNoChange()

	// Remove minimum units for a application; expect no changes.
	err = mysql.SetMinUnits(0)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a unit of a application with required minimum units.
	// Also avoid the unit removal. A single change should occur.
	preventUnitDestroyRemove(c, wordpress0)
	err = wordpress0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Two actions: destroy a unit and increase minimum units for a application.
	// A single change should occur, and the application name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, tc.ErrorIsNil)
	err = wordpress1.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Destroy a unit of a application not requiring minimum units; expect no changes.
	err = mysql0.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application with required minimum units; expect no changes.
	err = wordpress.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy a application not requiring minimum units; expect no changes.
	err = mysql.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchMinUnitsDiesOnStateClose(c *tc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		w := st.WatchMinUnits()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchSubnets(c *tc.C) {
	filter := func(id interface{}) bool {
		return id != "0"
	}
	w := s.State.WatchSubnets(filter)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	// Check initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	_, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.20.0.0/24"})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "10.0.0.0/24"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("1")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchSubnetsDiesOnStateClose(c *tc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		w := st.WatchSubnets(nil)
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) setupWatchRemoteRelations(c *tc.C, wc statetesting.StringsWatcherC) (*state.RemoteApplication, *state.Application, *state.Relation) {
	// Check initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: s.Model.ModelTag(),
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Add a remote relation, single change should occur.
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(remoteApp.Refresh(), tc.ErrorIsNil)
	c.Assert(app.Refresh(), tc.ErrorIsNil)

	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()
	return remoteApp, app, rel
}

func (s *StateSuite) TestWatchRemoteRelationsIgnoresLocal(c *tc.C) {
	// Set up a non-remote relation to ensure it is properly filtered out.
	s.AddTestingApplication(c, "wplocal", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysqllocal", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wplocal", "mysqllocal")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	// Check initial event.
	wc.AssertChange()
	// No change for local relation.
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRelation(c *tc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	_, _, rel := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote relation.
	// A single change should occur.
	err := rel.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyRemoteApplication(c *tc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	remoteApp, _, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the remote application.
	// A single change should occur.
	err := remoteApp.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDestroyLocalApplication(c *tc.C) {
	w := s.State.WatchRemoteRelations()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	_, app, _ := s.setupWatchRemoteRelations(c, wc)

	// Destroy the local application.
	// A single change should occur.
	err := app.Destroy()
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("wordpress:db mysql:database")
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRemoteRelationsDiesOnStateClose(c *tc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *tc.C, st *state.State) waiter {
		w := st.WatchRemoteRelations()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestSetModelAgentVersionErrors(c *tc.C) {
	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	stringVersion := agentVersion.String()

	// Add 4 machines: one with a different version, one with an
	// empty version, one with the current version, and one with
	// the new version.
	machine0, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = machine0.SetAgentVersion(version.MustParseBinary("9.9.9-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	machine1, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	machine2, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = machine2.SetAgentVersion(version.MustParseBinary(stringVersion + "-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	machine3, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = machine3.SetAgentVersion(version.MustParseBinary("4.5.6-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)

	// Verify machine0 and machine1 are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	expectErr := fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(err, tc.Satisfies, state.IsVersionInconsistentError)

	// Add a application and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress"),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	unit0, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit0.SetAgentVersion(version.MustParseBinary("6.6.6-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	_, err = application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	unit2, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit2.SetAgentVersion(version.MustParseBinary(stringVersion + "-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	unit3, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = unit3.SetAgentVersion(version.MustParseBinary("4.5.6-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)

	// Verify unit0 and unit1 are reported as error, along with the
	// machines from before.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: machine-0, machine-1, unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(err, tc.Satisfies, state.IsVersionInconsistentError)

	// Now remove the machines.
	for _, machine := range []*state.Machine{machine0, machine1, machine2} {
		err = machine.EnsureDead()
		c.Assert(err, tc.ErrorIsNil)
		err = machine.Remove()
		c.Assert(err, tc.ErrorIsNil)
	}

	// Verify only the units are reported as error.
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	expectErr = fmt.Sprintf("some agents have not upgraded to the current model version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(err, tc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *tc.C, st *state.State) (*config.Config, string) {
	// Get the agent-version set in the model.
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	application, err := st.AddApplication(state.AddApplicationArgs{
		Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress"),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetAgentVersion(version.MustParseBinary(currentVersion + "-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)

	return modelConfig, currentVersion
}

func (s *StateSuite) changeEnviron(c *tc.C, modelConfig *config.Config, name string, value interface{}) {
	attrs := modelConfig.AllAttrs()
	attrs[name] = value
	c.Assert(s.Model.UpdateModelConfig(attrs, nil), tc.IsNil)
}

func assertAgentVersion(c *tc.C, st *state.State, vers, stream string) {
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	modelConfig, err := m.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Assert(agentVersion.String(), tc.Equals, vers)
	agentStream := modelConfig.AgentStream()
	c.Assert(agentStream, tc.Equals, stream)

}

func (s *StateSuite) TestSetModelAgentVersionRetriesOnConfigChange(c *tc.C) {
	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change something
	// other than the version, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "default-series", "focal")
	}).Check()

	// Change the agent-version and ensure it has changed.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionSucceedsWithSameVersion(c *tc.C) {
	modelConfig, _ := s.prepareAgentVersionTests(c, s.State)

	// Set up a transaction hook to change the version
	// to the new one, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, modelConfig, "agent-version", "4.5.6")
	}).Check()

	// Change the agent-version and verify.
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionUpdateStream(c *tc.C) {
	proposed := "proposed"
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), &proposed, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", proposed)

	err = s.State.SetModelAgentVersion(version.MustParse("4.5.7"), nil, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.7", proposed)
}

func (s *StateSuite) TestSetModelAgentVersionUpdateStreamEmpty(c *tc.C) {
	stream := ""
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), &stream, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionOnOtherModel(c *tc.C) {
	current := version.MustParseBinary("1.24.7-ubuntu-amd64")
	s.PatchValue(&jujuversion.Current, current.Number)
	s.PatchValue(&arch.HostArch, func() string { return current.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	higher := version.MustParseBinary("1.25.0-ubuntu-amd64")
	lower := version.MustParseBinary("1.24.6-ubuntu-amd64")

	// Set other model version to < controller model version
	err := otherSt.SetModelAgentVersion(lower.Number, nil, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, otherSt, lower.Number.String(), "released")

	// Set other model version == controller version
	err = otherSt.SetModelAgentVersion(jujuversion.Current, nil, false)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, otherSt, jujuversion.Current.String(), "released")

	// Set other model version to > server version
	err = otherSt.SetModelAgentVersion(higher.Number, nil, false)
	expected := fmt.Sprintf("model cannot be upgraded to %s while the controller is %s: upgrade 'controller' model first",
		higher.Number,
		jujuversion.Current,
	)
	c.Assert(err, tc.ErrorMatches, expected)
}

func (s *StateSuite) TestSetModelAgentVersionExcessiveContention(c *tc.C) {
	modelConfig, currentVersion := s.prepareAgentVersionTests(c, s.State)

	// Set a hook to change the config 3 times
	// to test we return ErrExcessiveContention.
	hooks := []jujutxn.TestHook{
		{Before: func() { s.changeEnviron(c, modelConfig, "default-series", "focal") }},
		{Before: func() { s.changeEnviron(c, modelConfig, "default-series", "jammy") }},
		{Before: func() { s.changeEnviron(c, modelConfig, "default-series", "focal") }},
	}

	state.SetMaxTxnAttempts(c, s.State, 3)
	defer state.SetTestHooks(c, s.State, hooks...).Check()
	err := s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	c.Assert(errors.Cause(err), tc.Equals, jujutxn.ErrExcessiveContention)
	// Make sure the version remained the same.
	assertAgentVersion(c, s.State, currentVersion, "released")
}

func (s *StateSuite) TestSetModelAgentVersionMixedVersions(c *tc.C) {
	_, currentVersion := s.prepareAgentVersionTests(c, s.State)
	machine, err := s.State.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	// Force this to something old that should not match current versions
	err = machine.SetAgentVersion(version.MustParseBinary("1.0.1-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	// This should be refused because an agent doesn't match "currentVersion"
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, false)
	c.Check(err, tc.ErrorMatches, "some agents have not upgraded to the current model version .*: machine-0")
	// Version hasn't changed
	assertAgentVersion(c, s.State, currentVersion, "released")
	// But we can force it
	err = s.State.SetModelAgentVersion(version.MustParse("4.5.6"), nil, true)
	c.Assert(err, tc.ErrorIsNil)
	assertAgentVersion(c, s.State, "4.5.6", "released")
}

func (s *StateSuite) TestSetModelAgentVersionFailsIfUpgrading(c *tc.C) {
	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)

	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary(agentVersion.String() + "-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion, nil, false)
	c.Assert(errors.Is(err, stateerrors.ErrUpgradeInProgress), tc.IsTrue)
}

func (s *StateSuite) TestSetModelAgentVersionFailsReportsCorrectError(c *tc.C) {
	// Ensure that the correct error is reported if an upgrade is
	// progress but that isn't the reason for the
	// SetModelAgentVersion call failing.

	// Get the agent-version set in the model.
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)

	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetAgentVersion(version.MustParseBinary("9.9.9-ubuntu-amd64"))
	c.Assert(err, tc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "", "fake-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	nextVersion := agentVersion
	nextVersion.Minor++

	// Create an unfinished UpgradeInfo instance.
	_, err = s.State.EnsureUpgradeInfo(machine.Tag().Id(), agentVersion, nextVersion)
	c.Assert(err, tc.ErrorIsNil)

	err = s.State.SetModelAgentVersion(nextVersion, nil, false)
	c.Assert(err, tc.ErrorMatches, "some agents have not upgraded to the current model version.+")
}

type waiter interface {
	Wait() error
}

// testWatcherDiesWhenStateCloses calls the given function to start a watcher,
// closes the state and checks that the watcher dies with the expected error.
// The watcher should already have consumed the first
// event, otherwise the watcher's initialisation logic may
// interact with the closed state, causing it to return an
// unexpected error (often "Closed explicitly").
func testWatcherDiesWhenStateCloses(
	c *tc.C,
	session *mgo.Session,
	modelTag names.ModelTag,
	controllerTag names.ControllerTag,
	startWatcher func(c *tc.C, st *state.State) waiter,
) {
	controller, err := state.OpenController(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
	})
	c.Assert(err, tc.ErrorIsNil)
	sysState, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	watcher := startWatcher(c, sysState)
	err = controller.Close()
	c.Assert(err, tc.ErrorIsNil)
	done := make(chan error)
	go func() {
		done <- watcher.Wait()
	}()
	select {
	case err := <-done:
		c.Assert(err, tc.ErrorMatches, state.ErrStateClosed.Error())
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher %T did not exit when state closed", watcher)
	}
}

func (s *StateSuite) TestControllerInfo(c *tc.C) {
	ids, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids.CloudName, tc.Equals, "dummy")
	c.Assert(ids.ModelTag, tc.Equals, s.modelTag)
	c.Assert(ids.ControllerIds, tc.HasLen, 0)

	// TODO(rog) more testing here when we can actually add
	// controllers.
}

func (s *StateSuite) TestReopenWithNoMachines(c *tc.C) {
	expected := &state.ControllerInfo{
		CloudName: "dummy",
		ModelTag:  s.modelTag,
	}
	info, err := s.State.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)

	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, tc.ErrorIsNil)
	defer controller.Close()

	sysState, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	info, err = sysState.ControllerInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)
}

func (s *StateSuite) TestStateServingInfo(c *tc.C) {
	_, err := s.State.StateServingInfo()
	c.Assert(err, tc.ErrorMatches, "state serving info not found")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	data := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	err = s.State.SetStateServingInfo(data)
	c.Assert(err, tc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, data)
}

func (s *StateSuite) TestSetAPIHostPortsNoMgmtSpace(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}, {
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.6.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	newHostPorts = []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      13,
	}}}
	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	gotHostPorts, err = ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentSame(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}, {{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// API host ports are concurrently changed to the same
	// desired value; second arrival will fail its assertion,
	// refresh finding nothing to do, and then issue a
	// read-only assertion that succeeds.
	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts(hostPorts)
		c.Assert(err, tc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, tc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, tc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(prevRevno, tc.Not(tc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Equals, prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Equals, prevAgentsRevno)
}

func (s *StateSuite) TestSetAPIHostPortsNoMgmtSpaceConcurrentDifferent(c *tc.C) {
	hostPorts0 := network.SpaceHostPorts{{
		SpaceAddress: network.NewSpaceAddress("0.4.8.16", network.WithScope(network.ScopePublic)),
		NetPort:      2,
	}}
	hostPorts1 := network.SpaceHostPorts{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}

	// API host ports are concurrently changed to different
	// values; second arrival will fail its assertion, refresh
	// finding and reattempt.

	ctrC := state.ControllersC
	var prevRevno int64
	var prevAgentsRevno int64
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts0})
		c.Assert(err, tc.ErrorIsNil)
		revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
		c.Assert(err, tc.ErrorIsNil)
		prevRevno = revno
		revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
		c.Assert(err, tc.ErrorIsNil)
		prevAgentsRevno = revno
	}).Check()

	err := s.State.SetAPIHostPorts([]network.SpaceHostPorts{hostPorts1})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(prevRevno, tc.Not(tc.Equals), 0)

	revno, err := state.TxnRevno(s.State, ctrC, "apiHostPorts")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Not(tc.Equals), prevRevno)

	revno, err = state.TxnRevno(s.State, ctrC, "apiHostPortsForAgents")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(revno, tc.Not(tc.Equals), prevAgentsRevno)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	hostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostPorts, tc.DeepEquals, []network.SpaceHostPorts{hostPorts1})

	hostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hostPorts, tc.DeepEquals, []network.SpaceHostPorts{hostPorts1})
}

func (s *StateSuite) TestSetAPIHostPortsWithMgmtSpace(c *tc.C) {
	sp, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, tc.ErrorIsNil)

	s.SetJujuManagementSpace(c, "mgmt01")

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	hostPort1 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}
	hostPort2 := network.SpaceHostPort{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "0.4.8.16",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			SpaceID: sp.Id(),
		},
		NetPort: 2,
	}
	hostPort3 := network.SpaceHostPort{
		SpaceAddress: network.NewSpaceAddress("0.6.1.2", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      5,
	}
	newHostPorts := []network.SpaceHostPorts{{hostPort1, hostPort2}, {hostPort3}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)

	gotHostPorts, err = ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	// First slice filtered down to the address in the management space.
	// Second filtered to zero elements, so retains the supplied slice.
	c.Assert(gotHostPorts, tc.DeepEquals, []network.SpaceHostPorts{{hostPort2}, {hostPort3}})
}

func (s *StateSuite) TestSetAPIHostPortsForAgentsNoDocument(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	// Delete the addresses for agents document before setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), tc.Equals, mgo.ErrNotFound)

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestAPIHostPortsForAgentsNoDocument(c *tc.C) {
	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	newHostPorts := []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}

	err = s.State.SetAPIHostPorts(newHostPorts)
	c.Assert(err, tc.ErrorIsNil)

	// Delete the addresses for agents document after setting.
	col := s.State.MongoSession().DB("juju").C(state.ControllersC)
	key := "apiHostPortsForAgents"
	err = col.RemoveId(key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(col.FindId(key).One(&bson.D{}), tc.Equals, mgo.ErrNotFound)

	ctrlSt, err := s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	gotHostPorts, err := ctrlSt.APIHostPortsForAgents()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotHostPorts, tc.DeepEquals, newHostPorts)
}

func (s *StateSuite) TestNowToTheSecond(c *tc.C) {
	t := state.NowToTheSecond(s.State)
	rounded := t.Round(time.Second)
	c.Assert(t, tc.DeepEquals, rounded)
}

func (s *StateSuite) TestUnitsForInvalidId(c *tc.C) {
	// Check that an error is returned if an invalid machine id is provided.
	// Success cases are tested as part of TestMachinePrincipalUnits in the
	// MachineSuite.
	units, err := s.State.UnitsFor("invalid-id")
	c.Assert(units, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `"invalid-id" is not a valid machine id`)
}

func (s *StateSuite) TestRunTransactionObserver(c *tc.C) {
	type args struct {
		dbName    string
		modelUUID string
		attempt   int
		duration  time.Duration
		ops       []mgotxn.Op
		err       error
	}
	var mu sync.Mutex
	var recordedCalls []args
	getCalls := func() []args {
		mu.Lock()
		defer mu.Unlock()
		return recordedCalls[:]
	}

	params := s.testOpenParams()
	params.RunTransactionObserver = func(dbName, modelUUID string, attempt int, duration time.Duration, ops []mgotxn.Op, err error) {
		mu.Lock()
		defer mu.Unlock()
		recordedCalls = append(recordedCalls, args{
			dbName:    dbName,
			modelUUID: modelUUID,
			attempt:   attempt,
			duration:  duration,
			ops:       ops,
			err:       err,
		})
	}
	controller, err := state.OpenController(params)
	c.Assert(err, tc.ErrorIsNil)
	defer controller.Close()
	st, err := controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(getCalls(), tc.HasLen, 0)

	err = st.SetModelConstraints(constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)

	calls := getCalls()
	// There may be some leadership txns in the call list.
	// We only care about the constraints call.
	found := false
	for _, call := range calls {
		if call.ops[0].C != "constraints" {
			continue
		}
		c.Check(call.dbName, tc.Equals, "juju")
		c.Check(call.modelUUID, tc.Equals, s.modelTag.Id())
		c.Check(call.duration, tc.Not(tc.Equals), 0)
		c.Check(call.err, tc.IsNil)
		c.Check(call.ops, tc.HasLen, 1)
		c.Check(call.ops[0].Update, tc.NotNil)
		found = true
		break
	}
	c.Assert(found, tc.IsTrue)
}

type SetAdminMongoPasswordSuite struct {
	testing.BaseSuite
}

func TestSetAdminMongoPasswordSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &SetAdminMongoPasswordSuite{})
}

func setAdminPassword(c *tc.C, inst *mgotesting.MgoInstance, owner names.UserTag, password string) {
	session, err := inst.Dial()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()
	err = mongo.SetAdminMongoPassword(session, owner.String(), password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SetAdminMongoPasswordSuite) TestSetAdminMongoPassword(c *tc.C) {
	inst := &mgotesting.MgoInstance{
		EnableAuth:       true,
		EnableReplicaSet: true,
	}
	err := inst.Start(nil)
	c.Assert(err, tc.ErrorIsNil)
	defer inst.DestroyWithLog()

	// We need to make an admin user before we initialize the state
	// because in Mongo3.2 the localhost exception no longer has
	// permission to create indexes.
	// https://docs.mongodb.com/manual/core/security-users/#localhost-exception
	owner := names.NewLocalUserTag("initialize-admin")
	password := "huggies"
	setAdminPassword(c, inst, owner, password)

	noAuthInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      []string{inst.Addr()},
			CACert:     testing.CACert,
			DisableTLS: true,
		},
	}

	session, err := mongo.DialWithInfo(mongo.MongoInfo{
		Info:     noAuthInfo.Info,
		Tag:      owner,
		Password: password,
	}, mongotest.DialOpts())
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	cfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  session,
		AdminPassword: password,
	})
	c.Assert(err, tc.ErrorIsNil)
	st, err := ctlr.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	defer ctlr.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetAdminMongoPassword("foo")
	c.Assert(err, tc.ErrorIsNil)
	err = st.MongoSession().DB("admin").Login("admin", "foo")
	c.Assert(err, tc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	err = tryOpenState(m.ModelTag(), st.ControllerTag(), noAuthInfo)
	c.Check(errors.Cause(err), tc.Satisfies, errors.IsUnauthorized)
	// note: collections are set up in arbitrary order, proximate cause of
	// failure may differ.
	c.Check(err, tc.ErrorMatches, `[^:]+: unauthorized mongo access: .*`)

	passwordOnlyInfo := *noAuthInfo
	passwordOnlyInfo.Password = "foo"

	// Under mongo 3.2 it's not possible to create collections and
	// indexes with no user - the localhost exception only permits
	// creating users. There were some checks for unsetting the
	// password and then creating the state in an older version of
	// this test, but they couldn't be made to work with 3.2.
	err = tryOpenState(m.ModelTag(), st.ControllerTag(), &passwordOnlyInfo)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) setUpWatchRelationNetworkScenario(c *tc.C) *state.Relation {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: s.Model.ModelTag(),
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	wpCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wpCharm})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	return rel
}

func (s *StateSuite) TestWatchRelationIngressNetworks(c *tc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationIngressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Initial ingress network creation.
	relIngress := state.NewRelationIngressNetworks(s.State)
	_, err := relIngress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32", "4.3.2.1/16")
	wc.AssertNoChange()

	// Update value.
	_, err = relIngress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32")
	wc.AssertNoChange()

	// Update value, admin override.
	_, err = relIngress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("10.0.0.1/32")
	wc.AssertNoChange()

	// Same value.
	_, err = relIngress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Delete relation.
	state.RemoveRelation(c, rel, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange()
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationIngressNetworksIgnoresEgress(c *tc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationIngressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworks(c *tc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Initial egress network creation.
	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32", "4.3.2.1/16")
	wc.AssertNoChange()

	// Update value.
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("1.2.3.4/32")
	wc.AssertNoChange()

	// Update value, admin override.
	_, err = relEgress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange("10.0.0.1/32")
	wc.AssertNoChange()

	// Same value.
	_, err = relEgress.Save(rel.Tag().Id(), true, []string{"10.0.0.1/32"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Delete relation.
	state.RemoveRelation(c, rel, false)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange()
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchRelationEgressNetworksIgnoresIngress(c *tc.C) {
	rel := s.setUpWatchRelationNetworkScenario(c)
	// Check initial event.
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	relEgress := state.NewRelationIngressNetworks(s.State)
	_, err := relEgress.Save(rel.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) testOpenParams() state.OpenParams {
	return state.OpenParams{
		Clock:               clock.WallClock,
		ControllerTag:       s.State.ControllerTag(),
		ControllerModelTag:  s.modelTag,
		MongoSession:        s.Session,
		WatcherPollInterval: 10 * time.Millisecond,
	}
}

func (s *StateSuite) TestControllerTimestamp(c *tc.C) {
	now := testing.NonZeroTime()
	clock := testclock.NewClock(now)

	err := s.State.SetClockForTesting(clock)
	c.Assert(err, tc.ErrorIsNil)

	got, err := s.State.ControllerTimestamp()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.NotNil)

	c.Assert(*got, tc.DeepEquals, now)
}

func (s *StateSuite) TestAddRelationCreatesApplicationSettings(c *tc.C) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	settings := state.NewStateSettings(s.State)

	mysqlKey := fmt.Sprintf("r#%d#mysql", rel.Id())
	_, err = settings.ReadSettings(mysqlKey)
	c.Assert(err, tc.ErrorIsNil)

	wpKey := fmt.Sprintf("r#%d#wordpress", rel.Id())
	_, err = settings.ReadSettings(wpKey)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestPeerRelationCreatesApplicationSettings(c *tc.C) {
	app := state.AddTestingApplication(c, s.State, "riak", state.AddTestingCharm(c, s.State, "riak"))
	ep, err := app.Endpoint("ring")
	c.Assert(err, tc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(ep)
	c.Assert(err, tc.ErrorIsNil)

	settings := state.NewStateSettings(s.State)

	key := fmt.Sprintf("r#%d#riak", rel.Id())
	_, err = settings.ReadSettings(key)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StateSuite) TestAddRelationAssertsRelationCount(c *tc.C) {
	f := factory.NewFactory(s.State, s.StatePool)
	wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})

	mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlCharm})

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, tc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		// we modify the relationcount of the wordpress application as if
		// another relation was added in the meantime and we expect the
		// AddRelation call to fail because an assertion for relationcount
		// was added.
		s.applications.Update(
			bson.D{{Name: "_id", Value: state.DocID(s.State, "wordpress")}},
			bson.D{{Name: "$inc", Value: bson.D{{"relationcount", 1}}}},
		)
	}).Check()

	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorMatches, `cannot add relation \"wordpress:db mysql:server\": state changing too quickly; try again soon`)
}

func (s *StateSuite) TestAddRelationEnforcesRelationLimits(c *tc.C) {
	f := factory.NewFactory(s.State, s.StatePool)
	wordpressCharm := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: wordpressCharm})

	mysqlCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql"})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql1", Charm: mysqlCharm})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql2", Charm: mysqlCharm})

	eps, err := s.State.InferEndpoints("wordpress", "mysql1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorIsNil)

	eps, err = s.State.InferEndpoints("wordpress", "mysql2")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.State.AddRelation(eps...)
	c.Assert(err, tc.ErrorMatches, `cannot add relation \"wordpress:db mysql2:server\": establishing a new relation for wordpress:db would exceed its maximum relation limit of 1`)
}
