// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"reflect"
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type validatorSuite struct {
	bindings    *MockBindings
	machine     *MockMachine
	model       *MockModel
	repo        *MockRepository
	repoFactory *MockRepositoryFactory
	state       *MockDeployFromRepositoryState
}

func TestDeployRepositorySuite(t *tctesting.T) {
	tc.Run(t, &deployRepositorySuite{})
}
func TestValidatorSuite(t *tctesting.T) {
	tc.Run(t, &validatorSuite{})
}

func (s *validatorSuite) TestValidateSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.model.EXPECT().UUID().Return("")

	// getCharm
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	dt, errs := s.getValidator().validate(arg)
	c.Assert(errs, tc.HasLen, 0, tc.Commentf("%s", pretty.Sprint(errs)))
	c.Assert(dt, tc.DeepEquals, deployTemplate{
		applicationName: "test-charm",
		charm:           corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata),
		charmURL:        resultURL,
		numUnits:        1,
		origin:          resolvedOrigin,
	})
}

func (s *validatorSuite) TestValidateIAASAttachStorageFail(c *tc.C) {
	argStorageNames := []string{"one-0"}
	expectedStorageTags := []names.StorageTag{}

	s.testValidateAttachStorage(
		c, argStorageNames, expectedStorageTags,
		func() DeployFromRepositoryValidator { return s.iaasDeployFromRepositoryValidator() },
		errors.NotValid,
	)
}

func (s *validatorSuite) TestValidateIAASAttachStorageSuccess(c *tc.C) {
	argStorageNames := []string{"one/0", "two/3"}
	expectedStorageTags := []names.StorageTag{names.NewStorageTag("one/0"), names.NewStorageTag("two/3")}

	s.testValidateAttachStorage(
		c, argStorageNames, expectedStorageTags,
		func() DeployFromRepositoryValidator { return s.iaasDeployFromRepositoryValidator() },
		"",
	)
}

func (s *validatorSuite) TestValidateCAASAttachStorageFail(c *tc.C) {
	argStorageNames := []string{"one-0"}
	expectedStorageTags := []names.StorageTag{}
	s.testValidateAttachStorage(
		c, argStorageNames, expectedStorageTags,
		func() DeployFromRepositoryValidator { return s.caasDeployFromRepositoryValidator(c) },
		errors.NotValid,
	)
}

func (s *validatorSuite) TestValidateCAASAttachStorageSuccess(c *tc.C) {
	argStorageNames := []string{"one/0", "two/3"}
	expectedStorageTags := []names.StorageTag{names.NewStorageTag("one/0"), names.NewStorageTag("two/3")}

	s.testValidateAttachStorage(
		c, argStorageNames, expectedStorageTags,
		func() DeployFromRepositoryValidator { return s.caasDeployFromRepositoryValidator(c) },
		"",
	)
}

func (s *validatorSuite) testValidateAttachStorage(c *tc.C, argStorage []string, expectedStorageTags []names.StorageTag, getValidatorFunc func() DeployFromRepositoryValidator, expectedErr errors.ConstError) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.model.EXPECT().UUID().Return("")
	// getCharm
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))

	arg := params.DeployFromRepositoryArg{
		CharmName:     "testcharm",
		AttachStorage: argStorage,
	}
	dt, errs := getValidatorFunc().ValidateArg(arg)
	if expectedErr == "" {
		c.Assert(errs, tc.HasLen, 0)
		c.Assert(dt, tc.DeepEquals, deployTemplate{
			applicationName: "test-charm",
			charm:           corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata),
			charmURL:        resultURL,
			numUnits:        1,
			origin:          resolvedOrigin,
			attachStorage:   expectedStorageTags,
		})
	} else {
		c.Assert(errs, tc.HasLen, 1)
		c.Assert(errors.Is(errs[0], expectedErr), tc.IsTrue)
	}
}

func (s *validatorSuite) TestValidatePlacementSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	// getCharm
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.model.EXPECT().UUID().Return("")

	// Placement
	s.state.EXPECT().Machine("0").Return(s.machine, nil).Times(2)
	s.machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)
	s.machine.EXPECT().IsParentLockedForSeriesUpgrade().Return(false, nil)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	})
	hwc := &instance.HardwareCharacteristics{Arch: strptr("amd64")}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil)
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
		Placement: []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
	}
	dt, errs := s.getValidator().validate(arg)
	c.Assert(errs, tc.HasLen, 0)
	c.Assert(dt, tc.DeepEquals, deployTemplate{
		applicationName: "test-charm",
		charm:           corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata),
		charmURL:        resultURL,
		numUnits:        1,
		origin:          resolvedOrigin,
		placement:       arg.Placement,
	})
}

func (s *validatorSuite) TestValidateEndpointBindingSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	// getCharm
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.model.EXPECT().UUID().Return("")

	// state bindings
	endpointMap := map[string]string{"to": "from"}
	s.bindings.EXPECT().Map().Return(endpointMap)
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))

	arg := params.DeployFromRepositoryArg{
		CharmName:        "testcharm",
		EndpointBindings: endpointMap,
	}
	dt, errs := s.getValidator().validate(arg)
	c.Assert(errs, tc.HasLen, 0)
	c.Assert(dt, tc.DeepEquals, deployTemplate{
		applicationName: "test-charm",
		charm:           corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata),
		charmURL:        resultURL,
		endpoints:       endpointMap,
		numUnits:        1,
		origin:          resolvedOrigin,
	})
}

func (s *validatorSuite) TestValidateEndpointBindingFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	// getCharm
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.model.EXPECT().UUID().Return("")

	// state bindings
	endpointMap := map[string]string{"to": "from"}
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))

	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repo, nil).AnyTimes()
	v := &deployFromRepositoryValidator{
		model:       s.model,
		state:       s.state,
		repoFactory: s.repoFactory,
		newStateBindings: func(st state.EndpointBinding, givenMap map[string]string) (Bindings, error) {
			return nil, errors.NotFoundf("space")
		},
	}

	arg := params.DeployFromRepositoryArg{
		CharmName:        "testcharm",
		EndpointBindings: endpointMap,
	}
	_, errs := v.validate(arg)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIs, errors.NotFound)
}

func (s *validatorSuite) expectSimpleValidate() {
	// createOrigin
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig())).AnyTimes()
}

func (s *validatorSuite) TestResolveCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{
		Arch: strptr("arm64"),
	}, nil)

	obtained, err := s.getValidator().resolveCharm(curl, origin, false, false, constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.URL, tc.DeepEquals, resultURL)
	c.Assert(obtained.EssentialMetadata.ResolvedOrigin, tc.DeepEquals, resolvedOrigin)
}

func (s *validatorSuite) TestResolveCharmArchAll(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "all", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)

	obtained, err := s.getValidator().resolveCharm(curl, origin, false, false, constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.URL, tc.DeepEquals, resultURL)
	expectedOrigin := resolvedOrigin
	expectedOrigin.Platform.Architecture = "arm64"
	c.Assert(obtained.EssentialMetadata.ResolvedOrigin, tc.DeepEquals, expectedOrigin)
}

func (s *validatorSuite) TestResolveCharmUnsupportedSeriesErrorForce(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	supportedSeries := []string{"focal"}
	newErr := charm.NewUnsupportedSeriesError("jammy", supportedSeries)
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, newErr)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)

	obtained, err := s.getValidator().resolveCharm(curl, origin, true, false, constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.URL, tc.DeepEquals, resultURL)
	c.Assert(obtained.EssentialMetadata.ResolvedOrigin, tc.DeepEquals, resolvedOrigin)
}

func (s *validatorSuite) TestResolveCharmUnsupportedSeriesError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	supportedSeries := []string{"focal"}
	newErr := charm.NewUnsupportedSeriesError("jammy", supportedSeries)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(corecharm.ResolvedDataForDeploy{}, newErr)

	_, err := s.getValidator().resolveCharm(curl, origin, false, false, constraints.Value{})
	c.Assert(err, tc.ErrorMatches, `series "jammy" not supported by charm, supported series are: focal. Use --force to deploy the charm anyway.`)
}

func (s *validatorSuite) TestResolveCharmExplicitBaseErrorWhenUserImageID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("arm64")}, nil)

	_, err := s.getValidator().resolveCharm(curl, origin, false, false, constraints.Value{ImageID: strptr("ubuntu-bf2")})
	c.Assert(err, tc.ErrorMatches, `base must be explicitly provided when image-id constraint is used`)
}

func (s *validatorSuite) TestResolveCharmExplicitBaseErrorWhenModelImageID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04/stable"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{
		Arch:    strptr("arm64"),
		ImageID: strptr("ubuntu-bf2"),
	}, nil)

	_, err := s.getValidator().resolveCharm(curl, origin, false, false, constraints.Value{})
	c.Assert(err, tc.ErrorMatches, `base must be explicitly provided when image-id constraint is used`)
}

func (s *validatorSuite) TestCreateOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
		Revision:  intptr(7),
	}
	curl, origin, defaultBase, err := s.getValidator().createOrigin(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl, tc.DeepEquals, charm.MustParseURL("ch:testcharm-7"))
	c.Assert(origin, tc.DeepEquals, corecharm.Origin{
		Source:   "charm-hub",
		Revision: intptr(7),
		Channel:  &corecharm.DefaultChannel,
		Platform: corecharm.Platform{Architecture: "amd64"},
	})
	c.Assert(defaultBase, tc.IsFalse)
}

func (s *validatorSuite) TestCreateOriginChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
		Revision:  intptr(7),
		Channel:   strptr("yoga/candidate"),
	}
	curl, origin, defaultBase, err := s.getValidator().createOrigin(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl, tc.DeepEquals, charm.MustParseURL("ch:testcharm-7"))
	expectedChannel := corecharm.MustParseChannel("yoga/candidate")
	c.Assert(origin, tc.DeepEquals, corecharm.Origin{
		Source:   "charm-hub",
		Revision: intptr(7),
		Channel:  &expectedChannel,
		Platform: corecharm.Platform{Architecture: "amd64"},
	})
	c.Assert(defaultBase, tc.IsFalse)
}

func (s *validatorSuite) TestGetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))
	// getCharm

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	obtainedURL, obtainedOrigin, obtainedCharm, err := s.getValidator().getCharm(arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOrigin, tc.DeepEquals, resolvedOrigin)
	c.Assert(obtainedCharm, tc.DeepEquals, corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata))
	c.Assert(obtainedURL, tc.DeepEquals, resultURL)
}

func (s *validatorSuite) TestGetCharmAlreadyDeployed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.expectSimpleValidate()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	ch := NewMockCharm(ctrl)
	s.state.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	obtainedURL, obtainedOrigin, obtainedCharm, err := s.getValidator().getCharm(arg)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOrigin, tc.DeepEquals, resolvedOrigin)
	c.Assert(obtainedCharm, tc.NotNil)
	c.Assert(obtainedURL, tc.DeepEquals, resultURL)
}

func (s *validatorSuite) TestGetCharmFindsBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "bundle",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	_, _, _, err := s.getValidator().getCharm(arg)
	c.Assert(errors.Is(err, errors.BadRequest), tc.IsTrue)
}

func (s *validatorSuite) TestGetCharmNoJujuControllerCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/juju-qa-test-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	resolvedData.EssentialMetadata.Meta.Name = "juju-controller"
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}
	_, _, _, err := s.getValidator().getCharm(arg)
	c.Assert(errors.Is(err, errors.NotSupported), tc.IsTrue, tc.Commentf("%+v", err))
}

func (s *validatorSuite) TestDeducePlatformSimple(c *tc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("amd64")}, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	arg := params.DeployFromRepositoryArg{CharmName: "testme"}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.IsNil)
	c.Assert(usedModelDefaultBase, tc.IsFalse)
	c.Assert(plat, tc.DeepEquals, corecharm.Platform{Architecture: "amd64"})
}

func (s *validatorSuite) TestDeducePlatformArgArchBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Cons:      constraints.Value{Arch: strptr("arm64")},
		Base: &params.Base{
			Name:    "ubuntu",
			Channel: "22.10",
		},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.IsNil)
	c.Assert(usedModelDefaultBase, tc.IsFalse)
	c.Assert(plat, tc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "22.10/stable",
	})
}

func (s *validatorSuite) TestDeducePlatformModelDefaultBase(c *tc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"default-base": "ubuntu@22.04",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, tc.ErrorIsNil)
	s.model.EXPECT().Config().Return(cfg, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.IsNil)
	c.Assert(usedModelDefaultBase, tc.IsTrue)
	c.Assert(plat, tc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "22.04/stable",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementSimpleFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine("0").Return(s.machine, nil)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	})
	hwc := &instance.HardwareCharacteristics{Arch: strptr("arm64")}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: "0"},
			{Scope: "lxd"},
		},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.IsNil)
	c.Assert(usedModelDefaultBase, tc.IsFalse)
	c.Assert(plat, tc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "22.04",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementNoPanic(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.machine.EXPECT().Id().Return("5/lxd/6")
	s.state.EXPECT().Machine("5/lxd/6").Return(s.machine, nil)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	})
	hwc := &instance.HardwareCharacteristics{}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: "5/lxd/6"},
			{Scope: "lxd"},
		},
	}
	_, _, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.NotNil)
}

func (s *validatorSuite) TestDeducePlatformPlacementSimpleNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	//model constraint default
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{Arch: strptr("amd64")}, nil)
	s.state.EXPECT().Machine("0/lxd/0").Return(nil, errors.NotFoundf("machine 0/lxd/0"))

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{{
			Scope: instance.MachineScope, Directive: "0/lxd/0",
		}},
	}
	_, _, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *validatorSuite) TestResolvedCharmValidationSubordinate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	ch := NewMockCharm(ctrl)
	meta := &charm.Meta{
		Name:        "testcharm",
		Subordinate: true,
	}
	ch.EXPECT().Config().Return(nil)
	ch.EXPECT().Meta().Return(meta).AnyTimes()
	arg := params.DeployFromRepositoryArg{
		NumUnits: intptr(1),
	}
	dt, err := s.getValidator().resolvedCharmValidation(ch, arg)
	c.Assert(err, tc.HasLen, 0)
	c.Assert(dt.numUnits, tc.Equals, 0)
}

func (s *validatorSuite) TestDeducePlatformPlacementMutipleMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine(gomock.Any()).Return(s.machine, nil).Times(3)
	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	}).Times(3)
	hwc := &instance.HardwareCharacteristics{Arch: strptr("arm64")}
	s.machine.EXPECT().HardwareCharacteristics().Return(hwc, nil).Times(3)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: "0"},
			{Scope: instance.MachineScope, Directive: "1"},
			{Scope: instance.MachineScope, Directive: "3"},
		},
	}
	plat, usedModelDefaultBase, err := s.getValidator().deducePlatform(arg)
	c.Assert(err, tc.IsNil)
	c.Assert(usedModelDefaultBase, tc.IsFalse)
	c.Assert(plat, tc.DeepEquals, corecharm.Platform{
		Architecture: "arm64",
		OS:           "ubuntu",
		Channel:      "22.04",
	})
}

func (s *validatorSuite) TestDeducePlatformPlacementMutipleMatchFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{}, nil)
	s.state.EXPECT().Machine(gomock.Any()).Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().Base().Return(
		state.Base{
			OS:      "ubuntu",
			Channel: "22.04",
		}).AnyTimes()
	gomock.InOrder(
		s.machine.EXPECT().HardwareCharacteristics().Return(
			&instance.HardwareCharacteristics{Arch: strptr("arm64")},
			nil),
		s.machine.EXPECT().HardwareCharacteristics().Return(
			&instance.HardwareCharacteristics{Arch: strptr("amd64")},
			nil),
	)

	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: "0"},
			{Scope: instance.MachineScope, Directive: "1"},
		},
	}
	_, _, err := s.getValidator().deducePlatform(arg)
	c.Assert(errors.Is(err, errors.BadRequest), tc.IsTrue, tc.Commentf("%+v", err))
}

var configYaml = `
testme:
  optionOne: one
  optionTwo: 8
`[1:]

func (s *validatorSuite) TestAppCharmSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.model.EXPECT().Type().Return(state.ModelTypeIAAS)

	cfg := charm.NewConfig()
	cfg.Options = map[string]charm.Option{
		"optionOne": {
			Type:        "string",
			Description: "option one",
		},
		"optionTwo": {
			Type:        "int",
			Description: "option two",
		},
	}

	appCfgSchema, _, err := applicationConfigSchema(state.ModelTypeIAAS)
	c.Assert(err, tc.ErrorIsNil)

	expectedAppConfig, err := coreconfig.NewConfig(map[string]interface{}{"trust": true}, appCfgSchema, nil)
	c.Assert(err, tc.ErrorIsNil)

	appConfig, charmConfig, err := s.getValidator().appCharmSettings("testme", true, cfg, configYaml)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appConfig, tc.DeepEquals, expectedAppConfig)
	c.Assert(charmConfig["optionOne"], tc.DeepEquals, "one")
	c.Assert(charmConfig["optionTwo"], tc.DeepEquals, int64(8))
}

// The purpose of the resolveResourcesArgsMatcher is
// to compare the slices of resource.Resource, b/c the
// order is non-deterministic.
type resolveResourcesArgsMatcher struct {
	c        *tc.C
	expected *[]resource.Resource
}

func (m resolveResourcesArgsMatcher) String() string {
	return "match ResolveResources arg map"
}

func (m resolveResourcesArgsMatcher) Matches(x interface{}) bool {
	obtainedSlice, ok := x.([]resource.Resource)
	if !ok {
		return false
	}

	m.c.Assert(obtainedSlice, tc.HasLen, len(*m.expected))
	// Unfortunately the jc.SameContents don't work here
	// because resource.Resource is unhashable
	for _, r := range obtainedSlice {
		found := false
		for _, exR := range *m.expected {
			if reflect.DeepEqual(r, exR) {
				found = true
				break
			}
		}
		m.c.Assert(found, tc.Equals, true)
	}
	return true
}

func (s *validatorSuite) TestResolveResourcesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	curl := charm.MustParseURL("testcharm")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	// Resource 1 : file upload from client
	meta1 := resource.Meta{
		Name:        "foo-resource",
		Type:        resource.TypeFile,
		Path:        "foo.txt",
		Description: "bar",
	}
	res := resource.Resource{
		Meta:     meta1,
		Origin:   resource.OriginUpload,
		Revision: -1,
	}
	// Resource 2 : store resource with --resource <revision> flag
	meta2 := resource.Meta{
		Name:        "foo-resource2",
		Type:        resource.TypeFile,
		Path:        "foo.txt",
		Description: "bar",
	}
	res2 := resource.Resource{
		Meta:     meta2,
		Origin:   resource.OriginStore,
		Revision: 3,
	}
	// Resource 3 : store resource without the --resource flag
	// (revision is reported by the store)
	meta3 := resource.Meta{
		Name:        "foo-resource3",
		Type:        resource.TypeFile,
		Path:        "foo.txt",
		Description: "bar",
	}
	res3 := resource.Resource{
		Meta:     meta3,
		Origin:   resource.OriginStore,
		Revision: -1,
	}

	resMeta := map[string]resource.Meta{"foo-file": meta1, "foo-file2": meta2, "store-file-res": meta3}
	resArgs := []resource.Resource{res, res2, res3}
	// Note that for the Resource 3, in the args res3 has revision -1, and the result below has revision 4
	r4 := resource.Resource{
		Meta:     meta3,
		Origin:   resource.OriginStore,
		Revision: 4,
	}
	resResult := []resource.Resource{res, res2, r4}
	// First one of below is the file upload for Resource 1, the second is the revision for Resource 2e
	deployResArg := map[string]string{"foo-file": "bar", "foo-file2": "3"}

	s.repo.EXPECT().ResolveResources(resolveResourcesArgsMatcher{c: c, expected: &resArgs}, corecharm.CharmID{URL: curl, Origin: origin}).Return(resResult, nil)
	resources, pendingResourceUploads, resolveResErr := s.getValidator().resolveResources(curl, origin, deployResArg, resMeta)
	pendUp := &params.PendingResourceUpload{
		Name:     "foo-resource",
		Type:     "file",
		Filename: "bar",
	}
	c.Assert(resolveResErr, tc.ErrorIsNil)
	c.Assert(resources, tc.DeepEquals, resResult)
	c.Assert(pendingResourceUploads, tc.DeepEquals, []*params.PendingResourceUpload{pendUp})
}

func (s *validatorSuite) TestCaasDeployFromRepositoryValidator(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	// resolveCharm
	curl := charm.MustParseURL("testcharm")
	resultURL := charm.MustParseURL("ch:amd64/jammy/testcharm-4")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Channel:  &charm.Channel{Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:   "charm-hub",
		Type:     "charm",
		Channel:  &charm.Channel{Track: "default", Risk: "stable"},
		Platform: corecharm.Platform{Architecture: "amd64", OS: "ubuntu", Channel: "22.04"},
		Revision: intptr(4),
	}
	charmID := corecharm.CharmID{URL: curl, Origin: origin}
	resolvedData := getResolvedData(resultURL, resolvedOrigin)
	s.repo.EXPECT().ResolveForDeploy(charmID).Return(resolvedData, nil)
	s.repo.EXPECT().ResolveResources(nil, corecharm.CharmID{URL: resultURL, Origin: resolvedOrigin}).Return(nil, nil)
	s.state.EXPECT().Charm(gomock.Any()).Return(nil, errors.NotFoundf("charm"))
	s.state.EXPECT().ModelConstraints().Return(constraints.Value{
		Arch: strptr("arm64"),
	}, nil)
	s.model.EXPECT().UUID().Return("")

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}

	obtainedDT, errs := s.caasDeployFromRepositoryValidator(c).ValidateArg(arg)
	c.Assert(errs, tc.HasLen, 0)
	c.Assert(obtainedDT, tc.DeepEquals, deployTemplate{
		applicationName: "test-charm",
		charm:           corecharm.NewCharmInfoAdapter(resolvedData.EssentialMetadata),
		charmURL:        resultURL,
		numUnits:        1,
		origin:          resolvedOrigin,
	})
}

func (s *validatorSuite) TestIaaSDeployFromRepositoryFailResolveCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	s.repo.EXPECT().ResolveForDeploy(gomock.Any()).Return(corecharm.ResolvedDataForDeploy{}, fmt.Errorf("fail resolve"))
	s.model.EXPECT().UUID().Return("")

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}

	_, errs := s.iaasDeployFromRepositoryValidator().ValidateArg(arg)
	c.Assert(errs, tc.HasLen, 1)
}

func (s *validatorSuite) TestCaaSDeployFromRepositoryFailResolveCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSimpleValidate()
	s.repo.EXPECT().ResolveForDeploy(gomock.Any()).Return(corecharm.ResolvedDataForDeploy{}, fmt.Errorf("fail resolve"))
	s.model.EXPECT().UUID().Return("")

	arg := params.DeployFromRepositoryArg{
		CharmName: "testcharm",
	}

	_, errs := s.caasDeployFromRepositoryValidator(c).ValidateArg(arg)
	c.Assert(errs, tc.HasLen, 1)
}

func getResolvedData(resultURL *charm.URL, resolvedOrigin corecharm.Origin) corecharm.ResolvedDataForDeploy {
	expMeta := &charm.Meta{
		Name: "test-charm",
	}
	expManifest := &charm.Manifest{Bases: []charm.Base{
		{Name: "ubuntu", Channel: charm.Channel{Track: "22.04", Risk: "stable"}},
		{Name: "ubuntu", Channel: charm.Channel{Track: "20.04", Risk: "stable"}},
	}}
	expConfig := new(charm.Config)
	essMeta := corecharm.EssentialMetadata{
		Meta:           expMeta,
		Manifest:       expManifest,
		Config:         expConfig,
		ResolvedOrigin: resolvedOrigin,
	}
	return corecharm.ResolvedDataForDeploy{
		URL:               resultURL,
		EssentialMetadata: essMeta,
		Resources:         nil,
	}
}

func (s *validatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.bindings = NewMockBindings(ctrl)
	s.machine = NewMockMachine(ctrl)
	s.model = NewMockModel(ctrl)
	s.repo = NewMockRepository(ctrl)
	s.repoFactory = NewMockRepositoryFactory(ctrl)
	s.state = NewMockDeployFromRepositoryState(ctrl)
	return ctrl
}

func (s *validatorSuite) getValidator() *deployFromRepositoryValidator {
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repo, nil).AnyTimes()
	return &deployFromRepositoryValidator{
		model:       s.model,
		state:       s.state,
		repoFactory: s.repoFactory,
		newStateBindings: func(st state.EndpointBinding, givenMap map[string]string) (Bindings, error) {
			return s.bindings, nil
		},
	}
}

func (s *validatorSuite) caasDeployFromRepositoryValidator(c *tc.C) caasDeployFromRepositoryValidator {
	return caasDeployFromRepositoryValidator{
		validator: s.getValidator(),
		caasPrecheckFunc: func(dt deployTemplate) error {
			// Do a quick check to ensure the expected deployTemplate
			// has been passed.
			c.Assert(dt.applicationName, tc.Equals, "test-charm")
			return nil
		},
	}
}

func (s *validatorSuite) iaasDeployFromRepositoryValidator() iaasDeployFromRepositoryValidator {
	return iaasDeployFromRepositoryValidator{
		validator: s.getValidator(),
	}
}

func strptr(s string) *string {
	return &s
}

func intptr(i int) *int {
	return &i
}

type deployRepositorySuite struct {
	application *MockApplication
	charm       *MockCharm
	state       *MockDeployFromRepositoryState
	validator   *MockDeployFromRepositoryValidator
}

func (s *deployRepositorySuite) TestDeployFromRepositoryAPI(c *tc.C) {
	defer s.setupMocks(c).Finish()
	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	template := deployTemplate{
		applicationName: "metadata-name",
		charm:           corecharm.NewCharmInfoAdapter(corecharm.EssentialMetadata{}),
		charmURL:        charm.MustParseURL("ch:amd64/jammy/testme-5"),
		endpoints:       map[string]string{"to": "from"},
		numUnits:        1,
		origin: corecharm.Origin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel:  &charm.Channel{Risk: "stable"},
			Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04"),
		},
		placement: []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
	}
	s.validator.EXPECT().ValidateArg(arg).Return(template, nil)
	info := state.CharmInfo{
		Charm: template.charm,
		ID:    "ch:amd64/jammy/testme-5",
	}

	s.state.EXPECT().AddCharmMetadata(info).Return(s.charm, nil)

	addAppArgs := state.AddApplicationArgs{
		Name: "metadata-name",
		// the app.Charm is casted into a state.Charm in the code
		// we mock it separately here (s.charm above), the test works
		// thanks to the addApplicationArgsMatcher used below
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel: &state.Channel{
				Risk: "stable",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		Devices:          map[string]state.DeviceConstraints{},
		EndpointBindings: map[string]string{"to": "from"},
		NumUnits:         1,
		Placement:        []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
		Resources:        map[string]string{},
		Storage:          map[string]state.StorageConstraints{},
	}
	s.state.EXPECT().AddApplication(addApplicationArgsMatcher{c: c, expectedArgs: addAppArgs}).Return(s.application, nil)

	deployFromRepositoryAPI := s.getDeployFromRepositoryAPI()

	obtainedInfo, resources, errs := deployFromRepositoryAPI.DeployFromRepository(arg)
	c.Assert(errs, tc.HasLen, 0)
	c.Assert(resources, tc.HasLen, 0)
	c.Assert(obtainedInfo, tc.DeepEquals, params.DeployFromRepositoryInfo{
		Architecture:     "amd64",
		Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
		Channel:          "stable",
		EffectiveChannel: nil,
		Name:             "metadata-name",
		Revision:         5,
	})
}

// The reason for this matcher is that the AddApplicationArgs.Charm is
// obtained by casting application.Charm into a state.Charm, but we
// can't do that cast with a MockCharm
type addApplicationArgsMatcher struct {
	c            *tc.C
	expectedArgs state.AddApplicationArgs
}

func (m addApplicationArgsMatcher) String() string {
	return "match AddApplicationArgs"
}

func (m addApplicationArgsMatcher) Matches(x interface{}) bool {

	oA, ok := x.(state.AddApplicationArgs)
	if !ok {
		return false
	}

	eA := m.expectedArgs
	// Check everything but the Charm
	m.c.Assert(oA.Name, tc.DeepEquals, eA.Name)
	m.c.Assert(oA.ApplicationConfig, tc.DeepEquals, eA.ApplicationConfig)
	m.c.Assert(oA.NumUnits, tc.DeepEquals, eA.NumUnits)
	m.c.Assert(oA.Constraints, tc.DeepEquals, eA.Constraints)
	m.c.Assert(oA.Storage, tc.DeepEquals, eA.Storage)
	m.c.Assert(oA.Devices, tc.DeepEquals, eA.Devices)
	m.c.Assert(eA.AttachStorage, tc.DeepEquals, eA.AttachStorage)
	m.c.Assert(oA.EndpointBindings, tc.DeepEquals, eA.EndpointBindings)
	m.c.Assert(oA.CharmConfig, tc.DeepEquals, eA.CharmConfig)
	m.c.Assert(oA.Placement, tc.DeepEquals, eA.Placement)
	m.c.Assert(oA.Resources, tc.DeepEquals, eA.Resources)
	return true
}

func (s *deployRepositorySuite) TestAddPendingResourcesForDeployFromRepositoryAPI(c *tc.C) {
	defer s.setupMocks(c).Finish()
	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	pendUp := &params.PendingResourceUpload{
		Name:     "foo-resource",
		Type:     "file",
		Filename: "bar",
	}
	meta := resource.Meta{
		Name:        "foo-resource",
		Type:        resource.TypeFile,
		Path:        "foo.txt",
		Description: "bar",
	}
	r := resource.Resource{
		Meta:   meta,
		Origin: resource.OriginUpload,
	}

	template := deployTemplate{
		applicationName: "metadata-name",
		charm:           corecharm.NewCharmInfoAdapter(corecharm.EssentialMetadata{}),
		charmURL:        charm.MustParseURL("ch:amd64/jammy/testme-5"),
		endpoints:       map[string]string{"to": "from"},
		numUnits:        1,
		origin: corecharm.Origin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel:  &charm.Channel{Risk: "stable"},
			Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04"),
		},
		placement:              []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
		resources:              map[string]string{"foo-file": "bar"},
		pendingResourceUploads: []*params.PendingResourceUpload{pendUp},
		resolvedResources:      []resource.Resource{r},
	}
	s.validator.EXPECT().ValidateArg(arg).Return(template, nil)
	info := state.CharmInfo{
		Charm: template.charm,
		ID:    "ch:amd64/jammy/testme-5",
	}

	s.state.EXPECT().AddCharmMetadata(info).Return(s.charm, nil)

	s.state.EXPECT().AddPendingResource("metadata-name", r).Return("3", nil)

	addAppArgs := state.AddApplicationArgs{
		Name: "metadata-name",
		// the app.Charm is casted into a state.Charm in the code
		// we mock it separately here (s.charm above), the test works
		// thanks to the addApplicationArgsMatcher used below
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel: &state.Channel{
				Risk: "stable",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		Devices:          map[string]state.DeviceConstraints{},
		EndpointBindings: map[string]string{"to": "from"},
		NumUnits:         1,
		Placement:        []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
		Resources:        map[string]string{"foo-resource": "3"},
		Storage:          map[string]state.StorageConstraints{},
	}
	s.state.EXPECT().AddApplication(addApplicationArgsMatcher{c: c, expectedArgs: addAppArgs}).Return(s.application, nil)

	deployFromRepositoryAPI := s.getDeployFromRepositoryAPI()

	obtainedInfo, resources, errs := deployFromRepositoryAPI.DeployFromRepository(arg)
	c.Assert(errs, tc.HasLen, 0)
	c.Assert(resources, tc.HasLen, 1)
	c.Assert(obtainedInfo, tc.DeepEquals, params.DeployFromRepositoryInfo{
		Architecture:     "amd64",
		Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
		Channel:          "stable",
		EffectiveChannel: nil,
		Name:             "metadata-name",
		Revision:         5,
	})

	c.Assert(resources, tc.DeepEquals, []*params.PendingResourceUpload{pendUp})
}

func (s *deployRepositorySuite) TestRemovePendingResourcesWhenDeployErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()
	arg := params.DeployFromRepositoryArg{
		CharmName: "testme",
	}
	pendUp := &params.PendingResourceUpload{
		Name:     "foo-resource",
		Type:     "file",
		Filename: "bar",
	}
	meta := resource.Meta{
		Name:        "foo-resource",
		Type:        resource.TypeFile,
		Path:        "foo.txt",
		Description: "bar",
	}
	r := resource.Resource{
		Meta:   meta,
		Origin: resource.OriginUpload,
	}
	template := deployTemplate{
		applicationName: "metadata-name",
		charm:           corecharm.NewCharmInfoAdapter(corecharm.EssentialMetadata{}),
		charmURL:        charm.MustParseURL("ch:amd64/jammy/testme-5"),
		endpoints:       map[string]string{"to": "from"},
		numUnits:        1,
		origin: corecharm.Origin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel:  &charm.Channel{Risk: "stable"},
			Platform: corecharm.MustParsePlatform("amd64/ubuntu/22.04"),
		},
		placement:              []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
		resources:              map[string]string{"foo-file": "bar"},
		pendingResourceUploads: []*params.PendingResourceUpload{pendUp},
		resolvedResources:      []resource.Resource{r},
	}
	s.validator.EXPECT().ValidateArg(arg).Return(template, nil)
	info := state.CharmInfo{
		Charm: template.charm,
		ID:    "ch:amd64/jammy/testme-5",
	}

	s.state.EXPECT().AddCharmMetadata(info).Return(s.charm, nil)

	s.state.EXPECT().AddPendingResource("metadata-name", r).Return("3", nil)

	addAppArgs := state.AddApplicationArgs{
		Name: "metadata-name",
		// the app.Charm is casted into a state.Charm in the code
		// we mock it separately here (s.charm above), the test works
		// thanks to the addApplicationArgsMatcher used below
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Revision: intptr(5),
			Channel: &state.Channel{
				Risk: "stable",
			},
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		Devices:          map[string]state.DeviceConstraints{},
		EndpointBindings: map[string]string{"to": "from"},
		NumUnits:         1,
		Placement:        []*instance.Placement{{Directive: "0", Scope: instance.MachineScope}},
		Resources:        map[string]string{"foo-resource": "3"},
		Storage:          map[string]state.StorageConstraints{},
	}

	s.state.EXPECT().RemovePendingResources("metadata-name", map[string]string{"foo-resource": "3"})

	s.state.EXPECT().AddApplication(addApplicationArgsMatcher{c: c, expectedArgs: addAppArgs}).Return(s.application,
		errors.New("fail"))

	deployFromRepositoryAPI := s.getDeployFromRepositoryAPI()

	obtainedInfo, resources, errs := deployFromRepositoryAPI.DeployFromRepository(arg)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(resources, tc.HasLen, 0)
	c.Assert(obtainedInfo, tc.DeepEquals, params.DeployFromRepositoryInfo{})
}

func (s *deployRepositorySuite) getDeployFromRepositoryAPI() *DeployFromRepositoryAPI {
	return &DeployFromRepositoryAPI{
		state:      s.state,
		validator:  s.validator,
		stateCharm: func(Charm) *state.Charm { return nil },
	}
}

func (s *deployRepositorySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charm = NewMockCharm(ctrl)
	s.state = NewMockDeployFromRepositoryState(ctrl)
	s.validator = NewMockDeployFromRepositoryValidator(ctrl)
	return ctrl
}
