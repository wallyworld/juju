// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Resources,Authorizer
