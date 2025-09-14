// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/jujuapi_mocks.go github.com/juju/juju/secrets JujuAPIClient,SecretsState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backend_mocks.go github.com/juju/juju/secrets/provider SecretsBackend
