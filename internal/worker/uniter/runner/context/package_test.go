// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	_ "github.com/juju/juju/secrets/provider/all"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/hookunit_mock.go github.com/juju/juju/internal/worker/uniter/runner/context HookUnit
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/internal/worker/uniter/runner/context LeadershipContext
