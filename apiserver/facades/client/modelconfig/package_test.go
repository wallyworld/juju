// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsprovider.go github.com/juju/juju/secrets/provider SecretBackendProvider,SecretsBackend
