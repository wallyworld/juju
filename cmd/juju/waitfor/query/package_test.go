// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

//go:generate go run go.uber.org/mock/mockgen -package query -destination scope_mock_test.go github.com/juju/juju/cmd/juju/waitfor/query FuncScope,Scope
