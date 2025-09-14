// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/client/resources Backend,NewCharmRepository
