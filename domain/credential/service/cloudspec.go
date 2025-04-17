// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jujucloud "github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
)

// GetModelCloudSpec returns the cloud spec for the model.
func (s *Service) GetModelCloudSpec(ctx context.Context, modelUUID coremodel.UUID) (cloudspec.CloudSpec, error) {
	cld, cloudRegion, credInfo, err := s.st.GetModelCloudAndCredential(ctx, modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		err = coreerrors.NotFound
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	var cloudCred *jujucloud.Credential
	if credInfo != nil {
		c := jujucloud.NewCredential(jujucloud.AuthType(credInfo.AuthType), credInfo.Attributes)
		cloudCred = &c
	}
	return cloudspec.MakeCloudSpec(*cld, cloudRegion, cloudCred)
}
