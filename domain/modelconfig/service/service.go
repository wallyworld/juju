// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/modelconfig/handlers"
	"github.com/juju/juju/domain/modelconfig/validators"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
)

// ModelDefaultsProvider is responsible for providing the default config values
// for a model.
type ModelDefaultsProvider interface {
	// ModelDefaults will return the default config values to be used for a model
	// and its config.
	ModelDefaults(context.Context) (modeldefaults.Defaults, error)
}

// ControllerState represents the state entity for accessing secret
// backend info for models.
type ControllerState interface {
	handlers.SecretBackendState
}

// State represents the state entity for accessing and setting per
// model configuration values.
type State interface {
	ProviderState
	SpaceValidatorState

	// GetModelInfo returns the uuid and type of the model,
	// or an error satisfying [modelerrors.NotFound]
	GetModelInfo(ctx context.Context) (model.UUID, model.ModelType, error)

	// AgentVersion returns the current models agent version. If no agent
	// version has been set for the current model then a error satisfying
	// [errors.NotFound] is returned.
	AgentVersion(context.Context) (string, error)

	// ModelConfigHasAttributes returns the set of attributes that model config
	// currently has set out of the list supplied.
	ModelConfigHasAttributes(context.Context, []string) ([]string, error)

	// SetModelConfig is responsible for setting the current model config and
	// overwriting all previously set values even if the config supplied is
	// empty or nil.
	SetModelConfig(context.Context, map[string]string) error

	// UpdateModelConfig is responsible for both inserting, updating and
	// removing model config values for the current model.
	UpdateModelConfig(context.Context, map[string]string, []string) error
}

// SpaceValidatorState represents the state entity for validating space-related
// model config.
type SpaceValidatorState interface {
	// SpaceExists checks if the space identified by the given space name exists.
	SpaceExists(ctx context.Context, spaceName string) (bool, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)
}

// Service defines the service for interacting with ModelConfig.
type Service struct {
	defaultsProvider ModelDefaultsProvider
	modelValidator   config.Validator
	logger           logger.Logger
	ctrlSt           ControllerState
	st               State
}

// NewService creates a new ModelConfig service.
func NewService(
	defaultsProvider ModelDefaultsProvider,
	modelValidator config.Validator,
	logger logger.Logger,
	ctrlSt ControllerState,
	st State,
) *Service {
	return &Service{
		defaultsProvider: defaultsProvider,
		modelValidator:   modelValidator,
		logger:           logger,
		ctrlSt:           ctrlSt,
		st:               st,
	}
}

func (s *Service) configHandlers(modelUUID model.UUID, modelType model.ModelType) ([]handlers.ConfigHandler, error) {
	return []handlers.ConfigHandler{
		handlers.SecretBackendHandler{
			BackendState: s.ctrlSt,
			ModelUUID:    modelUUID,
			ModelType:    modelType,
		},
	}, nil
}

func (s *Service) runOnLoadHandlers(ctx context.Context, modelUUID model.UUID, modelType model.ModelType) (map[string]string, error) {
	cfgHandlers, err := s.configHandlers(modelUUID, modelType)
	if err != nil {
		return nil, fmt.Errorf("cannot set up config handlers: %w", err)
	}
	resultConfig := make(map[string]string)
	for _, h := range cfgHandlers {
		additionalCfg, err := h.OnLoad(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot apply config handler %q during load: %w", h.Name(), err)
		}
		for k, v := range additionalCfg {
			resultConfig[k] = v
		}
	}
	return resultConfig, nil
}

// ModelConfig returns the current config for the model.
func (s *Service) ModelConfig(ctx context.Context) (*config.Config, error) {
	stConfig, err := s.st.ModelConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting model config from state: %w", err)
	}

	modelUUID, modelType, err := s.st.GetModelInfo(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "cannot load model info")
	}
	resultConfig, err := s.runOnLoadHandlers(ctx, modelUUID, modelType)
	if err != nil {
		return nil, errors.Annotate(err, "cannot process model config")
	}
	for k, v := range resultConfig {
		stConfig[k] = v
	}

	agentVersion, err := s.st.AgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting model agent version for model config: %w", err)
	}

	altConfig := transform.Map(stConfig, func(k, v string) (string, any) { return k, v })

	// We add the agent version to model config here. Over time we need to
	// remove uses of agent version from model config. We prefer to augment
	// config with this value on read rather than persisting on writing.
	altConfig[config.AgentVersionKey] = agentVersion
	return config.New(config.NoDefaults, altConfig)
}

// ModelConfigValues returns the config values for the model and the source of
// the value.
func (s *Service) ModelConfigValues(
	ctx context.Context,
) (config.ConfigValues, error) {
	cfg, err := s.ModelConfig(ctx)
	if err != nil {
		return config.ConfigValues{}, err
	}

	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return config.ConfigValues{}, fmt.Errorf("getting model defaults: %w", err)
	}

	allAttrs := cfg.AllAttrs()
	if len(allAttrs) == 0 {
		allAttrs = map[string]any{}
		for k, v := range defaults {
			allAttrs[k] = v.Value
		}
	}

	result := make(config.ConfigValues, len(allAttrs))
	for attr, val := range allAttrs {
		isDefault, source := defaults[attr].Has(val)
		if !isDefault {
			source = config.JujuModelConfigSource
		}

		result[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}

	return result, nil
}

// buildUpdatedModelConfig is responsible for taking the currently set
// ModelConfig and applying in memory update operations.
func (s *Service) buildUpdatedModelConfig(
	ctx context.Context,
	updates map[string]any,
	removeAttrs []string,
) (*config.Config, *config.Config, error) {
	current, err := s.ModelConfig(ctx)
	if err != nil {
		return nil, current, err
	}

	newConf, err := current.Remove(removeAttrs)
	if err != nil {
		return newConf, current, fmt.Errorf("building new model config with removed attributes: %w", err)
	}

	newConf, err = newConf.Apply(updates)
	if err != nil {
		return newConf, current, fmt.Errorf("building new model config with removed attributes: %w", err)
	}

	return newConf, current, nil
}

// reconcileRemovedAttributes will take a set of attributes to remove from the
// model config and figure out if there exists a model default for the
// attribute. If a model default exists then a set of updates will be provided
// to instead change the attribute to the model default. This function will also
// check that the removed attributes first exist in the model's config otherwise
// we risk bringing in model defaults that were not previously set for the model
// config.
func (s *Service) reconcileRemovedAttributes(
	ctx context.Context,
	removeAttrs []string,
) (map[string]any, error) {
	updates := map[string]any{}
	hasAttrs, err := s.st.ModelConfigHasAttributes(ctx, removeAttrs)
	if err != nil {
		return updates, fmt.Errorf("determining model config has attributes: %w", err)
	}

	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return updates, fmt.Errorf("getting model defaults for config attribute removal: %w", err)
	}

	for _, attr := range hasAttrs {
		if val := defaults[attr].Value; val != nil {
			updates[attr] = val
		}
	}

	return updates, nil
}

func (s *Service) runOnSaveHandlers(
	ctx context.Context, modelUUID model.UUID, modelType model.ModelType, newConfigAttrs map[string]any,
) (rollbacks handlers.Rollbacks, err error) {
	defer func() {
		if err != nil {
			if rbErr := rollbacks.Rollback(ctx); rbErr != nil {
				s.logger.Errorf("cannot rollback failed model config operations: %s", rbErr)
			}
			rollbacks = nil
		}
	}()

	cfgHandlers, err := s.configHandlers(modelUUID, modelType)
	if err != nil {
		return rollbacks, fmt.Errorf("cannot set up config handlers: %w", err)
	}
	for _, h := range cfgHandlers {
		rb, err := h.OnSave(ctx, newConfigAttrs)
		if err != nil {
			return nil, fmt.Errorf("cannot apply config handler %q during save: %w", h.Name(), err)
		}
		rollbacks = append(rollbacks, rb)
	}
	return rollbacks, nil
}

// SetModelConfig will remove any existing model config for the model and
// replace with the new config provided. The new config will also be hydrated
// with any model default attributes that have not been set on the config.
func (s *Service) SetModelConfig(
	ctx context.Context,
	cfg map[string]any,
) (err error) {
	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return fmt.Errorf("getting model defaults: %w", err)
	}

	// We want to make a copy of cfg so that we don't modify the users input.
	cfgCopy := make(map[string]any, len(cfg))
	for k, v := range cfg {
		cfgCopy[k] = v
	}

	for k, v := range defaults {
		applyVal := v.ApplyStrategy(cfgCopy[k])
		if applyVal != nil {
			cfgCopy[k] = applyVal
		}
	}

	setCfg, err := config.New(config.NoDefaults, cfgCopy)
	if err != nil {
		return fmt.Errorf("constructing new model config with model defaults: %w", err)
	}

	_, err = s.modelValidator.Validate(ctx, setCfg, nil)
	if err != nil {
		return fmt.Errorf("validating model config to set for model: %w", err)
	}

	modelUUID, modelType, err := s.st.GetModelInfo(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot load model info")
	}
	attrs := setCfg.AllAttrs()
	rollbacks, err := s.runOnSaveHandlers(ctx, modelUUID, modelType, attrs)
	if err != nil {
		return errors.Annotate(err, "cannot process model config")
	}
	defer func() {
		if err != nil {
			if rbErr := rollbacks.Rollback(ctx); rbErr != nil {
				s.logger.Errorf("cannot rollback failed model config operations: %s", rbErr)
			}
		}
	}()

	rawCfg, err := CoerceConfigForStorage(attrs)
	if err != nil {
		return fmt.Errorf("coercing model config for storage: %w", err)
	}

	return s.st.SetModelConfig(ctx, rawCfg)
}

// UpdateModelConfig takes a set of updated and removed attributes to apply.
// Removed attributes are replaced with their model default values should they
// exist. All model config updates are validated against the currently set
// model config. The model config is ran through several validation steps before
// being persisted. If an error occurs during validation then a
// config.ValidationError is returned. The caller can also optionally pass in
// additional config.Validators to be run.
//
// The following validations on model config are run by default:
// - Agent version is not change between updates.
// - Charmhub url is not changed between updates.
// - The networking space chosen is valid and can be used.
// - The secret backend is valid and can be used.
// - Authorized keys are not changed.
func (s *Service) UpdateModelConfig(
	ctx context.Context,
	updateAttrs map[string]any,
	removeAttrs []string,
	additionalValidators ...config.Validator,
) (err error) {
	// noop with no updates or removals to perform.
	if len(updateAttrs) == 0 && len(removeAttrs) == 0 {
		return nil
	}

	updates, err := s.reconcileRemovedAttributes(ctx, removeAttrs)
	if err != nil {
		return errors.Trace(err)
	}

	// It's important here that we apply the user updates over the top of the
	// calculated ones. This way we always take the user's supplied key value
	// over defaults.
	for k, v := range updateAttrs {
		updates[k] = v
	}

	newCfg, currCfg, err := s.buildUpdatedModelConfig(ctx, updates, removeAttrs)
	if err != nil {
		return fmt.Errorf("making updated model configuration: %w", err)
	}

	modelUUID, modelType, err := s.st.GetModelInfo(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot load model info")
	}

	_, err = s.updateModelConfigValidator(modelType, additionalValidators...).Validate(ctx, newCfg, currCfg)
	if err != nil {
		return fmt.Errorf("validating updated model configuration: %w", err)
	}

	rollbacks, err := s.runOnSaveHandlers(ctx, modelUUID, modelType, updates)
	if err != nil {
		return errors.Annotate(err, "cannot process model config")
	}
	defer func() {
		if err != nil {
			if rbErr := rollbacks.Rollback(ctx); rbErr != nil {
				s.logger.Errorf("cannot rollback failed model config operations: %s", rbErr)
			}
		}
	}()

	rawCfg, err := CoerceConfigForStorage(updates)
	if err != nil {
		return fmt.Errorf("coercing new configuration for persistence: %w", err)
	}

	err = s.st.UpdateModelConfig(ctx, rawCfg, removeAttrs)
	if err != nil {
		return fmt.Errorf("updating model config: %w", err)
	}
	return nil
}

// spaceValidator implements validators.SpaceProvider.
type spaceValidator struct {
	st SpaceValidatorState
}

// HasSpace implements validators.SpaceProvider. It checks whether the
// given space exists.
func (v *spaceValidator) HasSpace(ctx context.Context, spaceName string) (bool, error) {
	return v.st.SpaceExists(ctx, spaceName)
}

// updateModelConfigValidator returns a config validator to use on model config
// when it is being updated. The validator returned will check that:
// - Agent version is not being changed.
// - CharmhubURL is not being changed.
// - Network space exists.
// - Secret backend is valid for the type of model.
// - There is no changes to authorized keys.
func (s *Service) updateModelConfigValidator(
	modelType model.ModelType,
	additional ...config.Validator,
) config.Validator {
	agg := &config.AggregateValidator{
		Validators: []config.Validator{
			validators.AgentVersionChange(),
			validators.CharmhubURLChange(),
			validators.SpaceChecker(&spaceValidator{
				st: s.st,
			}),
			validators.SecretBackendChecker(modelType),
			validators.AuthorizedKeysChange(),
			s.modelValidator,
		},
	}
	agg.Validators = append(agg.Validators, additional...)
	return agg
}

// WatchableService defines the service for interacting with ModelConfig
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService for interacting with
// ModelConfig and the ability to create watchers.
func NewWatchableService(
	defaultsProvider ModelDefaultsProvider,
	modelValidator config.Validator,
	logger logger.Logger,
	ctrlSt ControllerState,
	st State,
	watcherFactory WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			defaultsProvider: defaultsProvider,
			modelValidator:   modelValidator,
			logger:           logger,
			ctrlSt:           ctrlSt,
			st:               st,
		},
		watcherFactory: watcherFactory,
	}
}

// Watch returns a watcher that returns keys for any changes to model
// config.
func (s *WatchableService) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher(
		"model_config", changestream.All,
		eventsource.InitialNamespaceChanges(s.st.AllKeysQuery()),
	)
}
