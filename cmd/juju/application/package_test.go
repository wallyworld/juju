// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	_ "github.com/juju/juju/internal/provider/dummy"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/applicationapi_mock.go github.com/juju/juju/cmd/juju/application ApplicationAPI,RemoveApplicationAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelconfigapi_mock.go github.com/juju/juju/cmd/juju/application ModelConfigClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/deployer_mock.go github.com/juju/juju/cmd/juju/application/deployer Deployer,DeployerFactory
