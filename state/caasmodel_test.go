// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	k8stesting "github.com/juju/juju/caas/kubernetes/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

type CAASFixture struct {
	ConnSuite
}

func (s *CAASFixture) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.PatchValue(&k8sprovider.NewK8sClients, k8stesting.NoopFakeK8sClients)
}

// createTestModelConfig returns a new model config and its UUID for testing.
func (s *CAASFixture) createTestModelConfig(c *tc.C) (*config.Config, string) {
	return createTestModelConfig(c, s.modelTag.Id())
}

func (s *CAASFixture) newCAASModel(c *tc.C) (*state.CAASModel, *state.State) {
	st := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(*tc.C) { st.Close() })
	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	caasModel, err := model.CAASModel()
	c.Assert(err, tc.ErrorIsNil)
	return caasModel, st
}

type CAASModelSuite struct {
	CAASFixture
}

func TestCAASModelSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &CAASModelSuite{})
}

func (s *CAASModelSuite) TestNewModel(c *tc.C) {
	owner := s.Factory.MakeUser(c, nil)
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "caas-cloud",
		Type:      "kubernetes",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}, owner.Name())
	c.Assert(err, tc.ErrorIsNil)
	cfg, uuid := s.createTestModelConfig(c)
	modelTag := names.NewModelTag(uuid)
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	credTag := names.NewCloudCredentialTag(
		fmt.Sprintf("caas-cloud/%s/dummy-credential", owner.Name()))
	err = s.State.UpdateCloudCredential(credTag, cred)
	c.Assert(err, tc.ErrorIsNil)
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "caas-cloud",
		Config:                  cfg,
		Owner:                   owner.UserTag(),
		CloudCredential:         credTag,
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer st.Close()

	c.Assert(model.Type(), tc.Equals, state.ModelTypeCAAS)
	c.Assert(model.UUID(), tc.Equals, modelTag.Id())
	c.Assert(model.Tag(), tc.Equals, modelTag)
	c.Assert(model.ControllerTag(), tc.Equals, s.State.ControllerTag())
	c.Assert(model.Owner().Name(), tc.Equals, owner.Name())
	c.Assert(model.Name(), tc.Equals, "testing")
	c.Assert(model.Life(), tc.Equals, state.Alive)
	c.Assert(model.CloudRegion(), tc.Equals, "")
}

func (s *CAASModelSuite) TestDestroyEmptyModel(c *tc.C) {
	model, st := s.newCAASModel(c)
	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)
	c.Assert(st.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *CAASModelSuite) TestDestroyModel(c *tc.C) {
	model, st := s.newCAASModel(c)

	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, tc.ErrorIsNil)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, tc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	assertCleanupCount(c, st, 3)

	// App removal requires cluster resources to be cleared.
	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, st, 2)

	err = app.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	err = unit.Refresh()
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	assertDoesNotNeedCleanup(c, st)
}

func (s *CAASModelSuite) TestDestroyModelDestroyStorage(c *tc.C) {
	model, st := s.newCAASModel(c)
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	c.Assert(err, tc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, tc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: state.AddTestingCharmForSeries(c, st, "kubernetes", "storage-filesystem"),
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	si, err := sb.AllStorageInstances()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si, tc.HasLen, 1)
	fs, err := sb.AllFilesystems()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fs, tc.HasLen, 1)

	destroyStorage := true
	err = model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 4)

	c.Assert(app.Refresh(), tc.ErrorIsNil)
	c.Assert(app.Life(), tc.Equals, state.Dying)
	c.Assert(unit.Refresh(), tc.ErrorIsNil)
	c.Assert(unit.Life(), tc.Equals, state.Dying)

	// The uniter would call this when it sees it is dying.
	err = unit.EnsureDead()
	c.Assert(err, tc.ErrorIsNil)
	// The deployer or the caasapplicationprovisioner would call this once the unit is Dead.
	err = unit.Remove()
	c.Assert(err, tc.ErrorIsNil)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 2)

	// The caasapplicationprovisioner would call this when the app is gone from the cloud.
	err = app.ClearResources()
	c.Assert(err, tc.ErrorIsNil)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 2)

	si, err = sb.AllStorageInstances()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si, tc.HasLen, 0)
	fs, err = sb.AllFilesystems()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fs, tc.HasLen, 0)

	vols, err := sb.AllVolumes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vols, tc.HasLen, 1)
	c.Assert(vols[0].Life(), tc.Equals, state.Dying)
	// A storage provisioner would call this.
	err = sb.RemoveVolumeAttachment(unit.UnitTag(), vols[0].VolumeTag(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = sb.RemoveVolume(vols[0].VolumeTag())
	c.Assert(err, tc.ErrorIsNil)

	// Undertaker would call this.
	err = st.ProcessDyingModel()
	c.Assert(err, tc.ErrorIsNil)
	err = st.RemoveDyingModel()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CAASModelSuite) TestCAASModelWrongCloudRegion(c *tc.C) {
	cfg, _ := s.createTestModelConfig(c)
	_, _, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "fork",
		Config:                  cfg,
		Owner:                   names.NewUserTag("test@remote"),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, tc.ErrorMatches, `region "fork" not found \(expected one of \["dotty.region" "dummy-region" "nether-region" "unused-region"\]\)`)
}

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModels(c *tc.C) {
	st2 := s.Factory.MakeCAASModel(c, nil)
	defer st2.Close()

	f := factory.NewFactory(st2, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), tc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.Life(), tc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	// Cleanups for hosted model enqueued by controller model cleanups.
	assertNeedsCleanup(c, st2)
	assertCleanupRuns(c, st2)

	model2, err := st2.Model()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model2.Life(), tc.Equals, state.Dying)

	// App removal requires cluster resources to be cleared.
	err = app.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, st2, 2)

	c.Assert(st2.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), tc.ErrorIsNil)

	c.Assert(model2.Refresh(), tc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(model.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModelsWithResources(c *tc.C) {
	otherSt := s.Factory.MakeCAASModel(c, nil)
	defer otherSt.Close()

	assertModel := func(model *state.Model, st *state.State, life state.Life, expectedApps int) {
		c.Assert(model.Refresh(), tc.ErrorIsNil)
		c.Assert(model.Life(), tc.Equals, life)

		apps, err := st.AllApplications()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(apps, tc.HasLen, expectedApps)
	}

	// add some applications
	otherModel, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab"})
	c.Assert(err, tc.ErrorIsNil)

	f := factory.NewFactory(otherSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	args := state.AddApplicationArgs{
		Name: application.Name(),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm: ch,
	}
	application2, err := otherSt.AddApplication(args)
	c.Assert(err, tc.ErrorIsNil)

	controllerModel, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), tc.ErrorIsNil)

	assertCleanupCount(c, s.State, 2)
	assertAllMachinesDeadAndRemove(c, s.State)
	assertModel(controllerModel, s.State, state.Dying, 0)

	err = s.State.ProcessDyingModel()
	c.Assert(errors.Is(err, stateerrors.HasHostedModelsError), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, `hosting 1 other model`)

	assertCleanupCount(c, otherSt, 2)

	// App removal requires cluster resources to be cleared.
	err = application2.Refresh()
	c.Assert(err, tc.ErrorIsNil)
	err = application2.ClearResources()
	c.Assert(err, tc.ErrorIsNil)
	assertCleanupCount(c, otherSt, 2)

	assertModel(otherModel, otherSt, state.Dying, 0)
	c.Assert(otherSt.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(otherSt.RemoveDyingModel(), tc.ErrorIsNil)

	c.Assert(otherModel.Refresh(), tc.Satisfies, errors.IsNotFound)

	c.Assert(s.State.ProcessDyingModel(), tc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), tc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), tc.Satisfies, errors.IsNotFound)
}

func (s *CAASModelSuite) TestContainers(c *tc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab",
		Series: "kubernetes",
	})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	_, err := app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id1")})
	c.Assert(err, tc.ErrorIsNil)
	_, err = app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id2")})
	c.Assert(err, tc.ErrorIsNil)

	containers, err := m.Containers("provider-id1", "provider-id2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(containers, tc.HasLen, 2)
	var unitNames []string
	for _, c := range containers {
		unitNames = append(unitNames, c.Unit())
	}
	c.Assert(unitNames, tc.SameContents, []string{app.Name() + "/0", app.Name() + "/1"})
}

func (s *CAASModelSuite) TestUnitStatusNoPodSpec(c *tc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Status: &status.StatusInfo{
			Status:  status.Waiting,
			Message: status.MessageInitializingAgent,
		},
	})

	msWorkload := unitWorkloadStatus(c, m, unit.Name(), false)
	c.Check(msWorkload.Message, tc.Equals, "agent initialising")
	c.Check(msWorkload.Status, tc.Equals, status.Waiting)

	err := unit.SetStatus(status.StatusInfo{Status: status.Active, Message: "running"})
	c.Assert(err, tc.ErrorIsNil)
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), false)
	c.Check(msWorkload.Message, tc.Equals, "running")
	c.Check(msWorkload.Status, tc.Equals, status.Active)
}

func (s *CAASModelSuite) TestCloudContainerStatus(c *tc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Status: &status.StatusInfo{
			Status:  status.Active,
			Message: "Unit Active",
		},
	})

	// Cloud container overrides Allocating unit
	setCloudContainerStatus(c, unit, status.Allocating, "k8s allocating")
	msWorkload := unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "k8s allocating")
	c.Check(msWorkload.Status, tc.Equals, status.Allocating)

	// Cloud container error overrides unit status
	setCloudContainerStatus(c, unit, status.Error, "k8s charm error")
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "k8s charm error")
	c.Check(msWorkload.Status, tc.Equals, status.Error)

	// Unit status must be used.
	setCloudContainerStatus(c, unit, status.Running, "k8s idle")
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "Unit Active")
	c.Check(msWorkload.Status, tc.Equals, status.Active)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Blocked, "POD storage issue")
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "POD storage issue")
	c.Check(msWorkload.Status, tc.Equals, status.Blocked)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Waiting, "Building the bits")
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "Building the bits")
	c.Check(msWorkload.Status, tc.Equals, status.Waiting)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Running, "Bits have been built")
	msWorkload = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Check(msWorkload.Message, tc.Equals, "Unit Active")
	c.Check(msWorkload.Status, tc.Equals, status.Active)
}

func (s *CAASModelSuite) TestCloudContainerHistoryOverwrite(c *tc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	unit := f.MakeUnit(c, &factory.UnitParams{})

	workloadStatus := unitWorkloadStatus(c, m, unit.Name(), true)
	c.Assert(workloadStatus.Message, tc.Equals, status.MessageWaitForContainer)
	c.Assert(workloadStatus.Status, tc.Equals, status.Waiting)
	statusHistory, err := unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusHistory, tc.HasLen, 1)
	c.Assert(statusHistory[0].Message, tc.Equals, status.MessageInstallingAgent)
	c.Assert(statusHistory[0].Status, tc.Equals, status.Waiting)

	err = unit.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "Unit Active",
	})
	c.Assert(err, tc.ErrorIsNil)
	unitStatus, err := unit.Status()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitStatus.Message, tc.Equals, "Unit Active")
	c.Assert(unitStatus.Status, tc.Equals, status.Active)

	// Now that status is stored as Active, but displayed (and in history)
	// as waiting for container, once we set cloud container status as active
	// it must show active from the unit (incl. history)
	setCloudContainerStatus(c, unit, status.Running, "Container Active")
	workloadStatus = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Assert(workloadStatus.Message, tc.Equals, "Unit Active")
	c.Assert(workloadStatus.Status, tc.Equals, status.Active)
	statusHistory, err = unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusHistory, tc.HasLen, 2)
	c.Assert(statusHistory[0].Message, tc.Equals, "Unit Active")
	c.Assert(statusHistory[0].Status, tc.Equals, status.Active)
	c.Assert(statusHistory[1].Message, tc.Equals, status.MessageInstallingAgent)
	c.Assert(statusHistory[1].Status, tc.Equals, status.Waiting)

	err = unit.SetStatus(status.StatusInfo{
		Status:  status.Waiting,
		Message: "This is a different message",
	})
	c.Assert(err, tc.ErrorIsNil)
	workloadStatus = unitWorkloadStatus(c, m, unit.Name(), true)
	c.Assert(workloadStatus.Message, tc.Equals, "This is a different message")
	c.Assert(workloadStatus.Status, tc.Equals, status.Waiting)
	statusHistory, err = unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusHistory, tc.HasLen, 3)
	c.Assert(statusHistory[0].Message, tc.Equals, "This is a different message")
	c.Assert(statusHistory[0].Status, tc.Equals, status.Waiting)
	c.Assert(statusHistory[1].Message, tc.Equals, "Unit Active")
	c.Assert(statusHistory[1].Status, tc.Equals, status.Active)
	c.Assert(statusHistory[2].Message, tc.Equals, status.MessageInstallingAgent)
	c.Assert(statusHistory[2].Status, tc.Equals, status.Waiting)
}

func unitWorkloadStatus(c *tc.C, model *state.CAASModel, unitName string, expectWorkload bool) status.StatusInfo {
	ms, err := model.LoadModelStatus()
	c.Assert(err, tc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unitName, expectWorkload)
	c.Assert(err, tc.ErrorIsNil)
	return msWorkload
}

func setCloudContainerStatus(c *tc.C, unit *state.Unit, statusCode status.Status, message string) {
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		unit.UpdateOperation(state.UnitUpdateProperties{
			CloudContainerStatus: &status.StatusInfo{Status: statusCode, Message: message},
		})}
	app, err := unit.Application()
	c.Assert(err, tc.ErrorIsNil)
	err = app.UpdateUnits(&updateUnits)
	c.Assert(err, tc.ErrorIsNil)
}
