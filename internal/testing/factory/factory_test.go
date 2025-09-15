// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	"fmt"
	"regexp"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type factorySuite struct {
	statetesting.StateSuite
}

func TestFactorySuite(t *stdtesting.T) {
	testing.MgoTestPackage(t, &factorySuite{})
}

func (s *factorySuite) SetUpTest(c *tc.C) {
	s.NewPolicy = func(*state.State) state.Policy {
		return &statetesting.MockPolicy{
			GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
				return provider.CommonStorageProviders(), nil
			},
		}
	}
	s.StateSuite.SetUpTest(c)
}

func (s *factorySuite) TestMakeUserNil(c *tc.C) {
	user := s.Factory.MakeUser(c, nil)
	c.Assert(user.IsDisabled(), tc.IsFalse)

	saved, err := s.State.User(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Tag(), tc.Equals, user.Tag())
	c.Assert(saved.Name(), tc.Equals, user.Name())
	c.Assert(saved.DisplayName(), tc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), tc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), tc.Equals, user.DateCreated())
	c.Assert(saved.IsDisabled(), tc.Equals, user.IsDisabled())

	savedLastLogin, err := saved.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	lastLogin, err := user.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(savedLastLogin, tc.Equals, lastLogin)
}

func (s *factorySuite) TestMakeUserParams(c *tc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := s.Factory.MakeUser(c, nil)
	password := "sekrit"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator.Tag(),
		Password:    password,
	})
	c.Assert(user.IsDisabled(), tc.IsFalse)
	c.Assert(user.Name(), tc.Equals, username)
	c.Assert(user.DisplayName(), tc.Equals, displayName)
	c.Assert(user.CreatedBy(), tc.Equals, creator.UserTag().Name())
	c.Assert(user.PasswordValid(password), tc.IsTrue)

	saved, err := s.State.User(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Tag(), tc.Equals, user.Tag())
	c.Assert(saved.Name(), tc.Equals, user.Name())
	c.Assert(saved.DisplayName(), tc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), tc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), tc.Equals, user.DateCreated())
	c.Assert(saved.IsDisabled(), tc.Equals, user.IsDisabled())

	savedLastLogin, err := saved.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	lastLogin, err := user.LastLogin()
	c.Assert(err, tc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(savedLastLogin, tc.Equals, lastLogin)

	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *factorySuite) TestMakeUserInvalidCreator(c *tc.C) {
	invalidFunc := func() {
		s.Factory.MakeUser(c, &factory.UserParams{
			Name:        "bob",
			DisplayName: "Bob",
			Creator:     names.NewMachineTag("0"),
			Password:    "bob",
		})
	}

	c.Assert(invalidFunc, tc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.User(names.NewUserTag("bob"))
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(saved, tc.IsNil)
}

func (s *factorySuite) TestMakeUserNoModelUser(c *tc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := names.NewLocalUserTag("eric")
	password := "sekrit"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator,
		Password:    password,
		NoModelUser: true,
	})

	_, err := s.State.User(user.UserTag())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *factorySuite) TestMakeModelUserNil(c *tc.C) {
	modelUser := s.Factory.MakeModelUser(c, nil)
	saved, err := s.State.UserAccess(modelUser.UserTag, modelUser.Object)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Object.Id(), tc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, tc.Equals, modelUser.UserName)
	c.Assert(saved.DisplayName, tc.Equals, modelUser.DisplayName)
	c.Assert(saved.CreatedBy, tc.Equals, modelUser.CreatedBy)
}

func (s *factorySuite) TestMakeModelUserPartialParams(c *tc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar123", NoModelUser: true})
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User: "foobar123"})

	saved, err := s.State.UserAccess(modelUser.UserTag, modelUser.Object)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Object.Id(), tc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, tc.Equals, "foobar123")
	c.Assert(saved.DisplayName, tc.Equals, modelUser.DisplayName)
	c.Assert(saved.CreatedBy, tc.Equals, modelUser.CreatedBy)
}

func (s *factorySuite) TestMakeModelUserParams(c *tc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "foobar",
		Creator:     names.NewUserTag("createdby"),
		NoModelUser: true,
	})

	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User:        "foobar",
		CreatedBy:   names.NewUserTag("createdby"),
		DisplayName: "Foo Bar",
	})

	saved, err := s.State.UserAccess(modelUser.UserTag, s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Object.Id(), tc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, tc.Equals, "foobar")
	c.Assert(saved.CreatedBy.Id(), tc.Equals, "createdby")
	c.Assert(saved.DisplayName, tc.Equals, "Foo Bar")
}

func (s *factorySuite) TestMakeModelUserInvalidCreatedBy(c *tc.C) {
	invalidFunc := func() {
		s.Factory.MakeModelUser(c, &factory.ModelUserParams{
			User:      "bob",
			CreatedBy: names.NewMachineTag("0"),
		})
	}

	c.Assert(invalidFunc, tc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.UserAccess(names.NewLocalUserTag("bob"), s.Model.ModelTag())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(saved, tc.DeepEquals, permission.UserAccess{})
}

func (s *factorySuite) TestMakeModelUserNonLocalUser(c *tc.C) {
	creator := s.Factory.MakeUser(c, &factory.UserParams{Name: "created-by"})
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User:        "foobar@ubuntuone",
		DisplayName: "Foo Bar",
		CreatedBy:   creator.UserTag(),
	})

	saved, err := s.State.UserAccess(modelUser.UserTag, s.Model.ModelTag())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saved.Object.Id(), tc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, tc.Equals, "foobar@ubuntuone")
	c.Assert(saved.DisplayName, tc.Equals, "Foo Bar")
	c.Assert(saved.CreatedBy.Id(), tc.Equals, creator.UserTag().Id())
}

func (s *factorySuite) TestMakeMachineNil(c *tc.C) {
	machine, password := s.Factory.MakeMachineReturningPassword(c, nil)
	c.Assert(machine, tc.NotNil)

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Base().String(), tc.Equals, machine.Base().String())
	c.Assert(saved.Id(), tc.Equals, machine.Id())
	c.Assert(saved.Base().String(), tc.Equals, machine.Base().String())
	c.Assert(saved.Tag(), tc.Equals, machine.Tag())
	c.Assert(saved.Life(), tc.Equals, machine.Life())
	c.Assert(saved.Jobs(), tc.DeepEquals, machine.Jobs())
	c.Assert(saved.PasswordValid(password), tc.IsTrue)
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(savedInstanceId, tc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), tc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeMachine(c *tc.C) {
	base := state.UbuntuBase("12.10")
	jobs := []state.MachineJob{state.JobManageModel}
	password, err := utils.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	nonce := "some-nonce"
	id := instance.Id("some-id")
	volumes := []state.HostVolumeParams{{Volume: state.VolumeParams{Size: 1024}}}
	filesystems := []state.HostFilesystemParams{{
		Filesystem: state.FilesystemParams{Pool: "loop", Size: 2048},
	}}

	machine, pwd := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Base:        base,
		Jobs:        jobs,
		Password:    password,
		Nonce:       nonce,
		InstanceId:  id,
		Volumes:     volumes,
		Filesystems: filesystems,
	})
	c.Assert(machine, tc.NotNil)
	c.Assert(pwd, tc.Equals, password)

	c.Assert(machine.Base().String(), tc.Equals, base.String())
	c.Assert(machine.Jobs(), tc.DeepEquals, jobs)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstanceId, tc.Equals, id)
	c.Assert(machine.CheckProvisioned(nonce), tc.IsTrue)
	c.Assert(machine.PasswordValid(password), tc.IsTrue)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, tc.ErrorIsNil)
	assertVolume := func(name string, size uint64) {
		volume, err := sb.Volume(names.NewVolumeTag(name))
		c.Assert(err, tc.ErrorIsNil)
		volParams, ok := volume.Params()
		c.Assert(ok, tc.IsTrue)
		c.Assert(volParams, tc.DeepEquals, state.VolumeParams{Pool: "loop", Size: size})
		volAttachments, err := sb.VolumeAttachments(volume.VolumeTag())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(volAttachments, tc.HasLen, 1)
		c.Assert(volAttachments[0].Host(), tc.Equals, machine.Tag())
	}
	assertVolume(machine.Id()+"/0", 2048) // backing the filesystem
	assertVolume(machine.Id()+"/1", 1024)

	filesystem, err := sb.Filesystem(names.NewFilesystemTag(machine.Id() + "/0"))
	c.Assert(err, tc.ErrorIsNil)
	fsParams, ok := filesystem.Params()
	c.Assert(ok, tc.IsTrue)
	c.Assert(fsParams, tc.DeepEquals, state.FilesystemParams{Pool: "loop", Size: 2048})
	fsAttachments, err := sb.MachineFilesystemAttachments(machine.Tag().(names.MachineTag))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fsAttachments, tc.HasLen, 1)
	c.Assert(fsAttachments[0].Host(), tc.Equals, machine.Tag())

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Id(), tc.Equals, machine.Id())
	c.Assert(saved.Base().String(), tc.Equals, machine.Base().String())
	c.Assert(saved.Tag(), tc.Equals, machine.Tag())
	c.Assert(saved.Life(), tc.Equals, machine.Life())
	c.Assert(saved.Jobs(), tc.DeepEquals, machine.Jobs())
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(savedInstanceId, tc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), tc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeCharmNil(c *tc.C) {
	ch := s.Factory.MakeCharm(c, nil)
	c.Assert(ch, tc.NotNil)

	saved, err := s.State.Charm(ch.URL())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.URL(), tc.DeepEquals, ch.URL())
	c.Assert(saved.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(saved.StoragePath(), tc.Equals, ch.StoragePath())
	c.Assert(saved.BundleSha256(), tc.Equals, ch.BundleSha256())
}

func (s *factorySuite) TestMakeCharm(c *tc.C) {
	series := "quantal"
	name := "wordpress"
	revision := 13
	url := fmt.Sprintf("ch:%s/%s-%d", series, name, revision)
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: name,
		URL:  url,
	})
	c.Assert(ch, tc.NotNil)

	c.Assert(ch.URL(), tc.DeepEquals, url)

	saved, err := s.State.Charm(ch.URL())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.URL(), tc.DeepEquals, ch.URL())
	c.Assert(saved.Meta(), tc.DeepEquals, ch.Meta())
	c.Assert(saved.Meta().Name, tc.Equals, name)
	c.Assert(saved.StoragePath(), tc.Equals, ch.StoragePath())
	c.Assert(saved.BundleSha256(), tc.Equals, ch.BundleSha256())
}

func (s *factorySuite) TestMakeApplicationNil(c *tc.C) {
	application := s.Factory.MakeApplication(c, nil)
	c.Assert(application, tc.NotNil)

	saved, err := s.State.Application(application.Name())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Name(), tc.Equals, application.Name())
	c.Assert(saved.Tag(), tc.Equals, application.Tag())
	c.Assert(saved.Life(), tc.Equals, application.Life())
}

func (s *factorySuite) TestMakeApplication(c *tc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: charm,
	})
	c.Assert(application, tc.NotNil)

	c.Assert(application.Name(), tc.Equals, "wordpress")
	curl, _ := application.CharmURL()
	c.Assert(*curl, tc.Equals, charm.URL())

	saved, err := s.State.Application(application.Name())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Name(), tc.Equals, application.Name())
	c.Assert(saved.Tag(), tc.Equals, application.Tag())
	c.Assert(saved.Life(), tc.Equals, application.Life())
}

func (s *factorySuite) TestMakeUnitNil(c *tc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	c.Assert(unit, tc.NotNil)

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Name(), tc.Equals, unit.Name())
	c.Assert(saved.ApplicationName(), tc.Equals, unit.ApplicationName())
	c.Assert(saved.Base(), tc.DeepEquals, unit.Base())
	c.Assert(saved.Life(), tc.Equals, unit.Life())
}

func (s *factorySuite) TestMakeUnit(c *tc.C) {
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		SetCharmURL: true,
	})
	c.Assert(unit, tc.NotNil)

	c.Assert(unit.ApplicationName(), tc.Equals, application.Name())

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Name(), tc.Equals, unit.Name())
	c.Assert(saved.ApplicationName(), tc.Equals, unit.ApplicationName())
	c.Assert(saved.Base(), tc.DeepEquals, unit.Base())
	c.Assert(saved.Life(), tc.Equals, unit.Life())

	applicationCharmURL, _ := application.CharmURL()
	c.Assert(applicationCharmURL, tc.NotNil)
	unitCharmURL := saved.CharmURL()
	c.Assert(unitCharmURL, tc.NotNil)
	c.Assert(*unitCharmURL, tc.Equals, *applicationCharmURL)
}

func (s *factorySuite) TestMakeRelationNil(c *tc.C) {
	relation := s.Factory.MakeRelation(c, nil)
	c.Assert(relation, tc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Id(), tc.Equals, relation.Id())
	c.Assert(saved.Tag(), tc.Equals, relation.Tag())
	c.Assert(saved.Life(), tc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), tc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMakeRelation(c *tc.C) {
	s1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application1",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, tc.ErrorIsNil)

	s2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application2",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := s2.Endpoint("server")
	c.Assert(err, tc.ErrorIsNil)

	relation := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(relation, tc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(saved.Id(), tc.Equals, relation.Id())
	c.Assert(saved.Tag(), tc.Equals, relation.Tag())
	c.Assert(saved.Life(), tc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), tc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMakeModelNil(c *tc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	env, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	re := regexp.MustCompile(`^testmodel-\d+$`)
	c.Assert(re.MatchString(env.Name()), tc.IsTrue)
	c.Assert(env.UUID() == s.State.ModelUUID(), tc.IsFalse)
	origEnv, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.Owner(), tc.Equals, origEnv.Owner())

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := m.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-base"], tc.Equals, "")
	c.Assert(cfg.AllAttrs()["default-series"], tc.IsNil)
}

func (s *factorySuite) TestMakeModel(c *tc.C) {
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "owner",
	})
	params := &factory.ModelParams{
		Name:        "foo",
		Owner:       owner.UserTag(),
		ConfigAttrs: testing.Attrs{"default-base": "ubuntu@22.04"},
	}

	st := s.Factory.MakeModel(c, params)
	defer st.Close()

	env, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.Name(), tc.Equals, "foo")
	c.Assert(env.UUID() == s.State.ModelUUID(), tc.IsFalse)
	c.Assert(env.Owner(), tc.Equals, owner.UserTag())

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cfg, err := m.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-base"], tc.Equals, "ubuntu@22.04")
}

// TODO(stickupkid): We can remove this once we remove series.
func (s *factorySuite) TestMakeModelWithSeries(c *tc.C) {
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "owner",
	})
	params := &factory.ModelParams{
		Name:        "foo",
		Owner:       owner.UserTag(),
		ConfigAttrs: testing.Attrs{"default-series": "jammy"},
	}

	st := s.Factory.MakeModel(c, params)
	defer st.Close()

	env, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.Name(), tc.Equals, "foo")
	c.Assert(env.UUID() == s.State.ModelUUID(), tc.IsFalse)
	c.Assert(env.Owner(), tc.Equals, owner.UserTag())

	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	cfg, err := m.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-series"], tc.Equals, "jammy")
}
