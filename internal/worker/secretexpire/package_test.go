// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/internal/worker/secretexpire SecretManagerFacade
