// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/hookcommit"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
)

type State interface {
	AtomicState
}

type AtomicState interface {
	domain.AtomicStateBase

	GetUnitUUID(ctx domain.AtomicContext, unitName string) (coreunit.UUID, error)

	// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
	// The opened and closed ports for the same endpoints must not conflict.
	UpdateUnitPorts(ctx domain.AtomicContext, unitUUID coreunit.UUID, openPorts, closePorts network.GroupedPortRanges) error

	// SetUnitStateCharm replaces the agent charm
	// state for the unit with the input UUID.
	SetUnitStateCharm(domain.AtomicContext, string, map[string]string) error

	CreateCharmApplicationSecret(
		ctx domain.AtomicContext, version int, uri *secrets.URI, appName string, secret domainsecret.UpsertSecretParams,
	) error
	CreateCharmUnitSecret(
		ctx domain.AtomicContext, version int, uri *secrets.URI, unitName string, secret domainsecret.UpsertSecretParams,
	) error
	UpdateSecret(ctx domain.AtomicContext, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error

	DeleteSecret(ctx domain.AtomicContext, uri *secrets.URI, revs []int) ([]string, error)

	GrantAccess(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.GrantParams) error
	RevokeAccess(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.AccessParams) error
}

type Service struct {
	st     State
	clock  clock.Clock
	logger logger.Logger
}

func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		clock:  clock.WallClock,
		logger: logger,
	}
}

func (s *Service) CommitHookChanges(ctx context.Context, unitName string, changes hookcommit.CommitHookChangesParams) error {
	err := s.st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return err
		}
		_ = unitUUID

		for _, c := range changes.SecretCreates {
			if err := s.createCharmSecret(ctx, c); err != nil {
				return err
			}
		}

		for _, d := range changes.SecretDeletes {
			if err := s.deleteSecret(ctx, d); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (s *Service) createCharmSecret(ctx domain.AtomicContext, params hookcommit.CreateCharmSecretParams) (errOut error) {
	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		ValueRef:    params.ValueRef,
		Checksum:    params.Checksum,
	}
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
	}

	rotatePolicy := domainsecret.MarshallRotatePolicy(params.RotatePolicy)
	p.RotatePolicy = &rotatePolicy
	if params.RotatePolicy.WillRotate() {
		p.NextRotateTime = params.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	p.ExpireTime = params.ExpireTime

	// etc....

	return nil
}

// deleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *Service) deleteSecret(ctx domain.AtomicContext, params hookcommit.DeleteSecretParams) error {
	return nil
}
