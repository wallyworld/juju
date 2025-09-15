// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/core/logger Logger,LoggerCloser
