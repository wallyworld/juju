// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/interface_mocks.go github.com/juju/juju/apiserver/facades/client/charms/services StateBackend,ModelBackend,Storage,UploadedCharm
