// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

//go:generate go run go.uber.org/mock/mockgen -package leaseexpiry_test -destination clock_mock_test.go github.com/juju/clock Clock,Timer

type StubLogger struct{}

func (StubLogger) Infof(string, ...interface{}) {}

func (StubLogger) Debugf(string, ...interface{}) {}
