// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"

	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// StubService is a special service that collects temporary methods required for
// wiring together domains which not completely implemented or wired up.
//
// Given the temporary nature of this service, we have not implemented the full
// service/state layer indirection. Instead, the service directly uses a transaction
// runner.
//
// Deprecated: All methods here should be thrown away as soon as we're done with
// then.
type StubService struct {
	modelUUID       coremodel.UUID
	modelState      *domain.StateBase
	controllerState *domain.StateBase

	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken]
}

// ProviderWithSecretToken is a subset of caas broker.
type ProviderWithSecretToken interface {
	GetSecretToken(ctx context.Context, name string) (string, error)
}

// NewStubService returns a new StubService.
func NewStubService(
	modelUUID coremodel.UUID,
	controllerFactory database.TxnRunnerFactory,
	modelFactory database.TxnRunnerFactory,
	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken],
) *StubService {
	return &StubService{
		modelUUID:               modelUUID,
		controllerState:         domain.NewStateBase(controllerFactory),
		modelState:              domain.NewStateBase(modelFactory),
		providerWithSecretToken: providerWithSecretToken,
	}
}

// GetExecSecretToken returns a token that can be used to run exec operations
// on the provider cloud.
func (s *StubService) GetExecSecretToken(ctx context.Context) (string, error) {
	provider, err := s.providerWithSecretToken(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return "", errors.Errorf("getting secret token %w", coreerrors.NotSupported)
	}
	if err != nil {
		return "", errors.Capture(err)
	}

	return provider.GetSecretToken(ctx, k8sprovider.ExecRBACResourceName)
}
