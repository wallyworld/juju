// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Authorizer

var NewSecretsDrainAPI = newSecretsDrainAPI
