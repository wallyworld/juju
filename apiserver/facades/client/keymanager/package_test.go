// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/keymanager_mock.go github.com/juju/juju/apiserver/facades/client/keymanager Model,BlockChecker
