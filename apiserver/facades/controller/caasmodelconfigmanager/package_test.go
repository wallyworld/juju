// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/apiserver/facade Authorizer,Context,Resources
